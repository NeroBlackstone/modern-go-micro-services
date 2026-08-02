package webhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// RegistrationHookRequest Kratos 注册后 webhook 请求
type RegistrationHookRequest struct {
	Identity Identity `json:"identity"`
}

type Identity struct {
	ID       string                 `json:"id"`
	Traits   map[string]interface{} `json:"traits"`
	SchemaID string                 `json:"schema_id"`
}

// RegistrationHookResponse webhook 响应
type RegistrationHookResponse struct {
	Status string `json:"status"`
}

// KetoClient Keto API 客户端
type KetoClient struct {
	WriteURL string
	Client   *http.Client
}

// NewKetoClient 创建 Keto 客户端
func NewKetoClient(writeURL string) *KetoClient {
	return &KetoClient{
		WriteURL: writeURL,
		Client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// AddUserToGroup 将用户添加到指定组
func (k *KetoClient) AddUserToGroup(userID, group string) error {
	tuple := map[string]interface{}{
		"namespace":  "Group",
		"object":     group,
		"relation":   "members",
		"subject_id": userID,
	}

	jsonData, err := json.Marshal(tuple)
	if err != nil {
		return fmt.Errorf("failed to marshal tuple: %w", err)
	}

	url := fmt.Sprintf("%s/admin/relation-tuples", k.WriteURL)
	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := k.Client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// HandleRegistrationHook 处理注册后的 hook
func HandleRegistrationHook(ketoClient *KetoClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req RegistrationHookRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		userID := req.Identity.ID
		if userID == "" {
			http.Error(w, "Missing user ID", http.StatusBadRequest)
			return
		}

		// 自动将新用户添加到 users 组
		if err := ketoClient.AddUserToGroup(userID, "users"); err != nil {
			http.Error(w, fmt.Sprintf("Failed to add user to group: %v", err), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(RegistrationHookResponse{
			Status: "success",
		})
	}
}
