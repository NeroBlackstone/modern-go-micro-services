package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

// Hydra 管理 API 地址
var hydraAdminURL string

// Kratos 公共 API 地址
var kratosPublicURL string

func main() {
	hydraAdminURL = getEnv("HYDRA_ADMIN_URL", "http://localhost:4445")
	kratosPublicURL = getEnv("KRATOS_PUBLIC_URL", "http://localhost:4433")
	port := getEnv("SERVER_PORT", "3001")

	mux := http.NewServeMux()

	// OAuth2 登录挑战处理
	mux.HandleFunc("/login", handleLogin)
	// OAuth2 授权同意挑战处理
	mux.HandleFunc("/consent", handleConsent)
	// OAuth2 登出挑战处理
	mux.HandleFunc("/logout", handleLogout)

	// 健康检查
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"status":"ok"}`)
	})

	log.Printf("Hydra Login/Consent 服务启动在 :%s", port)
	log.Printf("  Hydra Admin API: %s", hydraAdminURL)
	log.Printf("  Kratos Public API: %s", kratosPublicURL)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

// handleLogin 处理 Hydra 的登录挑战
// 流程：
// 1. Hydra 重定向到此端点，携带 login_challenge
// 2. 调用 Hydra Admin API 获取登录请求信息
// 3. 检查用户是否有有效的 Kratos Session
// 4. 如果有 session，接受登录请求；如果没有，要求用户登录
func handleLogin(w http.ResponseWriter, r *http.Request) {
	challenge := r.URL.Query().Get("login_challenge")
	if challenge == "" {
		http.Error(w, "missing login_challenge", http.StatusBadRequest)
		return
	}

	log.Printf("[Login] 收到登录挑战: %s", challenge)

	// 1. 获取 Hydra 登录请求信息
	loginRequest, err := getLoginRequest(challenge)
	if err != nil {
		log.Printf("[Login] 获取登录请求失败: %v", err)
		http.Error(w, "failed to get login request", http.StatusInternalServerError)
		return
	}

	log.Printf("[Login] 请求信息: subject=%s, skip=%v, client_id=%s",
		loginRequest.Subject, loginRequest.Skip, loginRequest.ClientID)

	// 2. 如果 Hydra 已经有 subject（已登录用户），直接接受
	if loginRequest.Skip && loginRequest.Subject != "" {
		log.Printf("[Login] 跳过登录，用户已认证: %s", loginRequest.Subject)
		acceptLogin(w, r, challenge, loginRequest.Subject)
		return
	}

	// 3. 检查用户是否有有效的 Kratos Session
	kratosSession, err := checkKratosSession(r)
	if err != nil {
		// 没有有效 session，重定向到登录页面
		log.Printf("[Login] 无有效 Kratos Session，需要登录")
		redirectToLogin(w, r, challenge)
		return
	}

	// 4. 有有效 session，接受登录请求
	log.Printf("[Login] Kratos Session 有效，用户: %s", kratosSession.Identity.ID)
	acceptLogin(w, r, challenge, kratosSession.Identity.ID)
}

// handleConsent 处理 Hydra 的授权同意挑战
// 流程：
// 1. Hydra 重定向到此端点，携带 consent_challenge
// 2. 获取 consent 请求信息（scope、client 等）
// 3. 自动批准（开发环境）或要求用户确认
// 4. 接受 consent 请求，Hydra 重定向回客户端并携带 authorization_code
func handleConsent(w http.ResponseWriter, r *http.Request) {
	challenge := r.URL.Query().Get("consent_challenge")
	if challenge == "" {
		http.Error(w, "missing consent_challenge", http.StatusBadRequest)
		return
	}

	log.Printf("[Consent] 收到授权同意挑战: %s", challenge)

	// 1. 获取 Hydra consent 请求信息
	consentRequest, err := getConsentRequest(challenge)
	if err != nil {
		log.Printf("[Consent] 获取 consent 请求失败: %v", err)
		http.Error(w, "failed to get consent request", http.StatusInternalServerError)
		return
	}

	log.Printf("[Consent] 请求信息: subject=%s, client_id=%s, scope=%v",
		consentRequest.Subject, consentRequest.ClientID, consentRequest.RequestedScope)

	// 2. 如果已经给出过 consent，直接接受
	if consentRequest.Skip {
		log.Printf("[Consent] 跳过同意，已有历史授权")
		acceptConsent(w, r, challenge, consentRequest.Subject, consentRequest.RequestedScope)
		return
	}

	// 3. 开发环境自动批准 consent
	// 生产环境应该展示同意页面让用户手动确认
	log.Printf("[Consent] 自动批准授权（开发模式）")
	acceptConsent(w, r, challenge, consentRequest.Subject, consentRequest.RequestedScope)
}

// handleLogout 处理 Hydra 的登出挑战
func handleLogout(w http.ResponseWriter, r *http.Request) {
	challenge := r.URL.Query().Get("logout_challenge")
	if challenge == "" {
		http.Error(w, "missing logout_challenge", http.StatusBadRequest)
		return
	}

	log.Printf("[Logout] 收到登出挑战: %s", challenge)

	// 接受登出请求
	acceptLogout(w, r, challenge)
}

// ==================== Hydra Admin API 调用 ====================

type LoginRequest struct {
	Request        string         `json:"request"`
	SessionID      string         `json:"session_id"`
	Subject        string         `json:"subject"`
	Requester      string         `json:"requester"`
	ClientID       string         `json:"client_id"`
	ClientName     string         `json:"client_name"`
	RequestedAt    string         `json:"requested_at"`
	RequestedScope []string       `json:"requested_scope"`
	OIDCContext    map[string]any `json:"oidc_context"`
	Skip           bool           `json:"skip"`
	ChallengeID    string         `json:"challenge_id"`
}

type ConsentRequest struct {
	Request            string         `json:"request"`
	SessionID          string         `json:"session_id"`
	Subject            string         `json:"subject"`
	Requester          string         `json:"requester"`
	ClientID           string         `json:"client_id"`
	ClientName         string         `json:"client_name"`
	RequestedAt        string         `json:"requested_at"`
	RequestedScope     []string       `json:"requested_scope"`
	GrantedScope       []string       `json:"granted_scope"`
	OIDCContext        map[string]any `json:"oidc_context"`
	Skip               bool           `json:"skip"`
	ChallengeID        string         `json:"challenge_id"`
	SessionIDToken     map[string]any `json:"oidc_session_id_token"`
	SessionAccessToken map[string]any `json:"oidc_session_access_token"`
}

type KratosSession struct {
	ID       string `json:"id"`
	Active   bool   `json:"active"`
	Identity struct {
		ID     string         `json:"id"`
		Traits map[string]any `json:"traits"`
	} `json:"identity"`
	ExpiresAt string `json:"expires_at"`
	IssuedAt  string `json:"issued_at"`
}

func getLoginRequest(challenge string) (*LoginRequest, error) {
	resp, err := http.Get(fmt.Sprintf("%s/admin/oauth2/auth/requests/login?login_challenge=%s", hydraAdminURL, challenge))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var req LoginRequest
	if err := json.NewDecoder(resp.Body).Decode(&req); err != nil {
		return nil, err
	}
	return &req, nil
}

func getConsentRequest(challenge string) (*ConsentRequest, error) {
	resp, err := http.Get(fmt.Sprintf("%s/admin/oauth2/auth/requests/consent?consent_challenge=%s", hydraAdminURL, challenge))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var req ConsentRequest
	if err := json.NewDecoder(resp.Body).Decode(&req); err != nil {
		return nil, err
	}
	return &req, nil
}

func acceptLogin(w http.ResponseWriter, r *http.Request, challenge, subject string) {
	body := map[string]any{
		"subject":      subject,
		"remember":     true,
		"remember_for": 3600, // 1 小时
		"acmr": map[string]any{
			"force_subject_identifier": subject,
		},
	}

	data, _ := json.Marshal(body)
	resp, err := http.Post(
		fmt.Sprintf("%s/admin/oauth2/auth/requests/login/accept?login_challenge=%s", hydraAdminURL, challenge),
		"application/json",
		strings.NewReader(string(data)),
	)
	if err != nil {
		log.Printf("[Login] 接受登录请求失败: %v", err)
		http.Error(w, "failed to accept login", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("[Login] 解析响应失败: %v", err)
		http.Error(w, "failed to parse response", http.StatusInternalServerError)
		return
	}

	redirectTo, ok := result["redirect_to"].(string)
	if !ok {
		log.Printf("[Login] 响应中无 redirect_to")
		http.Error(w, "no redirect_to in response", http.StatusInternalServerError)
		return
	}

	log.Printf("[Login] 登录成功，重定向到: %s", redirectTo)
	http.Redirect(w, r, redirectTo, http.StatusFound)
}

func acceptConsent(w http.ResponseWriter, r *http.Request, challenge, subject string, requestedScope []string) {
	body := map[string]any{
		"grant_scope":                 requestedScope,
		"grant_access_token_audience": requestedScope,
		"remember":                    true,
		"remember_for":                3600,
		"session": map[string]any{
			"id_token": map[string]any{
				"email": getTrait(subject, "email"),
				"name":  getTrait(subject, "username"),
			},
			"access_token": map[string]any{
				"email":    getTrait(subject, "email"),
				"username": getTrait(subject, "username"),
			},
		},
	}

	data, _ := json.Marshal(body)
	resp, err := http.Post(
		fmt.Sprintf("%s/admin/oauth2/auth/requests/consent/accept?consent_challenge=%s", hydraAdminURL, challenge),
		"application/json",
		strings.NewReader(string(data)),
	)
	if err != nil {
		log.Printf("[Consent] 接受 consent 请求失败: %v", err)
		http.Error(w, "failed to accept consent", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("[Consent] 解析响应失败: %v", err)
		http.Error(w, "failed to parse response", http.StatusInternalServerError)
		return
	}

	redirectTo, ok := result["redirect_to"].(string)
	if !ok {
		log.Printf("[Consent] 响应中无 redirect_to")
		http.Error(w, "no redirect_to in response", http.StatusInternalServerError)
		return
	}

	log.Printf("[Consent] 同意成功，重定向到: %s", redirectTo)
	http.Redirect(w, r, redirectTo, http.StatusFound)
}

func acceptLogout(w http.ResponseWriter, r *http.Request, challenge string) {
	body := map[string]any{}

	data, _ := json.Marshal(body)
	resp, err := http.Post(
		fmt.Sprintf("%s/admin/oauth2/auth/requests/logout/accept?logout_challenge=%s", hydraAdminURL, challenge),
		"application/json",
		strings.NewReader(string(data)),
	)
	if err != nil {
		log.Printf("[Logout] 接受登出请求失败: %v", err)
		http.Error(w, "failed to accept logout", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("[Logout] 解析响应失败: %v", err)
		http.Error(w, "failed to parse response", http.StatusInternalServerError)
		return
	}

	redirectTo, ok := result["redirect_to"].(string)
	if !ok {
		log.Printf("[Logout] 响应中无 redirect_to")
		http.Error(w, "no redirect_to in response", http.StatusInternalServerError)
		return
	}

	log.Printf("[Logout] 登出成功，重定向到: %s", redirectTo)
	http.Redirect(w, r, redirectTo, http.StatusFound)
}

// ==================== Kratos Session 验证 ====================

func checkKratosSession(r *http.Request) (*KratosSession, error) {
	// 从 cookie 中获取 session token
	cookie, err := r.Cookie("ory_kratos_session")
	if err != nil {
		return nil, fmt.Errorf("no kratos session cookie")
	}

	// 调用 Kratos /session/whoami 验证 session
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/session/whoami", kratosPublicURL), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", fmt.Sprintf("ory_kratos_session=%s", cookie.Value))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kratos session invalid, status: %d", resp.StatusCode)
	}

	var session KratosSession
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, err
	}

	return &session, nil
}

func redirectToLogin(w http.ResponseWriter, r *http.Request, challenge string) {
	// 重定向到 Kratos 登录页面，登录完成后回调到 hydra-login-consent
	loginURL := fmt.Sprintf("http://localhost:4433/self-service/login/browser?return_to=http://localhost:3001/login?login_challenge=%s", challenge)
	http.Redirect(w, r, loginURL, http.StatusFound)
}

// ==================== 工具函数 ====================

// getTrait 从 Kratos identity traits 中提取字段
// 注意：这是一个简化实现，实际应该通过 Kratos session 获取
func getTrait(subject, key string) string {
	// 在生产环境中，应该调用 Kratos Admin API 获取 identity traits
	// 这里返回占位值，实际值由 Hydra 的 session 配置决定
	return ""
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// init 创建一个示例 OAuth2 Client（如果不存在）
func init() {
	// 这个函数在服务启动时执行
	// 实际的 client 创建应该通过 hydra create client 命令完成
	log.Println("[Init] Hydra Login/Consent 服务初始化完成")
	log.Println("[Init] 提示: 使用以下命令创建 OAuth2 Client:")
	log.Println("[Init]   hydra create client \\")
	log.Println("[Init]     --endpoint http://localhost:4445 \\")
	log.Println("[Init]     --grant-type authorization_code,refresh_token \\")
	log.Println("[Init]     --response-type code \\")
	log.Println("[Init]     --token-endpoint-auth-method client_secret_basic \\")
	log.Println("[Init]     --name bookstore-web-client \\")
	log.Println("[Init]     --redirect-uri http://localhost:8082/callback")
}
