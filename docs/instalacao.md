# Instalação

A BOB é um único programa em Go. Ela roda em qualquer VPS ou servidor Linux (Ubuntu, Debian, AlmaLinux), em
computador com Docker (Windows, macOS ou Linux) e também sem Docker. Requisitos mínimos: 1 vCPU, 512 MB de
memória e 1 GB de disco (cada número conectado usa em torno de 20 a 40 MB).

## 1. Docker Compose (recomendado)

Sobe a BOB com PostgreSQL. Precisa do Docker com o plugin Compose.

```bash
git clone https://github.com/Edton180/API-Whatsapp-BOB-.git api-whatsapp-bob
cd api-whatsapp-bob
cp .env.example .env
# edite o .env: GLOBAL_API_KEY e POSTGRES_PASSWORD (use: openssl rand -hex 24)
echo "POSTGRES_PASSWORD=$(openssl rand -hex 16)" >> .env
docker compose up -d --build
```

Abra `http://IP-DO-SERVIDOR:8080/painel/` e entre com a `GLOBAL_API_KEY`. A porta muda com `BOB_PORT` no `.env`.

## 2. Docker sem Compose (SQLite)

Bom para testar ou para poucos números. Os dados ficam no volume `bob-dados`.

```bash
docker build -t api-whatsapp-bob .
docker run -d --name bob --restart unless-stopped \
  -e GLOBAL_API_KEY=troque-esta-chave -p 8080:4000 \
  -v bob-dados:/app/dbdata api-whatsapp-bob
```

## 3. Sem Docker (Linux)

```bash
sudo apt install -y golang-go build-essential libwebp-dev ffmpeg   # Go 1.26 ou mais novo
git clone https://github.com/Edton180/API-Whatsapp-BOB-.git api-whatsapp-bob
cd api-whatsapp-bob
cp .env.example .env && nano .env
make build
./build/bob
```

Para manter no ar depois de reiniciar o servidor, crie `/etc/systemd/system/bob.service`:

```ini
[Unit]
Description=API WhatsApp BOB
After=network.target

[Service]
WorkingDirectory=/opt/api-whatsapp-bob
ExecStart=/opt/api-whatsapp-bob/build/bob
Restart=always
User=www-data

[Install]
WantedBy=multi-user.target
```

E rode `sudo systemctl enable --now bob`. O `ffmpeg` é opcional: sem ele, áudios vão como arquivo e não como
nota de voz.

## 4. Junto com o OmniFlow

No servidor do OmniFlow, rode `sudo bash scripts/whatsapp-bob.sh instalar`. O script baixa a BOB deste
repositório, compila, sobe na porta 4100 e já liga o canal **API WhatsApp BOB**. Para atualizar:
`sudo bash scripts/whatsapp-bob.sh atualizar` (ou o botão no menu API WhatsApp BOB do OmniFlow).

## HTTPS e domínio

Coloque a BOB atrás de um proxy (Nginx, Caddy, Traefik). Exemplo com Nginx (o WebSocket precisa dos cabeçalhos
`Upgrade`):

```nginx
server {
    server_name bob.seudominio.com.br;
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
        client_max_body_size 100m;
    }
}
```

Depois, `sudo certbot --nginx -d bob.seudominio.com.br`.

## Variáveis do `.env`

| Variável | Padrão | Para que serve |
|---|---|---|
| `GLOBAL_API_KEY` | (obrigatória) | Chave global: cria instâncias e entra no painel |
| `SERVER_PORT` | `8080` | Porta HTTP |
| `POSTGRES_AUTH_DB` | vazio | PostgreSQL das sessões do WhatsApp. Vazio usa SQLite (`DATA_DIR/main.db`) |
| `POSTGRES_USERS_DB` | vazio | PostgreSQL das instâncias e integrações. Vazio usa SQLite (`DATA_DIR/bob.db`) |
| `DATA_DIR` | `dbdata` | Pasta dos arquivos SQLite |
| `OS_NAME` | `API WhatsApp BOB` | Nome do aparelho em "Aparelhos conectados" |
| `CONNECT_ON_STARTUP` | `true` | Reconecta sozinho os números ao iniciar |
| `WEBHOOK_FILES` | `true` | Envia a mídia recebida em base64 no webhook |
| `WEBHOOK_URL` | vazio | Webhook padrão para instâncias sem webhook |
| `STORE_MESSAGES` | `true` | Guarda o histórico de mensagens e conversas no banco |
| `STORE_MESSAGES_DAYS` | `0` | Apaga do histórico mensagens com mais de N dias (`0` = nunca) |
| `RECONNECT_IDLE_HOURS` | `6` | Reconecta (sem QR Code) números conectados sem nenhum evento há N horas (`0` = desliga) |
| `EVENT_IGNORE_GROUP` | `false` | Ignora mensagens de grupos em todas as instâncias |
| `EVENT_IGNORE_STATUS` | `true` | Ignora status (stories) |
| `QRCODE_MAX_COUNT` | `5` | QR Codes gerados antes de desistir |
| `CHECK_USER_EXISTS` | `true` | Confirma o número no WhatsApp antes de enviar (corrige o nono dígito) |
| `PANEL_ENABLED` | `true` | Liga o painel em `/painel/` |
| `WADEBUG` | `WARN` | Log da biblioteca do WhatsApp: `DEBUG`, `INFO`, `WARN`, `ERROR` |
| `PROXY_PROTOCOL`, `PROXY_HOST`, `PROXY_PORT`, `PROXY_USERNAME`, `PROXY_PASSWORD` | vazio | Proxy padrão das instâncias |

## Backup

Tudo o que importa está no banco: com PostgreSQL, faça `pg_dump` dos dois bancos; com SQLite, copie a pasta
`DATA_DIR` com a BOB parada. Restaurando o backup, os números voltam conectados sem novo QR Code.
