// API WhatsApp BOB - Desenvolvido por Edton Braz.

// Package config lê a configuração da BOB das variáveis de ambiente (e do arquivo .env).
package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Proxy de saída padrão das instâncias (pode ser trocado por instância).
type Proxy struct {
	Protocol string
	Host     string
	Port     string
	Username string
	Password string
}

// Config reúne tudo o que a BOB precisa para subir.
type Config struct {
	Port             string
	APIKey           string
	AuthDB           string // banco das sessões do WhatsApp (PostgreSQL); vazio usa SQLite em DataDir
	UsersDB          string // banco das instâncias e integrações (PostgreSQL); vazio usa SQLite em DataDir
	DataDir          string
	ClientName       string
	OSName           string
	LogLevel         string
	ConnectOnStartup bool
	WebhookFiles     bool
	GlobalWebhook    string
	IgnoreGroups     bool
	IgnoreStatus     bool
	QRMaxCount       int
	CheckUserExists  bool
	PanelEnabled     bool
	PublicURL        string
	Proxy            Proxy
}

// Load lê o .env (se houver) e as variáveis de ambiente.
func Load() *Config {
	loadDotEnv(".env")
	data := env("DATA_DIR", "dbdata")
	if !filepath.IsAbs(data) {
		if wd, err := os.Getwd(); err == nil {
			data = filepath.Join(wd, data)
		}
	}
	return &Config{
		Port:             env("SERVER_PORT", "8080"),
		APIKey:           env("GLOBAL_API_KEY", ""),
		AuthDB:           env("POSTGRES_AUTH_DB", ""),
		UsersDB:          env("POSTGRES_USERS_DB", ""),
		DataDir:          data,
		ClientName:       env("CLIENT_NAME", "bob"),
		OSName:           env("OS_NAME", "API WhatsApp BOB"),
		LogLevel:         strings.ToUpper(env("WADEBUG", env("LOG_LEVEL", "WARN"))),
		ConnectOnStartup: envBool("CONNECT_ON_STARTUP", true),
		WebhookFiles:     envBool("WEBHOOK_FILES", true),
		GlobalWebhook:    env("WEBHOOK_URL", ""),
		IgnoreGroups:     envBool("EVENT_IGNORE_GROUP", false),
		IgnoreStatus:     envBool("EVENT_IGNORE_STATUS", true),
		QRMaxCount:       envInt("QRCODE_MAX_COUNT", 5),
		CheckUserExists:  envBool("CHECK_USER_EXISTS", true),
		PanelEnabled:     envBool("PANEL_ENABLED", true),
		PublicURL:        strings.TrimRight(env("PUBLIC_URL", ""), "/"),
		Proxy: Proxy{
			Protocol: env("PROXY_PROTOCOL", ""),
			Host:     env("PROXY_HOST", ""),
			Port:     env("PROXY_PORT", ""),
			Username: env("PROXY_USERNAME", ""),
			Password: env("PROXY_PASSWORD", ""),
		},
	}
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func envBool(key string, def bool) bool {
	v := strings.ToLower(env(key, ""))
	switch v {
	case "1", "true", "yes", "sim", "on":
		return true
	case "0", "false", "no", "nao", "não", "off":
		return false
	}
	return def
}

func envInt(key string, def int) int {
	if n, err := strconv.Atoi(env(key, "")); err == nil {
		return n
	}
	return def
}

// loadDotEnv lê CHAVE=valor sem sobrescrever o que já veio do ambiente.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if _, exists := os.LookupEnv(k); !exists {
			os.Setenv(k, v)
		}
	}
}
