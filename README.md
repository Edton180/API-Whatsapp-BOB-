<div align="center">

# API WhatsApp BOB

**API de WhatsApp livre, rápida e leve, escrita em Go e baseada na [Evolution Go](https://github.com/evolution-foundation/evolution-go).**

[![CI](https://github.com/Edton180/API-Whatsapp-BOB-/actions/workflows/ci.yml/badge.svg)](https://github.com/Edton180/API-Whatsapp-BOB-/actions/workflows/ci.yml)
[![Licença](https://img.shields.io/badge/licen%C3%A7a-Apache%202.0%20%2B%20condi%C3%A7%C3%B5es-blue)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)](go.mod)
[![Contribua](https://img.shields.io/badge/contribui%C3%A7%C3%B5es-bem--vindas-brightgreen)](CONTRIBUTING.md)

Criada e mantida por **Edton Kaik Braz Da Conceição** ([@Edton180](https://github.com/Edton180)), Tucuruí (PA).

</div>

---

## O que é

A API WhatsApp BOB é uma distribuição modificada da **Evolution Go** (Evolution Foundation), o servidor de WhatsApp em Go sobre a biblioteca [whatsmeow](https://github.com/tulir/whatsmeow). Ela mantém todo o motor da Evolution Go e acrescenta melhorias próprias, sempre podendo receber as atualizações do projeto oficial.

Pode ser usada por qualquer sistema (CRM, ERP, SaaS, automações) e é o motor de WhatsApp do **OmniFlow**.

> A BOB não é afiliada à Evolution Foundation nem endossada por ela. "Evolution", "Evolution Go" e "Evolution Foundation" são marcas da Evolution Foundation, citadas aqui só para indicar a origem do código (veja [TRADEMARKS.md](TRADEMARKS.md)).

## Projetos oficiais de origem

| Projeto | Papel na BOB | Licença |
| --- | --- | --- |
| [evolution-foundation/evolution-go](https://github.com/evolution-foundation/evolution-go) | Código-base (motor Go). Commit sincronizado em [`UPSTREAM`](UPSTREAM), versão em [`UPSTREAM_VERSION`](UPSTREAM_VERSION). | Apache 2.0 + condições ([LICENSE](LICENSE)) |
| [evolution-foundation/evolution-api](https://github.com/evolution-foundation/evolution-api) | Referência de funções e integrações (Chatwoot, Typebot, N8N...). Nenhum código TypeScript é copiado; as funções são reimplementadas em Go. | Apache 2.0 + condições |
| [tulir/whatsmeow](https://github.com/tulir/whatsmeow) | Protocolo do WhatsApp (dependência da Evolution Go). | MPL 2.0 |

## O que a BOB acrescenta

- **Figurinhas de verdade:** `/send/sticker` aceita URL, base64 ou data URI em PNG, JPG, GIF ou WebP e converte para WebP 512×512 (até 500 KB), como o WhatsApp exige. WebP animado passa direto.
- **Sincronização incremental com a Evolution Go:** só as partes que mudaram no projeto oficial são aplicadas, sem perder as melhorias da BOB ([docs/sincronizacao.md](docs/sincronizacao.md)).
- **Imagem Docker enxuta e com cache de compilação**, healthcheck e arquivos de licença dentro da imagem.
- **Sem o painel web da Evolution Go:** a marca do painel é protegida, então a administração é feita pela API, pelo Swagger ou pelo menu *API WhatsApp BOB* do OmniFlow.
- Página `/` com versão, criador e origem, e `/donate/pix` com os dados de doação.

## Funções disponíveis (motor Evolution Go)

| Grupo | Rotas |
| --- | --- |
| Instâncias | `/instance/create`, `/all`, `/info`, `/delete`, `/connect`, `/qr`, `/pair`, `/status`, `/disconnect`, `/reconnect`, `/logout`, proxy, logs, configurações avançadas |
| Envio | `/send/text`, `/link`, `/media`, `/poll`, `/sticker`, `/location`, `/contact`, `/button`, `/list`, `/carousel`, `/status/text`, `/status/media` |
| Mensagens | reagir, presença, marcar lida/ouvida, baixar mídia, apagar, editar |
| Chats | fixar, arquivar, silenciar, sincronizar histórico |
| Usuário | info, verificar número, foto, contatos, privacidade, bloquear, nome e recado do perfil |
| Grupos e comunidades | criar, listar, participantes, nome, descrição, foto, convite, entrar, sair, configurações |
| Outros | etiquetas, canais (newsletter), resultados de enquete, rejeitar chamada |
| Eventos | webhook, RabbitMQ, NATS e WebSocket; mídia no MinIO/S3 |

A referência completa fica no Swagger (`/swagger/index.html`) e na pasta [docs/wiki](docs/wiki).

## Instalação rápida (Docker)

```bash
git clone https://github.com/Edton180/API-Whatsapp-BOB-.git api-whatsapp-bob
cd api-whatsapp-bob
cp .env.example .env        # defina GLOBAL_API_KEY e o PostgreSQL
docker build -t api-whatsapp-bob .
docker run -d --name bob --env-file .env -p 8080:8080 api-whatsapp-bob
```

Mais opções (sem Docker, variáveis, produção): [docs/instalacao.md](docs/instalacao.md).

### Licença da Evolution Go

O motor herdado exige a ativação gratuita da licença da Evolution Go no primeiro uso (rotas `/license/*`). A BOB mantém esse mecanismo como está, porque ele faz parte do código e das condições do projeto oficial.

## Integrações (Chatwoot, Typebot, N8N)

Hoje a BOB integra com essas ferramentas pelos eventos (webhook, RabbitMQ, NATS, WebSocket). As integrações nativas no estilo da Evolution API estão no roteiro do projeto. Veja como ligar cada uma em [docs/integracoes.md](docs/integracoes.md).

## Contribua

A BOB é livre e aberta. Correções, novas funções e documentação são muito bem-vindas: leia o [CONTRIBUTING.md](CONTRIBUTING.md), abra uma [issue](https://github.com/Edton180/API-Whatsapp-BOB-/issues) ou envie um pull request.

## Apoie o projeto (Pix)

<table>
<tr>
<td align="center"><img src="docs/pix-qrcode.png" alt="QR Code Pix para apoiar a API WhatsApp BOB" width="220"></td>
<td>

Abra o app do seu banco, escolha **Pix → Ler QR Code** ou **Pix Copia e Cola**, e digite o valor que quiser.

- **Favorecido:** Edton Kaik Braz Da Conceição
- **Chave Pix (CPF):** `04646947285`

**Pix Copia e Cola:**

```
00020126330014br.gov.bcb.pix0111046469472855204000053039865802BR5915EDTON KAIK BRAZ6007TUCURUI62070503***6304DBF1
```

</td>
</tr>
</table>

O código segue o padrão BR Code do Banco Central, com CRC16 conferido por teste automático (`pkg/routes/bob_test.go`). Para gerar de novo: `python3 scripts/gerar-pix.py`.

## Licença

Apache License 2.0 com as condições adicionais da Evolution Go ([LICENSE](LICENSE)), atribuições em [NOTICE](NOTICE) e política de marca em [TRADEMARKS.md](TRADEMARKS.md). Ao usar a BOB dentro de outro sistema, mostre aos administradores um aviso de que ela é baseada na Evolution Go (condição 1.b da licença).

---

<div align="center">Desenvolvido por <b>Edton Braz</b>, baseado na Evolution Go.</div>
