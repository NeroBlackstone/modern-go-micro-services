package serviceauth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ServiceClaims 服务间 JWT 的 Claims
type ServiceClaims struct {
	// Iss 签发者（调用方服务名称）
	Iss string `json:"iss"`
	// Aud 接收者（目标服务名称）
	Aud string `json:"aud"`
	// Sub 调用方服务名称（与 iss 相同，用于快速识别）
	Sub string `json:"sub"`
	// CallerUser 原始用户 ID（从 Oathkeeper JWT 传递而来，可选）
	CallerUser string `json:"caller_user,omitempty"`
	// CallerEmail 原始用户邮箱（从 Oathkeeper JWT 传递而来，可选）
	CallerEmail string `json:"caller_email,omitempty"`
	jwt.RegisteredClaims
}

// GenerateToken 使用 HMAC-SHA256 签发服务间 JWT
func GenerateToken(cfg *Config, callerUser, callerEmail string) (string, error) {
	now := time.Now()
	claims := ServiceClaims{
		Iss:         cfg.ServiceName,
		Aud:         cfg.TargetService,
		Sub:         cfg.ServiceName,
		CallerUser:  callerUser,
		CallerEmail: callerEmail,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(cfg.TTL)),
			NotBefore: jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(cfg.SharedSecret))
	if err != nil {
		return "", fmt.Errorf("sign service token: %w", err)
	}
	return signed, nil
}

// ValidateToken 验证并解析服务间 JWT
func ValidateToken(cfg *Config, tokenString string) (*ServiceClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &ServiceClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(cfg.SharedSecret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse service token: %w", err)
	}

	claims, ok := token.Claims.(*ServiceClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid service token")
	}

	// 验证 aud 字段（目标服务必须匹配）
	if claims.Aud != cfg.ServiceName {
		return nil, fmt.Errorf("token audience mismatch: expected %s, got %s", cfg.ServiceName, claims.Aud)
	}

	return claims, nil
}
