# Membros de grupos

Três funções para trabalhar com os membros dos grupos em que o número participa:

1. **Extrair** a lista de membros (número, nome quando há, administrador), em JSON ou planilha CSV.
2. **Copiar** os membros de um grupo para outro.
3. **Enviar** uma mensagem no privado de cada membro de um ou mais grupos.

Todas usam o token da instância (ou a chave global com o cabeçalho `instance`).

> **Use com responsabilidade.** Adicionar pessoas em grupos e enviar mensagens a quem não pediu contato é o que
> mais leva o WhatsApp a bloquear um número, e pode ferir a LGPD. Envie só para quem tem relação com a sua empresa,
> mantenha as pausas e comece com poucos contatos. A BOB aplica pausas mínimas e um limite de 1.000 membros por
> tarefa, mas não garante que o número não seja bloqueado.

## 1. Extrair membros

```bash
curl -X POST http://localhost:4000/group/members -H "apikey: TOKEN" -H "Content-Type: application/json" \
  -d '{"groupJid":"120363000000000000@g.us"}'
```

Resposta (`data`): `groupJid`, `name`, `total`, `withNumber` e `members[]` com `jid`, `number`, `lid`, `name`,
`isAdmin` e `isSuperAdmin`.

- `number` vem vazio quando o WhatsApp só informa o identificador oculto da pessoa (LID). Isso acontece em grupos
  grandes ou quando o número nunca conversou com ela.
- Com `"format":"csv"` a resposta é o arquivo `membros.csv` (`numero,jid,nome,administrador`).
- Os ids dos grupos vêm de `GET /group/myall`.

## 2. Copiar membros para outro grupo

```bash
curl -X POST http://localhost:4000/group/members/copy -H "apikey: TOKEN" -H "Content-Type: application/json" \
  -d '{"fromGroupJid":"120363000000000000@g.us","toGroupJid":"120363000000000001@g.us","batch":5,"delay":15}'
```

| Campo | Padrão | O que faz |
|---|---|---|
| `fromGroupJid`, `toGroupJid` | obrigatórios | Grupo de origem e de destino. O número precisa ser **administrador do destino** |
| `skipAdmins` | `false` | Não copia os administradores da origem |
| `batch` | `5` | Quantos adiciona por vez (1 a 10) |
| `delay` | `15` | Segundos entre os lotes (mínimo 5) |
| `limit` | `0` | Máximo de membros (0 = todos, até 1.000) |

Quem já está no destino e o próprio número ficam de fora. O resultado por pessoa aparece na tarefa: `ok`,
`skipped` (já estava no grupo) ou `failed` com o motivo, por exemplo "a pessoa só aceita entrar em grupos por
convite (privacidade)". Para essas, envie o link de convite (`POST /group/invitelink`).

## 3. Enviar para os membros

```bash
curl -X POST http://localhost:4000/group/members/send -H "apikey: TOKEN" -H "Content-Type: application/json" \
  -d '{"groupJids":["120363000000000000@g.us"],"text":"Olá! Temos novidades.","delay":8}'
```

| Campo | Padrão | O que faz |
|---|---|---|
| `groupJid` ou `groupJids` | obrigatório | Um ou vários grupos. Quem está em mais de um recebe uma vez só |
| `text` | | Texto da mensagem (ou legenda da mídia) |
| `url` ou `base64`, `type`, `filename` | | Mídia opcional (imagem, vídeo, áudio, documento) |
| `skipAdmins` | `false` | Não envia aos administradores |
| `delay` | `8` | Segundos entre as mensagens (mínimo 3) |
| `limit` | `0` | Máximo de membros (0 = todos, até 1.000) |

A mensagem vai para a conversa individual de cada membro, não para o grupo.

## Acompanhar e cancelar

Copiar e enviar rodam em segundo plano. A resposta traz a tarefa, com o `id`:

| Rota | O que faz |
|---|---|
| `GET /group/jobs` | Tarefas da instância |
| `GET /group/job/{id}` | Progresso (`processed`, `ok`, `failed`, `skipped`) e o resultado por membro (`items`) |
| `POST /group/job/{id}/cancel` | Para a tarefa. O que já foi feito fica feito |

`status` é `running`, `done`, `cancelled` ou `failed` (por exemplo, se o número desconectar). Ao terminar, a BOB
manda o evento `GroupJob` no webhook e no WebSocket. As tarefas ficam na memória: reiniciar a BOB interrompe as que
estão em andamento, e as terminadas somem depois de 6 horas.

## Listas de números e mais ferramentas

Para extrair vários grupos de uma vez, ver quem está ativo, conferir números e links de convite, adicionar
uma lista qualquer a um grupo com link de convite para quem recusa, ou pausar e retomar uma tarefa, veja
[docs/ferramentas.md](ferramentas.md).
