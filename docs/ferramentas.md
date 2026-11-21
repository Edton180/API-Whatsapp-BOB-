# Extração de membros e tarefas em massa

A BOB extrai os participantes dos grupos, confere números e links de convite, e executa listas longas
(adicionar pessoas a um grupo, mandar a mesma mensagem para muitos números, criar vários grupos) um
item por vez, com intervalo entre eles.

> **Antes de usar.** Mandar mensagem para quem não pediu e adicionar desconhecidos em grupos é o
> caminho mais rápido para o WhatsApp banir o número, e no Brasil a LGPD exige base legal para usar
> o telefone de alguém. As rotas abaixo não contornam isso: elas só deixam o ritmo controlado e o
> relatório claro. Use com lista de quem autorizou o contato.

## Ferramentas de extração

Token da instância no `apikey`.

| Rota | O que faz |
|---|---|
| `POST /tools/group/members` | Participantes dos grupos, sem repetir quem está em mais de um |
| `POST /tools/group/active` | Quem escreveu no grupo nos últimos N dias, com a quantidade de mensagens |
| `GET /tools/chat/contacts` | Pessoas com quem a instância já tem conversa |
| `POST /tools/number/filter` | Separa os números que têm WhatsApp dos que não têm |
| `POST /tools/invite/check` | Confere links de convite sem entrar nos grupos |
| `POST /tools/group/add` | Adiciona **uma** pessoa ao grupo e diz o que aconteceu com ela |

### Participantes dos grupos

```http
POST /tools/group/members
{"groups": ["1203...@g.us", "1204...@g.us"], "excludeAdmins": false, "onlyAdmins": false}
```

Sem `groups`, a BOB lê **todos** os grupos da instância (no máximo 100 por chamada). A resposta traz
os grupos lidos e a lista de membros já sem repetições:

```json
{
  "groups": [
    {"jid": "1203...@g.us", "name": "Clientes SP", "total": 128, "hidden": 3, "isCommunity": false}
  ],
  "members": [
    {"phone": "5511999998888", "jid": "5511999998888@s.whatsapp.net", "name": "Maria",
     "admin": false, "groups": ["1203...@g.us"]}
  ],
  "total": 125,
  "hidden": 3
}
```

`hidden` conta os participantes com número oculto (`@lid`): o WhatsApp não entrega o telefone deles,
então não há como salvá-los como contato. Um grupo que não pôde ser lido aparece em `groups` com
`error` e os outros continuam.

### Quem está ativo no grupo

```http
POST /tools/group/active
{"groupJid": "1203...@g.us", "days": 30}
```

Conta as mensagens guardadas no histórico, então depende de `STORE_MESSAGES=true` (veja
[docs/historico.md](historico.md)). Serve para mandar mensagem só para quem participa, em vez da
lista inteira.

### Conferir números e convites

```http
POST /tools/number/filter
{"numbers": ["5511999998888", "5511888887777"]}
```

Até 5.000 números por chamada, em lotes de 50, com um segundo de pausa entre os lotes. Devolve
`valid` e `invalid`.

```http
POST /tools/invite/check
{"links": ["https://chat.whatsapp.com/ABC123"]}
```

Até 50 links. Cada um volta com `valid`, `jid`, `name`, `size`, `isCommunity` e `joinApproval` (se o
grupo exige aprovação do administrador).

### Adicionar uma pessoa

```http
POST /tools/group/add
{"groupJid": "1203...@g.us", "number": "5511999998888",
 "inviteFallback": true, "inviteText": "Oi! Entra no nosso grupo:"}
```

O `status` diz exatamente o que aconteceu:

| `status` | Significa |
|---|---|
| `added` | Entrou no grupo |
| `member` | Já era do grupo |
| `refused` | A pessoa não aceita ser adicionada por quem não é contato |
| `invited` | Recusou a entrada direta, então a BOB mandou o link do convite no privado |
| `failed` | Número sem WhatsApp, bloqueou o número da instância, ou saiu do grupo há pouco |

`inviteFallback: true` liga o envio do link quando a pessoa recusa a entrada direta. Sem ele, o
resultado fica em `refused`.

## Tarefas em massa

Uma tarefa percorre a lista um item por vez e pode ser pausada, retomada e cancelada. Cada tipo tem
intervalo mínimo e tamanho máximo:

| Tipo | Rota | Intervalo mínimo | Máximo de itens |
|---|---|---|---|
| Adicionar ao grupo | `POST /bulk/group/add` | 10 s | 1.000 |
| Enviar mensagem | `POST /bulk/send` | 3 s | 5.000 |
| Criar grupos | `POST /bulk/group/create` | 30 s | 50 |

O intervalo informado em `intervalSeconds` é o ponto de partida: a BOB varia cada espera entre 70% e
130% dele, para o ritmo não ficar mecânico. O máximo é 600 segundos. Números repetidos saem da lista.
Só pode haver **uma** tarefa de cada tipo em andamento por instância.

| Rota | O que faz |
|---|---|
| `GET /bulk/list` | Tarefas da instância, da mais nova para a mais antiga (sem os itens) |
| `GET /bulk/{id}` | Uma tarefa com o relatório item a item |
| `POST /bulk/{id}/pause` | Pausa |
| `POST /bulk/{id}/resume` | Retoma de onde parou |
| `POST /bulk/{id}/cancel` | Cancela (os itens que faltavam ficam `cancelled`) |

### Começar uma tarefa

```http
POST /bulk/group/add
{"groupJid": "1203...@g.us", "numbers": ["5511999998888", "..."],
 "intervalSeconds": 20, "inviteFallback": true, "label": "Clientes de novembro"}
```

```http
POST /bulk/send
{"numbers": ["5511999998888", "..."], "text": "Oi {{numero}}, tudo bem?",
 "intervalSeconds": 8, "label": "Aviso de manutenção"}
```

`{{numero}}` é trocado pelo número de quem recebe. Para mandar arquivo, use `url` (ou `base64`) com
`type`, `filename` e `mimetype`; o arquivo é baixado **uma vez**, antes de a tarefa começar, e o
`text` vira a legenda.

```http
POST /bulk/group/create
{"names": ["Turma A", "Turma B"], "participants": ["5511999998888"],
 "description": "Grupo da turma", "intervalSeconds": 60}
```

### O que a tarefa devolve

```json
{
  "id": "9f2c1b7a4d5e6f80",
  "kind": "groupAdd",
  "status": "running",
  "intervalSeconds": 20,
  "total": 300, "done": 42, "failed": 3, "pending": 255,
  "createdAt": 1763300000,
  "items": [
    {"target": "5511999998888", "status": "done", "detail": "added", "at": 1763300020},
    {"target": "5511888887777", "status": "failed", "detail": "número não tem WhatsApp", "at": 1763300041}
  ]
}
```

`status` da tarefa é `running`, `paused`, `completed` ou `cancelled`. `status` do item é `pending`,
`done`, `failed` ou `cancelled`.

### Pausa automática

Quando o WhatsApp responde `rate-overlimit` (limitou o número) ou a instância cai, a tarefa **pausa
sozinha** e o item **não é gasto**: ele volta para `pending` e o motivo fica em `lastError`. Espere
alguns minutos e chame `POST /bulk/{id}/resume`.

Sair da conta ou excluir a instância cancela as tarefas dela. As tarefas ficam na memória: reiniciar
a BOB encerra as que estavam rodando, então uma lista longa pede que o sistema que chamou a BOB
guarde o próprio progresso.
