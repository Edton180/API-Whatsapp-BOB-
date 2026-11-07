# Compatibilidade com a Evolution API v2

Além das rotas próprias da Evolution Go, a API Bob responde às rotas da
[Evolution API v2](https://doc.evolution-api.com/v2). Sistemas feitos para a
Evolution API (n8n, Chatwoot, Typebot, CRMs) podem apontar para a API Bob sem
mudar o código.

## Autenticação

Envie o header `apikey` com:

- a `GLOBAL_API_KEY`, que acessa qualquer instância; ou
- o token da instância citada na URL.

`:instance` é o **nome** da instância (o id também é aceito).

## Rotas

| Módulo | Método e rota |
| :--- | :--- |
| Instância | `POST /instance/create` (corpo com `instanceName`), `GET /instance/fetchInstances`, `GET /instance/connect/:instance`, `GET /instance/connectionState/:instance`, `POST /instance/restart/:instance`, `POST /instance/setPresence/:instance`, `DELETE /instance/logout/:instance`, `DELETE /instance/delete/:instance` |
| Configurações | `POST /settings/set/:instance`, `GET /settings/find/:instance` |
| Webhook | `POST /webhook/set/:instance`, `GET /webhook/find/:instance` |
| Proxy | `POST /proxy/set/:instance`, `GET /proxy/find/:instance` |
| Mensagens | `POST /message/sendText`, `sendMedia`, `sendPtv`, `sendWhatsAppAudio`, `sendSticker`, `sendLocation`, `sendContact`, `sendReaction`, `sendPoll`, `sendList`, `sendButtons`, `sendStatus` (todas com `/:instance`) |
| Chat | `POST /chat/whatsappNumbers`, `markMessageAsRead`, `markChatUnread`, `archiveChat`, `updateMessage`, `sendPresence`, `updateBlockStatus`, `fetchProfilePictureUrl`, `fetchProfile`, `fetchBusinessProfile`, `findContacts`, `getBase64FromMediaMessage`, `updateProfileName`, `updateProfileStatus`, `updateProfilePicture`, `updatePrivacySettings`; `DELETE /chat/deleteMessageForEveryone`, `removeProfilePicture`; `GET /chat/fetchPrivacySettings` |
| Grupos | `POST /group/create`, `updateGroupSubject`, `updateGroupDescription`, `updateGroupPicture`, `revokeInviteCode`, `sendInvite`, `updateParticipant`, `updateSetting`, `toggleEphemeral`; `GET /group/findGroupInfos`, `fetchAllGroups`, `participants`, `inviteCode`, `inviteInfo`, `acceptInviteCode`; `DELETE /group/leaveGroup` |
| Labels | `GET /label/findLabels/:instance`, `POST /label/handleLabel/:instance` |
| Chamadas | `POST /call/offer/:instance` responde `501`: o protocolo do WhatsApp Web não permite fazer ligações |

As rotas de grupo recebem o grupo em `?groupJid=` como na Evolution API.

## Exemplo

```bash
curl -X POST http://localhost:4000/message/sendText/minha-instancia \
  -H "apikey: $GLOBAL_API_KEY" -H "Content-Type: application/json" \
  -d '{"number":"5511999999999","text":"Olá!","delay":1200}'
```

Resposta (mesmo formato da Evolution API):

```json
{
  "key": {"remoteJid": "5511999999999@s.whatsapp.net", "fromMe": true, "id": "3EB0..."},
  "status": "PENDING",
  "message": {"extendedTextMessage": {"text": "Olá!"}},
  "messageType": "extendedTextMessage",
  "messageTimestamp": 1767225600,
  "instanceId": "…"
}
```

## Diferenças conhecidas

- `POST /webhook/set` aceita os nomes de eventos da Evolution API
  (`MESSAGES_UPSERT`, `CONNECTION_UPDATE`, `GROUP_UPDATE`...) e os converte
  para os eventos do motor. O corpo enviado ao webhook ainda segue o formato da
  Evolution Go; o formato da Evolution API está planejado.
- `webhookByEvents`, `webhookBase64`, `readStatus` e `syncFullHistory` ainda
  não são suportados e voltam como `false`.
- `getBase64FromMediaMessage` precisa da mensagem completa recebida no webhook
  (`message.message`), porque o motor ainda não guarda o conteúdo das mensagens.
- `findMessages`, `findChats` e `findStatusMessage` dependem desse histórico e
  ainda não existem.
