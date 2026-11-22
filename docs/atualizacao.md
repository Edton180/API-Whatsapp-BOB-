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
  Da 2.5.0 em diante o uso comercial requer licença comercial do autor ([LICENCA-COMERCIAL.md](../LICENCA-COMERCIAL.md)).
  `EVOLUTION_OPERATOR_EMAIL` pode ser apagado do `.env`.
- **Variáveis:** as mesmas (`GLOBAL_API_KEY`, `POSTGRES_AUTH_DB`, `POSTGRES_USERS_DB`, `SERVER_PORT`...).
  RabbitMQ, NATS e MinIO não fazem parte da 2.0: os eventos saem por webhook e WebSocket, e a mídia recebida vai
  em base64 no webhook.
- **Painel:** novo, em `/painel/`.

Pelo OmniFlow, use o botão de atualização do menu API WhatsApp BOB (ou `sudo bash scripts/whatsapp-bob.sh
atualizar`). Se a nova versão não subir, o script volta para a anterior sozinho.

Com Docker Compose, rode `git pull && docker compose up -d --build`.

## Comandos para atualizar

**Junto com o OmniFlow** (na pasta do OmniFlow, `~/omniflow` no computador ou `/opt/omniflow` na VPS):

```bash
sudo bash scripts/whatsapp-bob.sh versao      # commit instalado e o mais novo do repositório
sudo bash scripts/whatsapp-bob.sh atualizar   # backup, compila o código novo e volta sozinha se falhar
```

Ou pelo painel do OmniFlow: Super Admin › **API WhatsApp BOB** › Atualizar.

**Sozinha, com Docker Compose** (na pasta onde clonou a BOB):

```bash
git pull && docker compose up -d --build
```

### Repositório privado

O OmniFlow lê o commit mais novo pelo endereço do git (`git ls-remote`), sem depender da API do GitHub (que
responde `HTTP 403` depois de 60 consultas por hora sem token). Com o repositório **público** não precisa de
nada. Com o repositório **privado**, crie um token de leitura (GitHub › Settings › Developer settings ›
Fine-grained tokens, com acesso a `API-Whatsapp-BOB-` e `omniflow-connect-pay`, *Contents: Read-only*) e
grave em `OMNIFLOW_UPDATE_TOKEN` no `.env` do OmniFlow. O `whatsapp-bob.sh atualizar` usa esse token só na
linha de comando: ele não fica gravado no `.git` nem no `.env` da BOB.

Para conferir se a máquina enxerga o repositório (deve mostrar um código e `refs/heads/main`):

```bash
git ls-remote https://github.com/Edton180/API-Whatsapp-BOB-.git refs/heads/main
```

Se o comando pedir usuário e senha, o repositório está privado para quem não tem token.
