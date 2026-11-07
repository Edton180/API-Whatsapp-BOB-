# Histórico de mensagens e conversas

A BOB guarda no banco (PostgreSQL ou SQLite) as mensagens de cada instância:

- as **recebidas** e as **enviadas** (pela API ou pelo celular);
- as que o celular manda na **sincronização** ao conectar (as conversas recentes);
- o **status** de cada mensagem enviada: `SERVER_ACK` (enviada), `DELIVERY_ACK` (entregue),
  `READ` (lida) e `PLAYED` (áudio ouvido). O status só avança, nunca volta;
- **edições** (o texto novo substitui o antigo e a mensagem fica com `edited: true`) e
  **apagar para todos** (`deleted: true`; o conteúdo original continua guardado);
- as **conversas**, com nome, última mensagem, não lidas, arquivada e fixada;
- os **votos das enquetes**, já com o nome das opções escolhidas.

Reações, votos e mensagens internas do protocolo não viram mensagens no histórico.

## Configuração

| Variável | Padrão | O que faz |
|---|---|---|
| `STORE_MESSAGES` | `true` | Liga o histórico. Com `false`, as rotas abaixo respondem 400 |
| `STORE_MESSAGES_DAYS` | `0` | Apaga mensagens com mais de N dias (verificado a cada 6 horas). `0` guarda para sempre |

Excluir a instância apaga também o histórico dela.

## Rotas nativas (token da instância no `apikey`)

| Rota | O que faz |
|---|---|
| `GET /chat/list?limit=100&offset=0` | Conversas, fixadas primeiro e depois as mais recentes |
| `GET /chat/messages?chat=5511...&limit=50&offset=0` | Mensagens da conversa, as mais novas primeiro. Filtros: `before` e `after` (timestamp em segundos), `fromMe=true/false` |
| `GET /message/get?id=3EB0...` | Uma mensagem |
| `GET /message/poll?id=<id da enquete>` | Votos somados: cada opção com a quantidade e quem votou |
| `POST /chat/history` `{"chat": "5511...", "count": 50}` | Pede ao celular mensagens **mais antigas** que a mais antiga guardada. Elas chegam em seguida pelo evento `HistorySync` e entram no histórico |

Cada mensagem vem assim:

```json
{
  "id": "3EB0C431C26A1916E07E",
  "key": {"id": "3EB0C431C26A1916E07E", "remoteJid": "5511999998888@s.whatsapp.net", "fromMe": false},
  "pushName": "Cliente",
  "messageType": "conversation",
  "message": {"conversation": "Olá!"},
  "text": "Olá!",
  "messageTimestamp": 1762550000,
  "status": "DELIVERY_ACK",
  "edited": false,
  "deleted": false
}
```

Em grupos, `key.participant` traz quem mandou.

## Padrão v2

| Rota | Corpo |
|---|---|
| `POST /chat/findMessages/{instância}` | `{"where": {"key": {"remoteJid": "...", "fromMe": false, "id": "..."}}, "page": 1, "offset": 50}`. Responde `{"messages": {"total", "pages", "currentPage", "records"}}`. Como no padrão v2, `offset` é a quantidade por página |
| `POST /chat/findChats/{instância}` | `{"where": {"remoteJid": "..."}}` (opcional). Responde a lista de conversas com `lastMessage` |
| `POST /chat/findStatusMessage/{instância}` | `{"where": {"remoteJid": "...", "id": "..."}}`. Responde `[{keyId, remoteJid, fromMe, status}]` |

## Outras melhorias que usam o histórico

- **Responder citando** (`quoted`) e **baixar mídia** (`/message/downloadmedia` com `id`,
  `/chat/getBase64FromMediaMessage`) funcionam com mensagens antigas, mesmo depois de reiniciar.
- **Arquivar** e **marcar como lida/não lida** usam a última mensagem guardada da conversa quando
  ela não é informada, que é o que o WhatsApp exige para a mudança aparecer no celular.

## Enquetes

Quando alguém vota, a BOB decifra o voto e envia o evento `PollVote` (categoria `MESSAGE`):

```json
{
  "event": "PollVote",
  "data": {
    "pollId": "3EB0...", "chat": "5511999998888@s.whatsapp.net",
    "voter": "5511977776666@s.whatsapp.net", "pushName": "Maria",
    "selectedOptions": ["Tarde"], "timestamp": 1762550000, "messageId": "3EB1..."
  }
}
```

Um voto novo da mesma pessoa substitui o anterior (`selectedOptions` vazio = tirou o voto).
Para saber o nome das opções a enquete precisa estar no histórico, o que acontece com as
enquetes enviadas ou recebidas depois desta versão.
