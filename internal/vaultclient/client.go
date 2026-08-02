package vaultclient

import (
	"fmt"
	"os"
	"strconv"

	vault "github.com/hashicorp/vault/api"
	"go.uber.org/zap"
)

// NewClientFromEnv 从环境变量创建 Vault 客户端
// 需要 VAULT_ADDR 和 VAULT_TOKEN 环境变量
func NewClientFromEnv() *vault.Client {
	addr := os.Getenv("VAULT_ADDR")
	token := os.Getenv("VAULT_TOKEN")
	if addr == "" || token == "" {
		return nil
	}

	cfg := vault.DefaultConfig()
	cfg.Address = addr

	client, err := vault.NewClient(cfg)
	if err != nil {
		return nil
	}
	client.SetToken(token)
	return client
}

// GetKVSecret 读取 KV v2 密钥，返回 data.data 下的 map
func GetKVSecret(client *vault.Client, path string) (map[string]string, error) {
	secret, err := client.Logical().Read(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if secret == nil {
		return nil, fmt.Errorf("secret not found: %s", path)
	}

	// KV v2 返回结构：data.data 包含实际数据
	data, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid format at %s", path)
	}

	result := make(map[string]string, len(data))
	for k, v := range data {
		result[k] = fmt.Sprintf("%v", v)
	}
	return result, nil
}

// OverlayDatabaseConfig 从 Vault 覆盖数据库配置
func OverlayDatabaseConfig(client *vault.Client, kvPath string, host *string, port *int, user *string, password *string, dbname *string, logger *zap.Logger) {
	if client == nil {
		return
	}
	secrets, err := GetKVSecret(client, kvPath)
	if err != nil {
		logger.Warn("vault read failed, using local config", zap.String("path", kvPath), zap.Error(err))
		return
	}
	if v := secrets["host"]; v != "" {
		*host = v
	}
	if v := secrets["port"]; v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			*port = p
		}
	}
	if v := secrets["user"]; v != "" {
		*user = v
	}
	if v := secrets["password"]; v != "" {
		*password = v
	}
	if v := secrets["dbname"]; v != "" {
		*dbname = v
	}
	logger.Info("database overlaid from vault", zap.String("path", kvPath))
}

// OverlayString 从 Vault 密钥中覆盖单个字符串字段
func OverlayString(secrets map[string]string, key string, target *string) {
	if val, ok := secrets[key]; ok && val != "" {
		*target = val
	}
}
