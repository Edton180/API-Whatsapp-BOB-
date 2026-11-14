# Histórico de versões

## 2.4.2 - 2026-11-08

- **Chamadas sem áudio**: com `WEBRTC_PUBLIC_IP` preenchido, a BOB anunciava ao navegador só o IP público.
  O navegador na mesma rede (ou no mesmo computador) não alcançava esse IP e a chamada ficava muda nos dois
  sentidos. Agora os IPs informados são somados aos endereços locais, e `WEBRTC_PUBLIC_IP` aceita uma lista
  (ex.: `203.0.113.10,127.0.0.1`).
- Diagnóstico de chamada muda no registro da instância: o caminho escolhido até o navegador e, no fim, quanto
  áudio passou em cada sentido.

## 2.4.1 - 2026-11-08

- **QR Code e código de pareamento voltaram a conectar**: no instante em que o celular lia o QR, a BOB fechava o
  canal do QR Code e isso derrubava a conexão no meio do pareamento ("não foi possível conectar o dispositivo").
  O canal não é mais cancelado e a BOB espera o WhatsApp concluir o pareamento.
- Pedidos de conexão repetidos (o OmniFlow consulta o QR a cada poucos segundos) não abrem mais uma segunda conexão
  por cima da que está pareando. Reconectar nos 45 segundos depois do pareamento é ignorado.
