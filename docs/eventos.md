# Eventos: webhook e WebSocket

Cada instância pode ter um webhook. A BOB faz um `POST` com JSON para cada evento:

```json
{
  "event": "Message",
  "instanceId": "058b625f-2d11-4503-af31-2b5473739c84",
  "instanceName": "vendas",
  "instanceToken": "266A8B9D6D1770BA6F5EA628236C1335",
  "date_time": "2026-11-07T12:20:05-03:00",
  "data": { }
}
```

- Os eventos de cada destino saem **em ordem**, um de cada vez.
- Se o destino falhar (erro de rede ou HTTP 5xx), a BOB tenta de novo depois de 1 s, 3 s e 9 s.
- Defina o webhook em `/instance/connect` (`webhookUrl`), em `/instance/{id}/webhook` ou no painel
  (**Configurar › Webhook**). Sem webhook próprio, vale o `WEBHOOK_URL` do `.env`.

## Categorias para `subscribe`

Lista vazia ou `ALL` envia tudo.

| Categoria | Eventos |
|---|---|
| `MESSAGE` | `Message`, `PollVote` (voto de enquete já com o nome das opções, veja [historico.md](historico.md)), `UndecryptableMessage`, `MediaRetry` |
| `SEND_MESSAGE` | `SendMessage` (mensagem enviada pela API) |
| `READ_RECEIPT` | `Receipt` (`Type`: vazio = entregue, `read` = lida, `played` = ouvida) |
| `CONNECTION` | `Connected`, `PairSuccess`, `LoggedOut`, `Disconnected`, `ConnectFailure`, `TemporaryBan`, `StreamReplaced`... |
| `QRCODE` | `QRCode` (`data.qrcode` é a imagem PNG em data URI), `QRTimeout`, `QRSuccess` |
| `PRESENCE` / `CHAT_PRESENCE` | online/offline e "digitando..." |
| `CALL` | `CallOffer`, `CallAccept`, `CallTerminate`... |
| `CONTACT` | `Contact`, `PushName`, `Picture`, `BusinessName` |
| `GROUP` | `GroupInfo`, `JoinedGroup` |
| `LABEL` | `LabelEdit`, `LabelAssociationChat`, `LabelAssociationMessage` |
| `CHAT` | `Archive`, `Pin`, `Mute`, `Star`, `DeleteChat`... |
| `HISTORY_SYNC` | resumo da sincronização do histórico |
| `NEWSLETTER`, `BLOCKLIST` | canais e lista de bloqueio |

## Mensagem recebida (`Message`)

```json
{
  "Info": {
    "Chat": "5511999998888@s.whatsapp.net", "Sender": "5511999998888@s.whatsapp.net",
    "IsFromMe": false, "IsGroup": false, "ID": "3EB0C1A2B3", "PushName": "Cliente",
    "Timestamp": "2026-11-07T12:20:05-03:00", "Type": "text"
  },
  "Message": { "conversation": "Olá!" }
}
```

Mídia: o nó é `imageMessage`, `videoMessage`, `audioMessage`, `documentMessage` ou `stickerMessage`. Com
`WEBHOOK_FILES=true`, o arquivo vem pronto em `Message.base64` e o tipo em `Message.mimetype`. Sem isso, use
`POST /message/downloadmedia` com o campo `Message` recebido.

Quando o contato usa endereço oculto (`@lid`), o número aparece em `Info.SenderAlt` ou `Info.RecipientAlt`.

## WebSocket

```
ws://SEU-SERVIDOR/ws?token=TOKEN_DA_INSTANCIA          eventos de uma instância
ws://SEU-SERVIDOR/ws?apikey=CHAVE_GLOBAL               eventos de todas
ws://SEU-SERVIDOR/ws?apikey=CHAVE_GLOBAL&instance=vendas
```

Cada mensagem do WebSocket é o mesmo JSON do webhook. O painel usa isso na tela **Eventos ao vivo**.
