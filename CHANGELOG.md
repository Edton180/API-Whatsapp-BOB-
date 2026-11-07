# Histórico de versões

## 2.0.0 - 2026-11-07

A BOB passa a ter motor, painel e documentação próprios.

- **Motor próprio** escrito do zero sobre a biblioteca [whatsmeow](https://github.com/tulir/whatsmeow) (MPL 2.0). Nenhum código de outra API de WhatsApp.
- **Sem licença para ativar**: as rotas `/license/*` continuam existindo só por compatibilidade e respondem `active`.
- **Painel web** em `/painel/`: instâncias, QR Code, código de pareamento, webhook e eventos, comportamento (recusar ligação, marcar como lida, sempre online, ignorar grupos/status), proxy, integrações (Chatwoot, Typebot, N8N), registro, envio de teste, eventos ao vivo, dados do servidor e Pix.
- **Documentação** própria em `/swagger/index.html` e especificação em `/openapi.yaml` e `/openapi.json`.
- Mesmas rotas e formatos das versões 1.x, então sistemas já integrados (como o OmniFlow) continuam funcionando sem mudança.
- Atualização sem perder nada: usa as mesmas tabelas (`instances` e as sessões do WhatsApp), então números conectados continuam conectados depois de atualizar.
- Webhook com fila por destino (mantém a ordem) e até 4 tentativas; WebSocket em `/ws`.
- Banco PostgreSQL ou SQLite (sem configurar nada).
- Novas rotas: `/server/info`, `/instance/{id}/webhook`, `/message/downloadmedia`, `/chat/read`, `/chat/unread`, `/label/message`, `/group/join`, `/user/privacy`, `/newsletter/list`.
- Dados pessoais do criador removidos (a cidade saiu também do Pix).

## 1.x

Versões baseadas em outro motor, substituídas pela 2.0.
