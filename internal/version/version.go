// API WhatsApp BOB - Desenvolvido por Edton Braz.

// Package version guarda a identificação da BOB.
package version

import (
	"os"
	"strings"
)

const (
	Name       = "API WhatsApp BOB"
	Creator    = "Edton Braz (@Edton180)"
	Repository = "https://github.com/Edton180/API-Whatsapp-BOB-"
	Engine     = "whatsmeow (go.mau.fi/whatsmeow, MPL 2.0)"
)

// Version pode vir do -ldflags "-X .../version.Version=...", senão do arquivo VERSION.
var Version = ""

func file(name string) string {
	for _, p := range []string{name, "/app/" + name} {
		if b, err := os.ReadFile(p); err == nil {
			if v := strings.TrimSpace(string(b)); v != "" {
				return v
			}
		}
	}
	return ""
}

// Current devolve a versão em execução.
func Current() string {
	if Version != "" && Version != "dev" {
		return Version
	}
	if v := file("VERSION"); v != "" {
		return v
	}
	return "dev"
}

// Commit é o commit compilado na imagem Docker (vazio fora dela).
func Commit() string { return file("COMMIT") }
