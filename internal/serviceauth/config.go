package serviceauth

import "time"

// Config 服务间认证配置
type Config struct {
	// SharedSecret HMAC 签名密钥（服务间共享）
	SharedSecret string `mapstructure:"shared_secret"`
	// ServiceName 当前服务名称（用于 JWT iss 字段）
	ServiceName string `mapstructure:"service_name"`
	// TargetService 目标服务名称（用于 JWT aud 字段）
	TargetService string `mapstructure:"target_service"`
	// TTL Token 有效期
	TTL time.Duration `mapstructure:"ttl"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		SharedSecret: "change-me-in-production",
		ServiceName:  "unknown",
		TargetService: "unknown",
		TTL:          5 * time.Minute,
	}
}
