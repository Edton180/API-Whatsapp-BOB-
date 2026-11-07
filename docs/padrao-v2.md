# Rotas no padrão v2 (`/<módulo>/<ação>/{instância}`)

Muitos sistemas de WhatsApp (n8n, CRMs, chatbots) foram feitos para o formato de rotas da
Evolution API v2. A BOB responde nesse mesmo formato, com o **nome da instância na URL**,
usando o motor próprio dela. Para migrar, basta trocar o endereço do servidor e a chave.

Nada disso depende de outra API: é só o formato das rotas e dos JSON.

## Autenticação

Cabeçalho `apikey` com a `GLOBAL_API_KEY` ou com o token da instância citada na URL.
`{instância}` pode ser o nome ou o id.

## Criar e conectar

```bash
# cria (com webhook já ligado) e já pede o QR Code
curl -X POST http://localhost:8080/instance/create -H "apikey: CHAVE_GLOBAL" -H "Content-Type: application/json" \
  -d '{"instanceName":"vendas","qrcode":true,"webhook":{"url":"https://seu-sistema/webhook","events":["MESSAGES_UPSERT","CONNECTION_UPDATE"]}}'

# QR Code de novo, ou código de pareamento com ?number=
curl http://localhost:8080/instance/connect/vendas -H "apikey: CHAVE_GLOBAL"
curl "http://localhost:8080/instance/connect/vendas?number=5511999998888" -H "apikey: CHAVE_GLOBAL"

# estado: open, connecting ou close
curl http://localhost:8080/instance/connectionState/vendas -H "apikey: CHAVE_GLOBAL"
```

`POST /instance/create` atende os dois formatos: com `instanceName` usa o padrão v2; com `name`,
o formato nativo da BOB.

## Enviar

```bash
curl -X POST http://localhost:8080/message/sendText/vendas -H "apikey: CHAVE_GLOBAL" \
  -H "Content-Type: application/json" -d '{"number":"5511999998888","text":"Olá!","delay":1200}'
```

Resposta (HTTP 201):

```json
{
  "key": {"remoteJid": "5511999998888@s.whatsapp.net", "fromMe": true, "id": "3EB0..."},
  "status": "PENDING",
  "message": {"conversation": "Olá!"},
  "messageType": "conversation",
  "messageTimestamp": 1767225600,
  "instanceId": "…",
  "source": "api"
}
```

Erros vêm como `{"status": 400, "error": "Bad Request", "response": {"message": ["..."]}}`.

## Rotas

| Módulo | Rotas |
|---|---|
| Instância | `POST /instance/create`, `GET /instance/fetchInstances`, `GET /instance/connect/{i}`, `GET /instance/connectionState/{i}`, `POST /instance/restart/{i}`, `POST /instance/setPresence/{i}`, `DELETE /instance/logout/{i}`, `DELETE /instance/delete/{i}` |
| Configuração | `POST /settings/set/{i}`, `GET /settings/find/{i}`, `POST /webhook/set/{i}`, `GET /webhook/find/{i}`, `POST /proxy/set/{i}`, `GET /proxy/find/{i}` |
| Envio | `POST /message/` + `sendText`, `sendMedia`, `sendPtv`, `sendWhatsAppAudio`, `sendSticker`, `sendLocation`, `sendContact`, `sendReaction`, `sendPoll`, `sendList`, `sendButtons`, `sendStatus` |
| Conversas | `POST /chat/` + `whatsappNumbers`, `markMessageAsRead`, `markChatUnread`, `archiveChat`, `updateMessage`, `sendPresence`, `updateBlockStatus`, `fetchProfilePictureUrl`, `fetchProfile`, `fetchBusinessProfile`, `findContacts`, `findMessages`, `findChats`, `findStatusMessage`, `getBase64FromMediaMessage`, `updateProfileName`, `updateProfileStatus`, `updateProfilePicture`, `updatePrivacySettings`; `DELETE /chat/deleteMessageForEveryone`, `/chat/removeProfilePicture`; `GET /chat/fetchPrivacySettings` |
| Grupos | `POST /group/create`, `updateGroupSubject`, `updateGroupDescription`, `updateGroupPicture`, `revokeInviteCode`, `sendInvite`, `updateParticipant`, `updateSetting`, `toggleEphemeral`; `GET /group/findGroupInfos`, `fetchAllGroups`, `participants`, `inviteCode`, `inviteInfo`, `acceptInviteCode`; `DELETE /group/leaveGroup` |
| Etiquetas | `GET /label/findLabels/{i}`, `POST /label/handleLabel/{i}` |
| Chamadas | `POST /call/offer/{i}` responde 501: o WhatsApp Web não faz ligações |

Nas rotas de grupo, o grupo vai em `?groupJid=`, e o convite em `?inviteCode=`.

## Eventos do webhook

`/webhook/set` aceita os nomes `MESSAGES_UPSERT`, `MESSAGES_UPDATE`, `SEND_MESSAGE`,
`CONNECTION_UPDATE`, `QRCODE_UPDATED`, `PRESENCE_UPDATE`, `CONTACTS_UPSERT`, `GROUPS_UPSERT`,
`GROUP_UPDATE`, `GROUP_PARTICIPANTS_UPDATE`, `LABELS_EDIT`, `LABELS_ASSOCIATION`, `CALL`,
`CHATS_UPDATE`, `MESSAGES_SET`, `CHATS_SET` (ou `ALL`) e converte para as categorias da BOB
(veja [eventos.md](eventos.md)). O corpo enviado ao webhook segue o formato da BOB.

## Limites conhecidos

- `readStatus`, `syncFullHistory`, `webhookByEvents` e `webhookBase64` ainda não existem e
  voltam `false`. Para mídia em base64 no webhook use `WEBHOOK_FILES=true`.
- `getBase64FromMediaMessage` precisa do conteúdo da mensagem (`message.message`, como veio no
  webhook) ou do id de uma mensagem do histórico.
- `findMessages`, `findChats` e `findStatusMessage` leem o histórico guardado no banco (veja
  [historico.md](historico.md)); só existem as mensagens recebidas, enviadas ou sincronizadas
  depois que a instância conectou nesta versão.
