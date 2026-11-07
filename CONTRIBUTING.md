# Como contribuir com a API WhatsApp BOB

A BOB é livre e aberta. Toda ajuda conta: correção de bug, nova função, teste, documentação ou tradução.

## Passo a passo

1. Abra uma [issue](https://github.com/Edton180/API-Whatsapp-BOB-/issues) descrevendo o bug ou a ideia.
2. Faça um fork e crie um ramo: `feat/nome-da-funcao`, `fix/descricao`, `docs/assunto`.
3. Escreva o código em Go com `gofmt` e um teste para o que você mudou.
4. Rode `make test` (ou `go vet ./... && go test ./...`).
5. Se mudar rotas, atualize `scripts/gerar-openapi.py` e rode `python3 scripts/gerar-openapi.py`.
6. Use commits no padrão *Conventional Commits* (`feat:`, `fix:`, `docs:`...).
7. Envie o pull request para a `main`.

## Onde fica cada coisa

| Pasta | O que tem |
|---|---|
| `cmd/bob` | Início do servidor |
| `internal/engine` | Motor: sessões do WhatsApp (whatsmeow), eventos, envios e ações |
| `internal/api` | Rotas HTTP |
| `internal/events` | Webhook (com fila e novas tentativas) e WebSocket |
| `internal/integrations` | Chatwoot, Typebot e N8N |
| `internal/panel` | Painel web, documentação e OpenAPI (embutidos no binário) |
| `internal/store` | Banco (PostgreSQL ou SQLite) |
| `internal/media` | Figurinhas, áudio (ffmpeg), miniaturas e leitura de arquivos |

## Regras do projeto

- **Licença:** contribuições entram sob a Apache 2.0 ([LICENSE](LICENSE)).
- **Código de terceiros:** não copie código de outros projetos de API de WhatsApp. Use a biblioteca whatsmeow e escreva a função na BOB.
- **Nada de rota de mentira:** uma rota só entra se fizer o que diz (por exemplo, enviar a mensagem de verdade).
- **Compatibilidade:** não mude nomes de rotas ou de campos já publicados. Se precisar, crie um campo novo.

## Apoie

Se a BOB ajuda o seu trabalho, considere uma doação por Pix (veja o [README](README.md#apoie-o-projeto-pix)).