- Código de pareamento: se o WhatsApp fechar a conexão antes de responder ("websocket disconnected before info
  query returned response"), a BOB reconecta e tenta de novo uma vez.

## 2.4.0 - 2026-11-08

- **Chamadas de voz e vídeo pelo navegador**: atender, recusar, fazer ligações, desligar, microfone, câmera e
  transferir para outro atendente sem derrubar o contato. Vários atendentes veem a chamada tocando e o primeiro
  que atender leva (os outros recebem 409). Veja `docs/chamadas.md`.
- Ponte WhatsApp <-> WebRTC com a meowcaller (MIT) e o Pion (MIT). Áudio em G.722 (codec próprio da BOB, 16 kHz,
  sem reamostrar) com PCMU de reserva; vídeo H.264 passa sem recodificar.
- **Histórico de chamadas** na tabela `bob_calls` (recebidas, feitas, atendidas, perdidas, recusadas, duração).
  Áudio e vídeo nunca são gravados.
- Rotas `/call/settings`, `/call/ice-servers`, `/call/list`, `/call/live`, `/call/info/{callId}`, `/call/start`,
  `/call/answer`, `/call/transfer`, `/call/hangup`, `/call/mute`, `/call/camera`, `/call/ice`; `POST /call/reject`
  recusa a chamada viva; `POST /call/offer/{instância}` (padrão v2) passa a fazer o número tocar.
- Eventos `CallRinging`, `CallConnected`, `CallMedia` e `CallEnded` (categoria `CALL`).
- Credencial TURN temporária (padrão TURN REST): o segredo nunca vai para o navegador.
- Painel: página **Chamadas** com softphone, chamadas tocando e histórico.
- Variáveis `WEBRTC_ICE_SERVERS`, `WEBRTC_UDP_MUX_PORT`, `WEBRTC_UDP_PORT_MIN/MAX`, `WEBRTC_PUBLIC_IP`,
  `WEBRTC_TURN_SECRET` e `WEBRTC_TURN_TTL`; Docker publica a porta UDP 40000.

## 2.3.0 - 2026-11-08

- **Webhook no padrão v2**: um endereço por evento (`URL/messages-upsert`, `URL/connection-update`...) e mídia em
  base64 ligada ou desligada por instância (`byEvents` e `base64` em `/webhook/set`, `/instance/create` e no painel).
- **Versão do WhatsApp Web renovada sozinha**: a cada 6 horas e quando o WhatsApp recusa o cliente (erro 405,
  "cliente desatualizado"), que fazia o QR Code não aparecer. A sessão reconecta sozinha depois.
- **Números "zumbis"**: quem aparece conectado mas fica sem nenhum evento há mais de `RECONNECT_IDLE_HOURS`
  (padrão 6) é reconectado, sem QR Code.
- Prints do painel e da documentação atualizados.

## 2.2.0 - 2026-11-07

- **Histórico de mensagens e conversas** no banco: recebidas, enviadas e sincronizadas do celular, com status de
  entrega e leitura (`SERVER_ACK`, `DELIVERY_ACK`, `READ`, `PLAYED`), edições e mensagens apagadas para todos.
  Rotas `/chat/list`, `/chat/messages`, `/message/get` e, no padrão v2, `findMessages`, `findChats` e
  `findStatusMessage`. Veja `docs/historico.md`.
- **Buscar mensagens antigas** no celular: `POST /chat/history`.
- **Votos de enquete decifrados**: evento `PollVote` com o nome das opções e `GET /message/poll` com o resultado.
- Responder citando e baixar mídia funcionam com mensagens antigas, mesmo depois de reiniciar.
- Arquivar e marcar como lida/não lida usam a última mensagem guardada quando ela não é informada.
- Novas variáveis `STORE_MESSAGES` (padrão `true`) e `STORE_MESSAGES_DAYS` (padrão `0`, guarda para sempre).

## 2.1.0 - 2026-11-07

- **Rotas no padrão v2** (`/message/sendText/{instância}`, `/group/fetchAllGroups/{instância}`...): mais de 60 rotas no formato
  usado pela Evolution API v2, para sistemas já feitos nesse padrão migrarem trocando só o endereço. Veja `docs/padrao-v2.md`.
- **Grupos**: quem envia e quem edita, aprovação de entrada, pedidos pendentes (aprovar/recusar), ver convite sem entrar,
  mensagens temporárias, apagar foto.
- **Comunidades**: criar, ligar e desligar grupos, listar grupos.
- **Canais**: criar, ver por id ou link, seguir, deixar de seguir, silenciar, ler mensagens.
- **Perfil**: perfil comercial, alterar privacidade, apagar foto, ficar online/offline.
- **Envio**: vídeo redondo (`type: ptv`) e áudio como arquivo comum (`type: audio-file`).
- **Chamadas**: `/call/reject`.
- Arquivar e marcar como não lida aceitam a última mensagem da conversa, como o WhatsApp exige para refletir no celular.
- Biblioteca whatsmeow atualizada para a versão de 2026-11-07.

## 2.0.0 - 2026-11-07

A BOB passa a ter motor, painel e documentação próprios.

- **Motor próprio** escrito do zero sobre a biblioteca [whatsmeow](https://github.com/tulir/whatsmeow) (MPL 2.0). Nenhum código de outra API de WhatsApp.
- **Sem licença para ativar**: as rotas `/license/*` continuam existindo só por compatibilidade e respondem `active`.
- **Painel web** em `/painel/`: instâncias, QR Code, código de pareamento, webhook e eventos, comportamento (recusar ligação, marcar como lida, sempre online, ignorar grupos/status), proxy, integrações (Chatwoot, Typebot, N8N), registro, envio de teste, eventos ao vivo, dados do servidor e Pix.
- **Documentação** própria em `/swagger/index.html` e especificação em `/openapi.yaml` e `/openapi.json`.
- Mesmas rotas e formatos das versões 1.x, então sistemas já integrados (como o OmniFlow) continuam funcionando sem mudança.
- Atualização sem perder nada: usa as mesmas tabelas (`instances` e as sessões do WhatsApp), então números conectados continuam conectados depois de atualizar.
- Webhook com fila por destino (mantém a ordem) e até 4 tentativas; WebSocket em `/ws`.
- Banco PostgreSQL ou SQLite (sem configurar nada).
- Novas rotas: `/server/info`, `/instance/{id}/webhook`, `/message/downloadmedia`, `/chat/read`, `/chat/unread`, `/label/message`, `/group/join`, `/user/privacy`, `/newsletter/list`.
- Dados pessoais do criador removidos (a cidade saiu também do Pix).

## 1.x

Versões baseadas em outro motor, substituídas pela 2.0.
