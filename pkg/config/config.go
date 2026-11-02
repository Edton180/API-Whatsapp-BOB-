package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type AppConfig struct {
	ServerPort         string
	GlobalAPIKey       string
	DatabaseDSN        string
	N8NWebhookURL      string
	ChatwootBaseURL    string
	ChatwootAPIToken   string
	TypebotBaseURL     string
	TypebotDefaultSlug string
}

var Env *AppConfig

func LoadConfig() {
	_ = godotenv.Load()
	Env = &AppConfig{
		ServerPort:         getEnv("SERVER_PORT", "8080"),
		GlobalAPIKey:       getEnv("GLOBAL_API_KEY", "bob_global_secret_key"),
		DatabaseDSN:        getEnv("DATABASE_DSN", "postgres://bob:bob123@localhost:5432/bob_db?sslmode=disable"),
		N8NWebhookURL:      getEnv("N8N_WEBHOOK_URL", ""),
		ChatwootBaseURL:    getEnv("CHATWOOT_BASE_URL", ""),
		ChatwootAPIToken:   getEnv("CHATWOOT_API_ACCESS_TOKEN", ""),
		TypebotBaseURL:     getEnv("TYPEBOT_BASE_URL", "https://typebot.io"),
		TypebotDefaultSlug: getEnv("TYPEBOT_DEFAULT_SLUG", ""),
	}
	log.Println("[CONFIG] Configuração da API WhatsApp BOB carregada.")
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists && value != "" {
		return value
	}
	return fallback
}
