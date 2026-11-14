# Chamadas de voz e vídeo

A partir da 2.4.0 a BOB atende e faz ligações de voz e vídeo do WhatsApp pelo navegador dos atendentes. Vários
atendentes podem ver a mesma chamada tocando: o primeiro que atender leva, e a chamada pode ser passada para
outro atendente sem derrubar o contato.

O áudio e o vídeo passam pela BOB em tempo real e **não são gravados**. No banco fica só o registro da chamada
(número, se foi de vídeo, quem ligou, quando começou, quando foi atendida, duração e como terminou), na tabela
`bob_calls`.

## Como funciona

```
Contato (WhatsApp)  <--- protocolo de chamadas do WhatsApp --->  BOB  <--- WebRTC --->  Navegador do atendente
                        áudio MLOW/Opus 16 kHz, vídeo H.264           áudio G.722 (ou PCMU), vídeo H.264
```

- O lado do WhatsApp usa a biblioteca [meowcaller](https://github.com/purpshell/meowcaller) (MIT), que fala o
  protocolo de chamadas sobre a whatsmeow.
- O lado do navegador usa WebRTC com o [Pion](https://github.com/pion/webrtc) (MIT). O áudio vai em **G.722**
  (banda larga, 16 kHz, a mesma taxa do WhatsApp, sem reamostrar) com PCMU de reserva. O codec G.722 é
  implementação própria da BOB (`internal/calls/g722.go`).
- O vídeo H.264 passa sem ser recodificado nos dois sentidos.
- A sinalização é uma requisição só: o navegador manda a oferta SDP já com os candidatos e recebe a resposta.

> A meowcaller ainda marca o caminho do vídeo como "não validado". A voz é o caminho principal; o vídeo funciona
> quando os dois lados negociam H.264, e deve ser testado no seu servidor antes de ser liberado aos atendentes.

## Ligar as chamadas

As chamadas ficam **desligadas** em cada instância até você ligar. Com elas ligadas, a BOB pré-aceita as chamadas
recebidas para poder atender pelo navegador.

- No painel: **Chamadas** › escolha a instância › marque **Chamadas ligadas nesta instância**.
- Pela API: `POST /call/settings {"enabled": true}`.

Ligar ou desligar reconecta a instância com a mesma sessão (não precisa ler o QR Code de novo).

Se a instância estiver com **Recusar ligações** (`rejectCall`) ligado, as chamadas continuam sendo recusadas, com a
mensagem configurada, e ficam no histórico como recusadas.

## Rede: portas, IP público e TURN

O navegador precisa alcançar a BOB por UDP.

| Variável | Padrão | Para que serve |
|---|---|---|
| `WEBRTC_UDP_MUX_PORT` | `0` (`40000` no Docker) | Porta UDP única para todas as chamadas. Libere no firewall |
| `WEBRTC_UDP_PORT_MIN` / `WEBRTC_UDP_PORT_MAX` | `0` | Faixa de portas UDP, no lugar da porta única |
| `WEBRTC_PUBLIC_IP` | vazio | IPs anunciados ao navegador, separados por vírgula, **somados** aos endereços da própria máquina. Na VPS: o IP público. No computador com Docker: `127.0.0.1` |
| `WEBRTC_ICE_SERVERS` | `stun:stun.l.google.com:19302` | STUN e TURN entregues ao navegador, separados por vírgula |
| `WEBRTC_TURN_SECRET` | vazio | Segredo compartilhado com o TURN (coturn com `use-auth-secret`) |
| `WEBRTC_TURN_TTL` | `600` | Validade, em segundos, da senha TURN entregue ao navegador |

Exemplo para uma VPS com Docker:

```env
WEBRTC_UDP_MUX_PORT=40000
WEBRTC_PUBLIC_IP=203.0.113.10
WEBRTC_ICE_SERVERS=stun:stun.l.google.com:19302,turn:turn.seudominio.com:3478
WEBRTC_TURN_SECRET=um-segredo-longo
```

```bash
sudo ufw allow 40000/udp
```

No computador (Docker Desktop ou Docker no Ubuntu, com o navegador na mesma máquina), use
`WEBRTC_PUBLIC_IP=127.0.0.1`: a porta 40000/udp publicada pelo Docker atende no próprio computador.

Até a 2.4.1 o IP informado **substituía** os endereços locais. Um navegador na mesma rede do servidor
(ou no mesmo computador) não alcançava o IP público sem redirecionamento no roteador e a chamada
ficava muda nos dois sentidos. Desde a 2.4.2 os IPs são somados.

### Chamada muda

No fim de cada chamada o registro da instância (Instâncias › Registro) mostra quanto áudio passou em
cada sentido e se o navegador conectou:

```
áudio da chamada ABC123: 350 quadros vindos do WhatsApp, 1050 pacotes entregues ao navegador, 1040 pacotes vindos do navegador (navegador conectado: sim)
```

- `navegador conectado: não`: o navegador não alcançou a porta UDP. Confira `WEBRTC_PUBLIC_IP`, o firewall
  e, em redes que bloqueiam UDP, configure um TURN.
- `0 quadros vindos do WhatsApp`: o áudio do contato não chegou à BOB.
- `0 pacotes vindos do navegador`: o microfone do atendente não mandou áudio (permissão ou dispositivo).

**TURN** é recomendado para atendentes em redes que bloqueiam UDP (empresas, hotéis, 4G com CGNAT). Com o coturn:

```
use-auth-secret
static-auth-secret=um-segredo-longo
realm=turn.seudominio.com
```

O segredo nunca sai da BOB: o navegador recebe um usuário `validade:atendente` e uma senha temporária
(HMAC-SHA1 do usuário, padrão "TURN REST"), que vence em `WEBRTC_TURN_TTL` segundos.

Os navegadores só liberam o microfone e a câmera em **HTTPS** (ou em `localhost`).

## Rotas

Todas aceitam o token da instância no cabeçalho `apikey`, ou a chave global com o cabeçalho `instance`.

| Rota | O que faz |
|---|---|
| `GET /call/settings` | Chamadas ligadas? Ativas agora? Recusa automática? |
| `POST /call/settings` | `{"enabled": true}` liga ou desliga |
| `GET /call/ice-servers?agent=Ana` | STUN e TURN (com senha temporária) para o `RTCPeerConnection` |
| `GET /call/live` | Chamadas tocando ou em andamento, com o atendente de cada uma |
| `GET /call/list` | Histórico: `?status=missed\|ended\|rejected\|ringing\|connected&number=&video=true&since=<unix>&limit=50&offset=0` |
| `GET /call/info/{callId}` | Uma chamada |
| `POST /call/answer` | `{"callId", "sdp", "agent"}` atende no navegador. Outro atendente que tentar recebe **409** |
| `POST /call/start` | `{"number", "video", "sdp", "agent"}` liga para um número |
| `POST /call/transfer` | `{"callId", "sdp", "agent"}` passa a chamada para o navegador de outro atendente |
| `POST /call/reject` | `{"callId"}` recusa uma chamada tocando |
| `POST /call/hangup` | `{"callId"}` desliga |
| `POST /call/mute` | `{"callId", "muted": true}` microfone do atendente (o contato ouve silêncio) |
| `POST /call/camera` | `{"callId", "on": true}` câmera; numa chamada de voz pede ao contato para passar a vídeo, e aceita se o contato pediu |
| `POST /call/ice` | Candidato ICE extra (opcional) |
| `POST /call/offer/{instância}` | Padrão v2: faz o número tocar por `callDuration` segundos e desliga, sem atendente |

`answer`, `start` e `transfer` respondem:

```json
{"data": {"type": "answer", "sdp": "v=0...", "call": {"callId": "3EB0...", "number": "5511999998888", "status": "connected", "agent": "Ana", "...": "..."}}}
```

Códigos de erro: `404` chamada que já terminou, `409` chamada com outro atendente, `412` chamadas desligadas na
instância, `409` também quando a instância não está conectada.

## Eventos

Categoria `CALL` no webhook e no WebSocket (no padrão v2 todos chegam como `CALL`, em `URL/call`).

| Evento | Quando |
|---|---|
| `CallRinging` | Chamada recebida tocando, ou ligação feita chamando |
| `CallConnected` | Atendida (pelo navegador, ou o contato atendeu a ligação feita) |
| `CallMedia` | Microfone, câmera, pedido de vídeo do contato ou transferência |
| `CallEnded` | Encerrada (`status`: `ended`, `missed`, `rejected`; `reason` com o motivo do WhatsApp) |

`CallOffer`, `CallAccept` e `CallTerminate` continuam chegando como antes, para quem já usa.

Exemplo de `CallRinging`:

```json
{
  "event": "CallRinging",
  "instanceName": "atendimento",
  "data": {
    "callId": "3EB0C4A1F2D3E4B5A6C7D8E9F0A1B2C3",
    "number": "5511999998888",
    "remoteJid": "5511999998888@s.whatsapp.net",
    "fromMe": false,
    "isVideo": false,
    "status": "ringing",
    "startedAt": 1794120000,
    "live": true,
    "agent": ""
  }
}
```

Uma chamada atendida no celular termina com `reason: "accepted_elsewhere"` e fica como `ended` (não como perdida).

## Navegador (exemplo mínimo)

```js
const H = { apikey: TOKEN_DA_INSTANCIA, "Content-Type": "application/json" };
const ice = await (await fetch("/call/ice-servers?agent=Ana", { headers: H })).json();
const mic = await navigator.mediaDevices.getUserMedia({ audio: true });
const pc = new RTCPeerConnection({ iceServers: ice.data.iceServers });
pc.addTrack(mic.getAudioTracks()[0], mic);
const camera = pc.addTransceiver("video", { direction: "sendrecv" }).sender; // câmera liga depois com replaceTrack
pc.ontrack = (e) => { document.querySelector(e.track.kind === "audio" ? "audio" : "video").srcObject = new MediaStream([e.track]); };
await pc.setLocalDescription(await pc.createOffer());
await new Promise((ok) => { pc.onicegatheringstatechange = () => pc.iceGatheringState === "complete" && ok(); setTimeout(ok, 4000); });

const r = await (await fetch("/call/answer", { method: "POST", headers: H,
  body: JSON.stringify({ callId, sdp: pc.localDescription.sdp, agent: "Ana" }) })).json();
await pc.setRemoteDescription({ type: "answer", sdp: r.data.sdp });
```

O softphone do painel (`internal/panel/static/app.js`, seção "chamadas") é um exemplo completo, com microfone,
câmera, transferência e desligar.

## Limites

- Chamadas em grupo não passam pela BOB (continuam tocando no celular).
- Se o navegador do atendente cair (rede ou aba fechada), a BOB desliga a chamada para o contato não ficar falando
  sozinho. Sem conexão em 30 segundos depois de atender, a chamada também é encerrada.
- Uma chamada que estava em andamento quando a BOB reiniciou fica como encerrada com `reason: "server_restart"`.
