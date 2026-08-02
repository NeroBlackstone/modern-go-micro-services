package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// 配置
var (
	hydraPublicURL string
	hydraAdminURL  string
	clientID       string
	clientSecret   string
	redirectURI    string
	serverPort     string
)

// TokenResponse OAuth2 Token 响应
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	IDToken      string `json:"id_token"`
}

// UserSession 用户会话
type UserSession struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
	ExpiresAt    time.Time
	UserInfo     map[string]any
}

// 全局会话存储（演示用）
var sessions = map[string]*UserSession{}

func main() {
	hydraPublicURL = getEnv("HYDRA_PUBLIC_URL", "http://localhost:4444")
	hydraAdminURL = getEnv("HYDRA_ADMIN_URL", "http://localhost:4445")
	clientID = getEnv("CLIENT_ID", "bookstore-web-client")
	clientSecret = getEnv("CLIENT_SECRET", "change-me-in-production")
	redirectURI = getEnv("REDIRECT_URI", "http://localhost:8082/callback")
	serverPort = getEnv("SERVER_PORT", "8082")

	mux := http.NewServeMux()

	// 首页 - 显示登录按钮
	mux.HandleFunc("/", handleIndex)
	// OAuth2 登录端点
	mux.HandleFunc("/login", handleLogin)
	// OAuth2 回调端点
	mux.HandleFunc("/callback", handleCallback)
	// 用户信息页面
	mux.HandleFunc("/profile", handleProfile)
	// 登出
	mux.HandleFunc("/logout", handleLogout)
	// API 测试 - 用 access_token 访问受保护资源
	mux.HandleFunc("/api/test", handleAPITest)
	// 健康检查
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"status":"ok"}`)
	})

	log.Printf("OAuth2 客户端演示应用启动在 :%s", serverPort)
	log.Printf("  Hydra Public URL: %s", hydraPublicURL)
	log.Printf("  Client ID: %s", clientID)
	log.Printf("  Redirect URI: %s", redirectURI)
	log.Fatal(http.ListenAndServe(":"+serverPort, mux))
}

// handleIndex 首页
func handleIndex(w http.ResponseWriter, r *http.Request) {
	sessionID := getSessionID(r)
	session := sessions[sessionID]

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>OAuth2 Client Demo</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, sans-serif; max-width: 800px; margin: 50px auto; padding: 20px; }
        .card { border: 1px solid #ddd; border-radius: 8px; padding: 20px; margin: 20px 0; }
        .btn { display: inline-block; padding: 10px 20px; background: #007bff; color: white; text-decoration: none; border-radius: 4px; margin: 5px; }
        .btn:hover { background: #0056b3; }
        .btn-danger { background: #dc3545; }
        .btn-danger:hover { background: #c82333; }
        .token { background: #f5f5f5; padding: 10px; border-radius: 4px; word-break: break-all; font-family: monospace; font-size: 12px; }
        h1 { color: #333; }
        h2 { color: #555; }
    </style>
</head>
<body>
    <h1>🔐 OAuth2 Client Demo</h1>
    <p>这是一个演示 OAuth2 Authorization Code 流程的客户端应用。</p>

    <div class="card">
        <h2>📋 当前状态</h2>`)

	if session != nil {
		fmt.Fprintf(w, `
        <p>✅ 已登录</p>
        <p><strong>Access Token:</strong></p>
        <div class="token">%s</div>
        <p><strong>Token 过期时间:</strong> %s</p>
        <p>
            <a href="/profile" class="btn">查看用户信息</a>
            <a href="/api/test" class="btn">测试 API 调用</a>
            <a href="/logout" class="btn btn-danger">登出</a>
        </p>`, session.AccessToken[:50]+"...", session.ExpiresAt.Format("2006-01-02 15:04:05"))
	} else {
		fmt.Fprint(w, `
        <p>❌ 未登录</p>
        <p><a href="/login" class="btn">使用 OAuth2 登录</a></p>`)
	}

	fmt.Fprint(w, `
    </div>

    <div class="card">
        <h2>📚 OAuth2 流程说明</h2>
        <ol>
            <li>点击「使用 OAuth2 登录」→ 重定向到 Hydra 授权端点</li>
            <li>Hydra 检查用户是否已登录（通过 Kratos session）</li>
            <li>如果未登录，跳转到 Kratos 登录页面</li>
            <li>登录成功后，Hydra 请求用户授权（consent）</li>
            <li>用户同意后，Hydra 重定向回此应用，携带 authorization_code</li>
            <li>应用用 code 换取 access_token</li>
            <li>使用 access_token 访问受保护的 API</li>
        </ol>
    </div>
</body>
</html>`)
}

// handleLogin 发起 OAuth2 授权请求
func handleLogin(w http.ResponseWriter, r *http.Request) {
	// 生成 state 防止 CSRF
	state := base64.URLEncoding.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))

	// 构建授权 URL
	authURL := fmt.Sprintf("%s/oauth2/auth?response_type=code&client_id=%s&redirect_uri=%s&scope=openid+offline+email+profile&state=%s",
		hydraPublicURL,
		url.QueryEscape(clientID),
		url.QueryEscape(redirectURI),
		state,
	)

	log.Printf("[Login] 发起 OAuth2 授权请求，state=%s", state)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleCallback 处理 OAuth2 回调
func handleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errorParam := r.URL.Query().Get("error")

	if errorParam != "" {
		errorDesc := r.URL.Query().Get("error_description")
		log.Printf("[Callback] 授权失败: %s - %s", errorParam, errorDesc)
		http.Error(w, fmt.Sprintf("授权失败: %s - %s", errorParam, errorDesc), http.StatusBadRequest)
		return
	}

	if code == "" {
		http.Error(w, "missing authorization code", http.StatusBadRequest)
		return
	}

	log.Printf("[Callback] 收到 authorization_code，state=%s", state)

	// 用 code 换取 token
	tokenResp, err := exchangeCodeForToken(code)
	if err != nil {
		log.Printf("[Callback] 换取 token 失败: %v", err)
		http.Error(w, fmt.Sprintf("换取 token 失败: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("[Callback] 成功获取 token，类型: %s，过期时间: %d 秒", tokenResp.TokenType, tokenResp.ExpiresIn)

	// 解析 JWT 获取用户信息
	userInfo := parseJWTClaims(tokenResp.IDToken)
	log.Printf("[Callback] 用户信息: %v", userInfo)

	// 存储会话
	sessionID := generateSessionID()
	sessions[sessionID] = &UserSession{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		IDToken:      tokenResp.IDToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		UserInfo:     userInfo,
	}

	// 设置 session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_session",
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   3600,
	})

	log.Printf("[Callback] 登录成功，重定向到首页")
	http.Redirect(w, r, "/", http.StatusFound)
}

// handleProfile 显示用户信息
func handleProfile(w http.ResponseWriter, r *http.Request) {
	sessionID := getSessionID(r)
	session := sessions[sessionID]

	if session == nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>User Profile - OAuth2 Demo</title>
    <style>
        body { font-family: -apple-system, sans-serif; max-width: 800px; margin: 50px auto; padding: 20px; }
        .card { border: 1px solid #ddd; border-radius: 8px; padding: 20px; margin: 20px 0; }
        .btn { display: inline-block; padding: 10px 20px; background: #007bff; color: white; text-decoration: none; border-radius: 4px; }
        pre { background: #f5f5f5; padding: 15px; border-radius: 4px; overflow-x: auto; }
    </style>
</head>
<body>
    <h1>👤 用户信息</h1>
    <div class="card">
        <h2>JWT Claims (ID Token)</h2>
        <pre>%s</pre>
    </div>
    <div class="card">
        <h2>Access Token</h2>
        <p><code>%s</code></p>
    </div>
    <a href="/" class="btn">返回首页</a>
</body>
</html>`, prettyJSON(session.UserInfo), session.AccessToken)
}

// handleAPITest 测试用 access_token 调用受保护 API
func handleAPITest(w http.ResponseWriter, r *http.Request) {
	sessionID := getSessionID(r)
	session := sessions[sessionID]

	if session == nil {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	// 调用 user-service 的受保护 API
	apiURL := "http://localhost:80/api/v1/user/profile"
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("创建请求失败: %v", err), http.StatusInternalServerError)
		return
	}

	// 添加 Authorization header
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("API 调用失败: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{
    "api_endpoint": "%s",
    "status_code": %d,
    "response": %s
}`, apiURL, resp.StatusCode, string(body))
}

// handleLogout 登出
func handleLogout(w http.ResponseWriter, r *http.Request) {
	sessionID := getSessionID(r)

	// 删除会话
	if sessionID != "" {
		delete(sessions, sessionID)
	}

	// 清除 cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	log.Printf("[Logout] 用户已登出")
	http.Redirect(w, r, "/", http.StatusFound)
}

// ==================== Token 交换 ====================

func exchangeCodeForToken(code string) (*TokenResponse, error) {
	// 使用 Basic Auth 认证
	auth := base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/oauth2/token", hydraPublicURL), strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic "+auth)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	return &tokenResp, nil
}

// ==================== JWT 解析 ====================

func parseJWTClaims(token string) map[string]any {
	// 简化的 JWT 解析（仅用于演示）
	// 生产环境应使用专业的 JWT 库
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return map[string]any{"error": "invalid JWT format"}
	}

	payload, err := base64.URLEncoding.DecodeString(parts[1])
	if err != nil {
		// 尝试带 padding 的解码
		payload, err = base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return map[string]any{"error": "failed to decode JWT payload"}
		}
	}

	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return map[string]any{"error": "failed to parse JWT claims"}
	}

	return claims
}

// ==================== 工具函数 ====================

func getSessionID(r *http.Request) string {
	cookie, err := r.Cookie("oauth_session")
	if err != nil {
		return ""
	}
	return cookie.Value
}

func generateSessionID() string {
	return fmt.Sprintf("sess_%d", time.Now().UnixNano())
}

func prettyJSON(data any) string {
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", data)
	}
	return string(bytes)
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
