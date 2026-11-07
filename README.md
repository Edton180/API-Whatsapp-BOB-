<div align="center">

# 🚀 API WhatsApp BOB — Repositório Oficial
### A União Definitiva do Motor em Go da **Evolution Go** com Todas as Funções e Integrações da **Evolution API**

[![Repositório Oficial](https://img.shields.io/badge/Rep%C3%B3sitorio-Oficial-success?style=for-the-badge&logo=github)](https://github.com/Edton180/API-Whatsapp-BOB-)
[![Motor](https://img.shields.io/badge/Engine-Evolution%20Go%20(Golang)-00ADD8?style=for-the-badge&logo=go)](https://github.com/evolution-foundation/evolution-go)
[![Funções](https://img.shields.io/badge/Features-Evolution%20API%20Full-25D366?style=for-the-badge&logo=whatsapp)](https://github.com/evolution-foundation/evolution-api)
[![Criador](https://img.shields.io/badge/Criador-Edton%20Kaik%20Braz%20Da%20Concei%C3%A7%C3%A3o-0A66C2?style=for-the-badge)](https://github.com/Edton180)
[![Contribuições](https://img.shields.io/badge/Comunidade-Contribua%20no%20Oficial-brightgreen?style=for-the-badge)](CONTRIBUTING.md)

</div>

---

## 📖 Arquitetura: O Melhor dos Dois Mundos em Go

A **API WhatsApp BOB** une a velocidade e leveza da linguagem **Go (Golang)** com a riqueza completa de recursos da **Evolution API**:

1. **Baseada na [Evolution Go](https://github.com/evolution-foundation/evolution-go):** Construída em **Golang** sobre `whatsmeow`, garantindo consumo mínimo de RAM, altíssima velocidade de disparo, estabilidade multi-device e suporte a múltiplas instâncias simultâneas.
2. **Rumo à paridade com a [Evolution API](https://github.com/evolution-foundation/evolution-api):** o objetivo é oferecer, sobre o motor em Go, os mesmos endpoints da Evolution API v2 (mensagens, grupos, chats, perfil, configurações e webhooks). Veja a tabela abaixo para o que já está disponível.
3. **Licença:** o motor segue a licença da Evolution Go (Apache 2.0 com condições adicionais, veja [LICENSE](LICENSE)), incluindo a ativação de licença exigida pelo motor. É o motor de alta velocidade da plataforma **OmniFlow - Plataforma Multi-Tenant de Atendimento**.

---

## 👨‍💻 Criador e Autor do Projeto

* **Criador e Mantenedor Oficial:** **Edton Kaik Braz Da Conceição** ([@Edton180](https://github.com/Edton180))
* **Localização:** Tucuruí - Pará, Brasil
* **Repositório Oficial:** [https://github.com/Edton180/API-Whatsapp-BOB-](https://github.com/Edton180/API-Whatsapp-BOB-)

---

## 🏦 Apoie o Projeto Oficial (Doação via PIX)

A **API WhatsApp BOB** é 100% livre, aberta e gratuita para toda a comunidade. Apoie o criador **Edton Kaik Braz Da Conceição** na manutenção e evolução contínua do projeto enviando qualquer valor via **PIX**:

<div align="center">
<table>
  <tr>
    <td align="center" width="320">
      <br>
      <img src="https://api.qrserver.com/v1/create-qr-code/?size=240x240&margin=10&data=00020126360014br.gov.bcb.pix0111046469472855204000053039865802BR5925EDTON%20KAIK%20BRAZ%20DA%20CONCEI6007TUCURUI62070503%2A%2A%2A63046E07" alt="QR Code PIX Oficial - Edton Kaik Braz Da Conceição" width="240" height="240" />
      <br><br>
      <b>📱 Abra o aplicativo do seu banco e escaneie o QR Code</b>
    </td>
    <td align="left" width="460">
      <h3>💠 Comprovante / Dados Bancários PIX</h3>
      <ul>
        <li><b>Favorecido / Criador:</b> Edton Kaik Braz Da Conceição</li>
        <li><b>Chave PIX (CPF):</b> <code>04646947285</code></li>
        <li><b>CPF Formatado:</b> <code>046.469.472-85</code></li>
        <li><b>Cidade:</b> Tucuruí - PA</li>
        <li><b>Valor:</b> Livre (Qualquer valor apoia o projeto)</li>
      </ul>
      <p><b>📋 PIX Copia e Cola (Padrão Oficial EMV Banco Central):</b></p>
      <pre>00020126360014br.gov.bcb.pix0111046469472855204000053039865802BR5925EDTON KAIK BRAZ DA CONCEI6007TUCURUI62070503***63046E07</pre>
    </td>
  </tr>
</table>
</div>

---

## 🔌 Endpoints disponíveis

O motor (`cmd/evolution-go`) expõe as rotas da Evolution Go. A instância é identificada pelo token dela no header `apikey`; rotas de administração (`/instance/create`, `/instance/all`...) usam a `GLOBAL_API_KEY`.

| Módulo | Rotas |
| :--- | :--- |
| **Instâncias** | `/instance/create`, `/instance/all`, `/instance/info/:id`, `/instance/delete/:id`, `/instance/connect`, `/instance/status`, `/instance/qr`, `/instance/pair`, `/instance/disconnect`, `/instance/reconnect`, `/instance/logout`, `/instance/proxy/:id`, `/instance/:id/advanced-settings` |
| **Envio** | `/send/text`, `/send/link`, `/send/media`, `/send/poll`, `/send/sticker`, `/send/location`, `/send/contact`, `/send/button`, `/send/list`, `/send/carousel`, `/send/status/text`, `/send/status/media` |
| **Mensagens** | `/message/react`, `/message/presence`, `/message/markread`, `/message/markplayed`, `/message/downloadmedia`, `/message/status`, `/message/delete`, `/message/edit` |
| **Usuário e contatos** | `/user/info`, `/user/check`, `/user/avatar`, `/user/contacts`, `/user/privacy`, `/user/block`, `/user/unblock`, `/user/blocklist`, `/user/profilePicture`, `/user/profileName`, `/user/profileStatus` |
| **Chats** | `/chat/pin`, `/chat/unpin`, `/chat/archive`, `/chat/unarchive`, `/chat/mute`, `/chat/unmute`, `/chat/history-sync` |
| **Grupos e comunidades** | `/group/list`, `/group/info`, `/group/invitelink`, `/group/photo`, `/group/name`, `/group/description`, `/group/create`, `/group/participant`, `/group/join`, `/group/leave`, `/group/settings`, `/community/create`, `/community/add`, `/community/remove` |
| **Labels, newsletters, chamadas, enquetes** | `/label/*`, `/unlabel/*`, `/newsletter/*`, `/call/reject`, `/polls/:id/results` |

A documentação completa fica em `/swagger/index.html` e em [`docs/wiki`](docs/wiki/README.md). As rotas no formato da Evolution API v2 (`/message/sendText/:instance` etc.) estão sendo adicionadas sobre este motor.

--- | :--- |
| **Instâncias & Conexão** | `/instance/create`, `/instance/connect`, `/instance/fetchInstances`, `/instance/delete`, `/settings/set/:instance`, `/settings/find/:instance` |
| **Mensagens Completas** | `/message/sendText/:instance`, `/message/sendMedia/:instance`, `/message/sendWhatsAppAudio/:instance`, `/message/sendPoll/:instance`, `/message/sendList/:instance`, `/message/sendButtons/:instance`, `/message/sendLocation/:instance`, `/message/sendReaction/:instance`, `/message/sendSticker/:instance`, `/message/sendContact/:instance`, `/message/sendStatus/:instance` |
| **Gestão de Grupos** | `/group/create/:instance`, `/group/fetchAllGroups/:instance`, `/group/findGroupInfos/:instance`, `/group/updateParticipant/:instance`, `/group/updateSubject/:instance`, `/group/updateDescription/:instance`, `/group/updateSetting/:instance`, `/group/inviteCode/:instance`, `/group/revokeInviteCode/:instance`, `/group/leaveGroup/:instance` |
| **Chats, Contatos & Perfil** | `/chat/whatsappNumbers/:instance`, `/chat/markMessageAsRead/:instance`, `/chat/archiveChat/:instance`, `/chat/deleteMessageForEveryone/:instance`, `/chat/sendPresence/:instance`, `/chat/fetchProfile/:instance`, `/chat/updateProfileName/:instance`, `/chat/updateProfileStatus/:instance`, `/chat/updateProfilePicture/:instance` |
| **Integrações Nativas** | `/chatwoot/set/:instance`, `/typebot/set/:instance`, `/n8n/set/:instance`, `/openai/set/:instance`, `/dify/set/:instance`, `/evolutionBot/set/:instance`, `/webhook/set/:instance`, `/rabbitmq/set/:instance`, `/sqs/set/:instance`, `/websocket/set/:instance` |

---

## 🔄 Sincronização Seletiva Contínua (`evolution-go` + `evolution-api` + `OmniFlow`)

Sempre que a `evolution-go` ou a `evolution-api` lançarem atualizações, você puxa apenas as partes atualizadas para a **API WhatsApp BOB** (e reflete no **OmniFlow**) executando:

```bash
./scripts/sync-upstreams.sh
```

---

## 🤝 Contribua Diretamente no Repositório Oficial

Não divida esforços! Toda a comunidade é convidada a **contribuir e melhorar diretamente este Repositório Oficial (`https://github.com/Edton180/API-Whatsapp-BOB-`)**:
* Abra **[Issues Oficiais](https://github.com/Edton180/API-Whatsapp-BOB-/issues)** para relatar bugs ou sugerir melhorias.
* Envie **[Pull Requests](https://github.com/Edton180/API-Whatsapp-BOB-/pulls)** diretamente para a branch `main` oficial para evoluirmos juntos a API mais rápida em Go do Brasil!
* Leia nosso [Guia de Contribuição Oficial (CONTRIBUTING.md)](CONTRIBUTING.md).
