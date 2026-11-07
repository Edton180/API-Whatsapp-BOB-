# API WhatsApp BOB - Histórico

O histórico do motor herdado fica em [CHANGELOG.md](CHANGELOG.md) (Evolution Go).

## 1.0.0 - 2026-11-07

- Base: Evolution Go 0.7.2 (commit `9337afc`).
- Figurinhas convertidas para WebP 512×512 em `/send/sticker` (URL, base64 ou data URI).
- Sincronização incremental com a Evolution Go oficial (script e workflow diário).
- Painel web e arquivos de marca da Evolution Go removidos (TRADEMARKS.md); página `/` e `/donate/pix`.
- Pix Copia e Cola corrigido (GUI `br.gov.bcb.pix`, tamanhos e CRC16 válidos) e QR Code em `docs/pix-qrcode.png`.
- Removidas as rotas de exemplo que respondiam "enviado" sem enviar nada e o servidor `cmd/bob-server`, que não compilava; restaurado `pkg/config/config.go` do motor.
- README, CONTRIBUTING e documentação em português.
