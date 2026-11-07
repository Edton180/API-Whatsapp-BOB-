# Instalação da API WhatsApp BOB

## Requisitos

- PostgreSQL 14 ou mais novo (dois bancos: autenticação e usuários; a BOB cria se não existirem).
- Docker **ou** Go 1.25 com CGO e `libwebp-dev` / `libjpeg-dev` (para figurinhas).
- Uma chave forte em `GLOBAL_API_KEY`.

## Com Docker

```bash
git clone https://github.com/Edton180/API-Whatsapp-BOB-.git api-whatsapp-bob
cd api-whatsapp-bob
cp .env.example .env
docker build -t api-whatsapp-bob .
docker run -d --name bob --restart unless-stopped --env-file .env -p 8080:8080 \
  -v bob-dados:/app/dbdata api-whatsapp-bob
```

Teste: `curl http://localhost:8080/` mostra nome, versão e origem. O Swagger fica em `/swagger/index.html`.

## Sem Docker (Linux)

```bash
sudo apt install -y build-essential libwebp-dev libjpeg-dev
go build -o bob ./cmd/evolution-go
./bob
```

## Primeiros passos pela API

```bash
# 1. Criar a instância (chave global)
curl -X POST http://localhost:8080/instance/create -H "apikey: $GLOBAL_API_KEY" \
  -H 'Content-Type: application/json' -d '{"name":"loja","token":"TOKEN-DA-INSTANCIA"}'

# 2. Conectar e assinar eventos (token da instância)
curl -X POST http://localhost:8080/instance/connect -H "apikey: TOKEN-DA-INSTANCIA" \
  -H 'Content-Type: application/json' -d '{"webhookUrl":"https://seu-sistema/webhook","subscribe":["MESSAGE","CONNECTION"]}'

# 3. Ler o QR Code
curl http://localhost:8080/instance/qr -H "apikey: TOKEN-DA-INSTANCIA"

# 4. Enviar uma mensagem
curl -X POST http://localhost:8080/send/text -H "apikey: TOKEN-DA-INSTANCIA" \
  -H 'Content-Type: application/json' -d '{"number":"5594999999999","text":"Olá!"}'
```

## Licença da Evolution Go

Na primeira execução o motor pede a ativação gratuita da licença da Evolution Go (`/license/*`). Para ativar sem navegador, defina `EVOLUTION_OPERATOR_EMAIL` com o e-mail já registrado. Esse mecanismo vem do projeto oficial e é mantido sem alteração.

## No OmniFlow

O OmniFlow instala, atualiza e administra a BOB pelo menu **API WhatsApp BOB**. Não é preciso rodar estes comandos à mão.
