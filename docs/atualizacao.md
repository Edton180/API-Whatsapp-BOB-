# Atualização

## Da 1.x para a 2.0

A 2.0 troca o motor por um motor próprio da BOB, escrito sobre a biblioteca whatsmeow. Nada precisa ser
refeito:

- **Números conectados continuam conectados.** A 2.0 lê as mesmas tabelas: `instances` (instâncias, tokens,
  webhooks e configurações) e as tabelas de sessão do WhatsApp. Não é preciso ler o QR Code de novo.
- **Mesmas rotas e formatos.** `/instance/*`, `/send/*`, `/message/*`, `/user/*`, `/chat/*`, `/label/*`,
  `/group/*`, `/chatwoot/*`, `/typebot/*` e `/n8n/*` respondem como antes, e o webhook mantém o formato
  `{event, instanceId, instanceName, instanceToken, data}`.
- **Licença:** não existe mais ativação. `/license/status` responde `active` só para sistemas antigos.
  `EVOLUTION_OPERATOR_EMAIL` pode ser apagado do `.env`.
- **Variáveis:** as mesmas (`GLOBAL_API_KEY`, `POSTGRES_AUTH_DB`, `POSTGRES_USERS_DB`, `SERVER_PORT`...).
  RabbitMQ, NATS e MinIO não fazem parte da 2.0: os eventos saem por webhook e WebSocket, e a mídia recebida vai
  em base64 no webhook.
- **Painel:** novo, em `/painel/`.

Pelo OmniFlow, use o botão de atualização do menu API WhatsApp BOB (ou `sudo bash scripts/whatsapp-bob.sh
atualizar`). Se a nova versão não subir, o script volta para a anterior sozinho.

Com Docker Compose, rode `git pull && docker compose up -d --build`.
