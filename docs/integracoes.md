# Integrações

## Como funciona hoje

A BOB publica os eventos do WhatsApp (mensagens, conexão, recibos, grupos...) por:

- **Webhook:** `webhookUrl` e `subscribe` em `/instance/connect`.
- **RabbitMQ:** `AMQP_URL`, `AMQP_GLOBAL_ENABLED`, `AMQP_GLOBAL_EVENTS`.
- **NATS:** `NATS_URL`, `NATS_GLOBAL_ENABLED`.
- **WebSocket:** `websocketEnable` em `/instance/connect`.

## N8N

1. Crie um fluxo com o nó **Webhook** (método POST) e copie a URL de produção.
2. Use essa URL em `webhookUrl` ao conectar a instância.
3. Para responder, use o nó **HTTP Request** chamando `POST /send/text` com o cabeçalho `apikey` da instância.

## Typebot

Hoje: um fluxo no N8N recebe a mensagem da BOB, chama a API do Typebot (`/api/v1/typebots/{id}/startChat` e `/continueChat`) e devolve a resposta por `/send/text`.

## Chatwoot

Hoje: crie no Chatwoot uma caixa de entrada do tipo **API**, aponte o webhook da BOB para um fluxo N8N que cria a conversa no Chatwoot, e configure o webhook da caixa do Chatwoot para chamar `/send/text` da BOB. No OmniFlow, o atendimento já é feito direto pela plataforma.

## Roteiro: integrações nativas

As rotas nativas no estilo da Evolution API (`/chatwoot/set`, `/typebot/set`, `/n8n/set`, com sessão de bot e importação de mensagens) estão no roteiro do projeto. Elas serão escritas em Go, ligadas ao motor de eventos real, com testes, e só serão publicadas quando funcionarem de ponta a ponta. Contribuições são bem-vindas, veja o [CONTRIBUTING.md](../CONTRIBUTING.md).
