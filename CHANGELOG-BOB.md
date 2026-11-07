# API WhatsApp BOB - Histórico

O histórico do motor herdado fica em [CHANGELOG.md](CHANGELOG.md) (Evolution Go).

## 1.1.0 - 2026-11-07

- Integrações nativas com Chatwoot, Typebot e N8N (`pkg/bob/integrations`), com as rotas no estilo da Evolution API: `/chatwoot/set`, `/typebot/set`, `/n8n/set`, `/find`, `DELETE` e `/session/clear`.
- Chatwoot: contato e conversa criados sozinhos, mensagens do celular importadas, resposta do atendente (com assinatura opcional e anexos) enviada ao WhatsApp por `/chatwoot/webhook/{instancia}/{segredo}`, sem eco.
- Typebot: sessão por contato, palavra-chave de início e de saída, expiração, texto formatado, mídia e opções numeradas.
- N8N: mesmo corpo enviado pela Evolution API, autenticação básica, resposta em uma ou várias mensagens e mídia.
- Tabelas `bob_integrations` e `bob_bot_sessions` criadas sozinhas; testes em `go test ./pkg/bob/...`.

## 1.0.1 - 2026-11-07

- A página `/` informa o commit compilado (`commit`) e o commit da Evolution Go contido (`upstream_commit`), para o OmniFlow saber se há atualização.

## 1.0.0 - 2026-11-07

- Base: Evolution Go 0.7.2 (commit `9337afc`).
- Figurinhas convertidas para WebP 512×512 em `/send/sticker` (URL, base64 ou data URI).
- Sincronização incremental com a Evolution Go oficial (script e workflow diário).
- Painel web e arquivos de marca da Evolution Go removidos (TRADEMARKS.md); página `/` e `/donate/pix`.
- Pix Copia e Cola corrigido (GUI `br.gov.bcb.pix`, tamanhos e CRC16 válidos) e QR Code em `docs/pix-qrcode.png`.
- Removidas as rotas de exemplo que respondiam "enviado" sem enviar nada e o servidor `cmd/bob-server`, que não compilava; restaurado `pkg/config/config.go` do motor.
- README, CONTRIBUTING e documentação em português.
