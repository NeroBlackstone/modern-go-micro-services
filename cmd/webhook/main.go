package main

import (
	"log"
	"net/http"
	"os"

	"modern-micro-services/internal/webhook"
)

func main() {
	// Keto Write API 地址
	ketoWriteURL := os.Getenv("KETO_WRITE_URL")
	if ketoWriteURL == "" {
		ketoWriteURL = "http://keto:4467"
	}

	// 创建 Keto 客户端
	ketoClient := webhook.NewKetoClient(ketoWriteURL)

	// 注册路由
	mux := http.NewServeMux()
	mux.HandleFunc("/webhooks/registration", webhook.HandleRegistrationHook(ketoClient))

	// 健康检查
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	port := os.Getenv("WEBHOOK_PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting webhook server on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
