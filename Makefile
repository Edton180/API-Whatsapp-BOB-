# API WhatsApp BOB - Desenvolvido por Edton Braz.
.PHONY: help run build test docs docker up down

VERSION := $(shell cat VERSION)
LDFLAGS := -ldflags "-s -w -X github.com/edton180/api-whatsapp-bob/internal/version.Version=$(VERSION)"

help: ## Mostra os comandos
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*## "}{printf "  make %-8s %s\n",$$1,$$2}'

run: ## Roda a API (lê o .env)
	CGO_ENABLED=1 go run $(LDFLAGS) ./cmd/bob

build: ## Compila em build/bob
	CGO_ENABLED=1 go build $(LDFLAGS) -o build/bob ./cmd/bob

test: ## Verifica e testa
	go vet ./...
	go test -race -count=1 ./...

docs: ## Gera a especificação OpenAPI
	python3 scripts/gerar-openapi.py

docker: ## Gera a imagem Docker
	docker build --build-arg VERSION=$(VERSION) -t api-whatsapp-bob:$(VERSION) .

up: ## Sobe com Docker Compose (BOB + PostgreSQL)
	docker compose up -d --build

down: ## Para o Docker Compose
	docker compose down
