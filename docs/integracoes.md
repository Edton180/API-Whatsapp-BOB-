# Integrações nativas: Chatwoot, Typebot e N8N

A BOB tem integrações nativas, escritas em Go e ligadas direto ao motor de eventos. As rotas seguem o
estilo da [Evolution API](https://github.com/evolution-foundation/evolution-api), então quem já usa a Evolution
API reaproveita a configuração. É código próprio da BOB: nenhum código TypeScript da Evolution API foi copiado.

Todas as rotas usam o cabeçalho `apikey` com o **token da instância**. A licença gratuita da Evolution Go precisa
estar ativa (o motor bloqueia as rotas antes disso).

| Ação | Rota |
| --- | --- |
| Ligar ou alterar | `POST /{integracao}/set` (ou `POST /{integracao}`) |
| Ver configuração | `GET /{integracao}/find` (ou `GET /{integracao}`) |
| Remover | `DELETE /{integracao}` |
| Encerrar a conversa de um contato com o bot | `POST /typebot/session/clear` ou `/n8n/session/clear` com `{"remoteJid": "55...@s.whatsapp.net"}` |

`{integracao}` é `chatwoot`, `typebot` ou `n8n`. Senhas e tokens voltam mascarados (`********`) no `find`.
Para desligar sem apagar, envie a configuração com `"enabled": false`.

## Chatwoot

Cada mensagem do WhatsApp vira mensagem na caixa de entrada do Chatwoot (contato e conversa são criados
sozinhos), e a resposta do atendente no Chatwoot sai pelo WhatsApp.

1. No Chatwoot, crie uma caixa de entrada do tipo **API** (deixe o webhook em branco por enquanto) e anote o
   número dela (`inboxId`, aparece na URL das configurações da caixa).
2. Copie o token de acesso em **Perfil › Token de acesso** e o número da conta (`accountId`, aparece na URL).
3. Ligue a integração:

```bash
curl -X POST https://SUA-BOB/chatwoot/set -H "apikey: TOKEN_DA_INSTANCIA" -H "Content-Type: application/json" -d '{
  "url": "https://chatwoot.suaempresa.com",
  "accountId": "1",
  "token": "TOKEN_DO_CHATWOOT",
  "inboxId": 3,
  "signMsg": true
}'
```

4. A resposta traz `webhookUrl` (`https://SUA-BOB/chatwoot/webhook/{instancia}/{segredo}`). Cole esse endereço
   no campo **Webhook** da caixa de entrada API do Chatwoot.

| Campo | Padrão | Uso |
| --- | --- | --- |
| `signMsg` | `false` | Coloca o nome do atendente em negrito antes da mensagem |
| `importFromMe` | `true` | Copia para o Chatwoot as mensagens enviadas pelo celular |
| `ignoreGroups` | `true` | Não copia mensagens de grupos |

Notas privadas do Chatwoot não são enviadas ao cliente. Anexos enviados pelo atendente saem como mídia. Quando
a conversa é resolvida, a próxima mensagem do cliente abre uma conversa nova. Se o endereço público da BOB
mudar, chame `GET /chatwoot/find` para ver o `webhookUrl` novo; o segredo continua o mesmo.

## Typebot

O contato conversa com um fluxo do Typebot pelo WhatsApp. A BOB guarda a sessão de cada contato.

```bash
curl -X POST https://SUA-BOB/typebot/set -H "apikey: TOKEN_DA_INSTANCIA" -H "Content-Type: application/json" -d '{
  "url": "https://typebot.suaempresa.com",
  "typebot": "id-publico-do-fluxo",
  "triggerType": "keyword",
  "triggerOperator": "contains",
  "triggerValue": "menu",
  "keywordFinish": "#sair",
  "expire": 30
}'
```

- `url` é o endereço do **visualizador** do Typebot (onde o fluxo é publicado). O id público aparece em
  **Compartilhar** no Typebot. Para fluxos privados, informe `apiToken`.
- A BOB chama `POST /api/v1/typebots/{id}/startChat` e depois `POST /api/v1/sessions/{sessionId}/continueChat`,
  e preenche as variáveis `remoteJid`, `pushName` e `number`, se existirem no fluxo.
- Textos (com negrito, itálico e links), imagens, vídeos e áudios são enviados. Botões e listas do Typebot viram
  opções numeradas, que funcionam em qualquer celular.

## N8N

Cada mensagem é enviada a um fluxo do N8N, e a resposta do fluxo volta ao contato.

```bash
curl -X POST https://SUA-BOB/n8n/set -H "apikey: TOKEN_DA_INSTANCIA" -H "Content-Type: application/json" -d '{
  "webhookUrl": "https://n8n.suaempresa.com/webhook/atendimento",
  "basicAuthUser": "bob",
  "basicAuthPass": "senha"
}'
```

O N8N recebe (mesmo formato da Evolution API):

```json
{ "chatInput": "texto", "sessionId": "55...@s.whatsapp.net", "remoteJid": "55...@s.whatsapp.net",
  "number": "55...", "pushName": "Cliente", "fromMe": false, "messageId": "...", "mediaType": "",
  "instanceName": "bob", "instanceId": "...", "apiKey": "TOKEN_DA_INSTANCIA" }
```

No nó **Respond to Webhook**, devolva `{"output": "resposta"}` (também aceita `text`, `reply`, `answer` ou
`message`, ou uma lista desses objetos). Três quebras de linha seguidas separam a resposta em várias mensagens.
Para mídia, devolva `{"mediaUrl": "https://...", "mediaType": "image", "caption": "legenda"}`. Use `sessionId`
como chave da memória do agente de IA do N8N.

## Regras dos bots (Typebot e N8N)

| Campo | Padrão | Uso |
| --- | --- | --- |
| `triggerType` | `all` | `all` responde tudo, `keyword` só começa com a palavra-chave, `none` desliga o início |
| `triggerOperator` | `contains` | `equals`, `contains` ou `startsWith` |
| `triggerValue` | | Palavra-chave que inicia o bot |
| `keywordFinish` | | Palavra que encerra a conversa com o bot |
| `expire` | `0` | Minutos sem mensagem até a sessão vencer (`0` = nunca) |
| `ignoreGroups` | `true` | Não responde em grupos |
| `listeningFromMe` | `false` | Também envia ao bot as mensagens enviadas pelo celular |
| `delayMessage` | `0` | Pausa em milissegundos entre as mensagens da resposta |

Se o Typebot e o N8N estiverem ligados na mesma instância, o Typebot responde. A integração com o Chatwoot
funciona junto com qualquer um dos dois.

## Outros meios

Os eventos continuam saindo por webhook (`webhookUrl` em `/instance/connect`), RabbitMQ (`AMQP_URL`), NATS
(`NATS_URL`) e WebSocket, como na Evolution Go. As integrações nativas não mudam esses envios.

## Como funciona por dentro

O código fica em `pkg/bob/integrations`. O motor chama `bob_integrations.Dispatch` no início de `CallWebhook`
(uma linha em `pkg/whatsmeow/service/whatsmeow.go`), e o resto roda em goroutines, sem atrasar os eventos. As
configurações ficam nas tabelas `bob_integrations` e `bob_bot_sessions`, criadas sozinhas no banco da BOB. Os
testes (`go test ./pkg/bob/...`) simulam Chatwoot, Typebot e N8N.
