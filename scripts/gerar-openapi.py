#!/usr/bin/env python3
# API WhatsApp BOB - Desenvolvido por Edton Braz.
# Gera internal/panel/openapi.yaml (documentação mostrada em /swagger). Rode: python3 scripts/gerar-openapi.py
import yaml, pathlib

G, I, A = "global", "instancia", "publica"
N = "5511999998888"
R = [
  # tag, método, caminho, auth, resumo, corpo de exemplo
  ("Servidor", "get", "/", A, "Identificação da API (versão, commit, links)", None),
  ("Servidor", "get", "/server/ok", A, "Saúde da API (healthcheck)", None),
  ("Servidor", "get", "/server/info", G, "Resumo do servidor: versão, uptime, instâncias, memória", None),
  ("Servidor", "get", "/license/status", A, "Compatibilidade: sempre 'active' (a BOB não exige licença)", None),
  ("Servidor", "get", "/donate/pix", A, "Dados de doação ao projeto (Pix)", None),
  ("Instâncias", "post", "/instance/create", G, "Cria uma instância (um número de WhatsApp)",
   {"name": "atendimento", "token": "TOKEN-OPCIONAL", "webhook": "https://seu-sistema/webhook", "events": ["MESSAGE", "CONNECTION"],
    "advancedSettings": {"rejectCall": True, "msgRejectCall": "Não atendemos ligações", "readMessages": False, "ignoreGroups": True}}),
  ("Instâncias", "get", "/instance/all", G, "Lista as instâncias com o estado da conexão", None),
  ("Instâncias", "get", "/instance/info/{id}", G, "Dados de uma instância (id ou nome)", None),
  ("Instâncias", "delete", "/instance/delete/{id}", G, "Desconecta, encerra a sessão e apaga a instância", None),
  ("Instâncias", "post", "/instance/forcereconnect/{id}", G, "Força a reconexão de uma instância travada", None),
  ("Instâncias", "post", "/instance/proxy/{id}", G, "Define o proxy da instância", {"protocol": "http", "host": "proxy.exemplo.com", "port": "8080", "username": "", "password": ""}),
  ("Instâncias", "delete", "/instance/proxy/{id}", G, "Remove o proxy da instância", None),
  ("Instâncias", "get", "/instance/logs/{id}", G, "Histórico recente de eventos da instância", None),
  ("Instâncias", "get", "/instance/{id}/advanced-settings", "ambas", "Lê as configurações avançadas", None),
  ("Instâncias", "put", "/instance/{id}/advanced-settings", "ambas", "Altera as configurações avançadas",
   {"alwaysOnline": False, "rejectCall": True, "msgRejectCall": "Não atendemos ligações", "readMessages": False, "ignoreGroups": True, "ignoreStatus": True}),
  ("Instâncias", "post", "/instance/{id}/webhook", "ambas", "Troca o webhook e os eventos sem reconectar", {"webhook": "https://seu-sistema/webhook", "subscribe": ["MESSAGE", "READ_RECEIPT", "CONNECTION"], "byEvents": False, "base64": True}),
  ("Conexão", "post", "/instance/connect", I, "Inicia a sessão; sem login, gera QR Codes (e código de pareamento se phone for informado)",
   {"webhookUrl": "https://seu-sistema/webhook", "subscribe": ["MESSAGE", "SEND_MESSAGE", "READ_RECEIPT", "CONNECTION", "QRCODE"], "immediate": True, "phone": ""}),
  ("Conexão", "get", "/instance/status", I, "Conectada? Logada? Nome do perfil", None),
  ("Conexão", "get", "/instance/qr", I, "QR Code atual (imagem PNG em data URI e texto)", None),
  ("Conexão", "post", "/instance/pair", I, "Código de 8 caracteres para conectar digitando no celular", {"phone": N}),
  ("Conexão", "post", "/instance/disconnect", I, "Desconecta mantendo o login (volta sem QR Code)", None),
  ("Conexão", "post", "/instance/reconnect", I, "Reconecta", None),
  ("Conexão", "delete", "/instance/logout", I, "Encerra a sessão no celular (precisa de novo QR Code)", None),
  ("Envio", "post", "/send/text", I, "Texto", {"number": N, "text": "Olá! 👋", "delay": 1200, "quoted": {"messageId": "", "participant": ""}, "mentionedJid": []}),
  ("Envio", "post", "/send/link", I, "Texto com link", {"number": N, "text": "Veja", "url": "https://github.com/Edton180/API-Whatsapp-BOB-", "title": "API WhatsApp BOB", "description": "API de WhatsApp em Go"}),
  ("Envio", "post", "/send/media", I, "Imagem, vídeo, áudio (vira nota de voz) ou documento. Aceita JSON (url/base64) ou multipart (file)",
   {"number": N, "url": "https://exemplo.com/foto.jpg", "type": "image", "caption": "Legenda", "filename": ""}),
  ("Envio", "post", "/send/sticker", I, "Figurinha: PNG, JPG, GIF ou WebP vira WebP 512×512", {"number": N, "sticker": "https://exemplo.com/imagem.png"}),
  ("Envio", "post", "/send/location", I, "Localização", {"number": N, "latitude": -23.55, "longitude": -46.63, "name": "Escritório", "address": "Av. Paulista"}),
  ("Envio", "post", "/send/contact", I, "Cartão de contato", {"number": N, "vcard": {"fullName": "Suporte", "phone": "5511988887777", "organization": "Empresa"}}),
  ("Envio", "post", "/send/poll", I, "Enquete", {"number": N, "question": "Qual horário prefere?", "options": ["Manhã", "Tarde"], "maxAnswer": 1}),
  ("Envio", "post", "/send/button", I, "Botões: reply, url, call, copy e pix",
   {"number": N, "title": "Pedido #120", "description": "Escolha uma opção", "footer": "Loja",
    "buttons": [{"type": "reply", "displayText": "Confirmar", "id": "ok"}, {"type": "url", "displayText": "Site", "url": "https://exemplo.com"},
                {"type": "copy", "displayText": "Copiar código", "copyCode": "ABC123"}]}),
  ("Envio", "post", "/send/list", I, "Lista de opções",
   {"number": N, "title": "Cardápio", "description": "Escolha", "buttonText": "Ver opções", "footerText": "",
    "sections": [{"title": "Lanches", "rows": [{"rowId": "x1", "title": "X-Burguer", "description": "R$ 20"}]}]}),
  ("Envio", "post", "/send/status/text", I, "Publica um status de texto", {"text": "Novidades!", "backgroundColor": 4280138324, "font": 1}),
  ("Envio", "post", "/send/status/media", I, "Publica um status com imagem ou vídeo", {"url": "https://exemplo.com/foto.jpg", "type": "image", "caption": ""}),
  ("Mensagens", "post", "/message/react", I, "Reage com emoji (vazio remove)", {"number": N, "id": "3EB0...", "reaction": "👍", "fromMe": False}),
  ("Mensagens", "post", "/message/markread", I, "Marca como lidas", {"number": N, "id": ["3EB0..."]}),
  ("Mensagens", "post", "/message/delete", I, "Apaga para todos (mensagem enviada pela instância)", {"chat": N, "messageId": "3EB0..."}),
  ("Mensagens", "post", "/message/edit", I, "Edita o texto de uma mensagem enviada", {"chat": N, "messageId": "3EB0...", "message": "Texto corrigido"}),
  ("Mensagens", "post", "/message/presence", I, "Digitando, gravando áudio ou pausado", {"number": N, "state": "composing", "isAudio": False, "delay": 3000}),
  ("Mensagens", "post", "/message/downloadmedia", I, "Baixa a mídia de uma mensagem recebida (campo Message do webhook)", {"message": {"imageMessage": {}}}),
  ("Usuários", "post", "/user/check", I, "Quais números têm WhatsApp", {"number": [N]}),
  ("Usuários", "post", "/user/info", I, "Recado e dados comerciais", {"number": [N]}),
  ("Usuários", "post", "/user/avatar", I, "URL da foto de perfil", {"number": N, "preview": False}),
  ("Usuários", "get", "/user/contacts", I, "Agenda sincronizada do aparelho", None),
  ("Usuários", "post", "/user/block", I, "Bloqueia", {"number": N}),
  ("Usuários", "post", "/user/unblock", I, "Desbloqueia", {"number": N}),
  ("Usuários", "get", "/user/blocklist", I, "Contatos bloqueados", None),
  ("Usuários", "get", "/user/privacy", I, "Configurações de privacidade", None),
  ("Usuários", "post", "/user/profileName", I, "Troca o nome do perfil", {"name": "Atendimento"}),
  ("Usuários", "post", "/user/profileStatus", I, "Troca o recado", {"status": "Atendimento das 8h às 18h"}),
  ("Usuários", "post", "/user/profilePicture", I, "Troca a foto do perfil", {"image": "https://exemplo.com/logo.png"}),
] + [("Conversas", "post", f"/chat/{a}", I, d, {"chat": N}) for a, d in [
  ("archive", "Arquiva"), ("unarchive", "Desarquiva"), ("pin", "Fixa"), ("unpin", "Desafixa"), ("mute", "Silencia (duration em horas)"),
  ("unmute", "Tira o silêncio"), ("read", "Marca a conversa como lida"), ("unread", "Marca como não lida")]] + [
  ("Etiquetas", "get", "/label/list", I, "Etiquetas do WhatsApp Business", None),
  ("Etiquetas", "post", "/label/chat", I, "Etiqueta uma conversa", {"jid": N, "labelId": "1"}),
  ("Etiquetas", "post", "/unlabel/chat", I, "Tira a etiqueta da conversa", {"jid": N, "labelId": "1"}),
  ("Etiquetas", "post", "/label/message", I, "Etiqueta uma mensagem", {"jid": N, "labelId": "1", "messageId": "3EB0..."}),
  ("Etiquetas", "post", "/unlabel/message", I, "Tira a etiqueta da mensagem", {"jid": N, "labelId": "1", "messageId": "3EB0..."}),
  ("Etiquetas", "post", "/label/edit", I, "Cria, renomeia ou apaga etiqueta", {"labelId": "1", "name": "Cliente VIP", "color": 2, "deleted": False}),
  ("Grupos", "get", "/group/myall", I, "Grupos em que o número participa", None),
  ("Grupos", "post", "/group/info", I, "Dados do grupo", {"groupJid": "120363000000000000@g.us"}),
  ("Grupos", "post", "/group/create", I, "Cria grupo", {"groupName": "Equipe", "participants": [N]}),
  ("Grupos", "post", "/group/participant", I, "add, remove, promote ou demote", {"groupJid": "120363000000000000@g.us", "action": "add", "participants": [N]}),
  ("Grupos", "post", "/group/name", I, "Troca o nome", {"groupJid": "120363000000000000@g.us", "name": "Equipe 2"}),
  ("Grupos", "post", "/group/description", I, "Troca a descrição", {"groupJid": "120363000000000000@g.us", "description": "Avisos"}),
  ("Grupos", "post", "/group/photo", I, "Troca a foto", {"groupJid": "120363000000000000@g.us", "image": "https://exemplo.com/logo.png"}),
  ("Grupos", "post", "/group/invitelink", I, "Link de convite (reset gera outro)", {"groupJid": "120363000000000000@g.us", "reset": False}),
  ("Grupos", "post", "/group/join", I, "Entra pelo link ou código", {"code": "https://chat.whatsapp.com/XXXX"}),
  ("Grupos", "post", "/group/leave", I, "Sai do grupo", {"groupJid": "120363000000000000@g.us"}),
  ("Canais", "get", "/newsletter/list", I, "Canais (newsletters) seguidos", None),
]
# complementos da API nativa
R += [
  ("Envio", "post", "/send/media#ptv", I, "Vídeo redondo (recado de vídeo): use type ptv em /send/media", None),
  ("Grupos", "post", "/group/settings", I, "Quem envia e quem edita: announcement, not_announcement, locked, unlocked, approval_on, approval_off, admin_add, all_member_add",
   {"groupJid": "120363000000000000@g.us", "action": "announcement"}),
  ("Grupos", "post", "/group/ephemeral", I, "Mensagens temporárias do grupo: 0, 86400, 604800 ou 7776000 segundos", {"groupJid": "120363000000000000@g.us", "expiration": 604800}),
  ("Grupos", "post", "/group/inviteinfo", I, "Dados do grupo pelo link, sem entrar", {"code": "https://chat.whatsapp.com/XXXX"}),
  ("Grupos", "post", "/group/requests", I, "Pedidos para entrar (grupo com aprovação)", {"groupJid": "120363000000000000@g.us"}),
  ("Grupos", "post", "/group/requests/update", I, "Aprova ou recusa pedidos", {"groupJid": "120363000000000000@g.us", "action": "approve", "participants": [N]}),
  ("Grupos", "post", "/group/photo/remove", I, "Apaga a foto do grupo", {"groupJid": "120363000000000000@g.us"}),
  ("Conversas", "post", "/chat/ephemeral", I, "Mensagens temporárias de uma conversa", {"number": N, "expiration": 86400}),
  ("Comunidades", "post", "/community/create", I, "Cria uma comunidade", {"name": "Clientes", "description": "Avisos e grupos"}),
  ("Comunidades", "post", "/community/link", I, "Liga um grupo à comunidade", {"communityJid": "120363000000000000@g.us", "groupJid": "120363000000000000@g.us"}),
  ("Comunidades", "post", "/community/unlink", I, "Tira um grupo da comunidade", {"communityJid": "120363000000000000@g.us", "groupJid": "120363000000000000@g.us"}),
  ("Comunidades", "post", "/community/groups", I, "Grupos da comunidade", {"communityJid": "120363000000000000@g.us"}),
  ("Usuários", "post", "/user/business", I, "Perfil comercial (endereço, e-mail, site, horário)", {"number": N}),
  ("Usuários", "post", "/user/privacy", I, "Altera a privacidade (all, contacts, contact_blacklist, none, match_last_seen)",
   {"last": "contacts", "online": "match_last_seen", "profile": "all", "status": "contacts", "readreceipts": "all", "groupadd": "contacts", "calladd": "all"}),
  ("Usuários", "delete", "/user/profilePicture", I, "Apaga a foto do perfil", None),
  ("Conexão", "post", "/instance/presence", I, "Mostra como online (available) ou offline (unavailable)", {"presence": "available"}),
  ("Chamadas", "post", "/call/reject", I, "Recusa uma ligação tocando (callId do evento CallRinging ou CallOffer)", {"callId": "ABC123", "from": N}),
  ("Chamadas", "get", "/call/settings", I, "Chamadas ligadas na instância? Recusa automática?", None),
  ("Chamadas", "post", "/call/settings", I, "Liga ou desliga as chamadas (reconecta a instância sem QR Code)", {"enabled": True}),
  ("Chamadas", "get", "/call/ice-servers", I, "STUN e TURN para o RTCPeerConnection do navegador (?agent=nome); a senha TURN é temporária", None),
  ("Chamadas", "get", "/call/list", I, "Histórico (?status=missed|ended|rejected|ringing|connected&number=&video=&since=<unix>&limit=50&offset=0)", None),
  ("Chamadas", "get", "/call/live", I, "Chamadas tocando ou em andamento, com o atendente de cada uma", None),
  ("Chamadas", "get", "/call/info/{callId}", I, "Dados de uma chamada", None),
  ("Chamadas", "post", "/call/start", I, "Liga para um número pelo navegador: manda a oferta SDP e recebe a resposta",
   {"number": N, "video": False, "sdp": "v=0...", "agent": "Ana"}),
  ("Chamadas", "post", "/call/answer", I, "Atende no navegador (o primeiro atendente leva; os outros recebem 409)", {"callId": "ABC123", "sdp": "v=0...", "agent": "Ana"}),
  ("Chamadas", "post", "/call/transfer", I, "Passa a chamada para o navegador de outro atendente sem derrubar o contato", {"callId": "ABC123", "sdp": "v=0...", "agent": "Bruno"}),
  ("Chamadas", "post", "/call/hangup", I, "Desliga", {"callId": "ABC123"}),
  ("Chamadas", "post", "/call/mute", I, "Microfone do atendente (o contato ouve silêncio)", {"callId": "ABC123", "muted": True}),
  ("Chamadas", "post", "/call/camera", I, "Liga ou desliga a câmera (numa chamada de voz, pede para passar a vídeo)", {"callId": "ABC123", "on": True}),
  ("Chamadas", "post", "/call/ice", I, "Candidato ICE extra do navegador (opcional; a oferta já leva os candidatos)", {"callId": "ABC123", "candidate": "candidate:...", "sdpMid": "0", "sdpMLineIndex": 0}),
  ("Canais", "post", "/newsletter/create", I, "Cria um canal", {"name": "Novidades", "description": "Ofertas da semana", "picture": ""}),
  ("Canais", "post", "/newsletter/info", I, "Dados do canal pelo id ou link", {"newsletterId": "120363000000000000@newsletter"}),
  ("Canais", "post", "/newsletter/follow", I, "Segue o canal", {"newsletterId": "120363000000000000@newsletter"}),
  ("Canais", "post", "/newsletter/unfollow", I, "Deixa de seguir", {"newsletterId": "120363000000000000@newsletter"}),
  ("Canais", "post", "/newsletter/mute", I, "Silencia o canal", {"newsletterId": "120363000000000000@newsletter"}),
  ("Canais", "post", "/newsletter/unmute", I, "Tira o silêncio", {"newsletterId": "120363000000000000@newsletter"}),
  ("Canais", "post", "/newsletter/messages", I, "Últimas mensagens do canal", {"newsletterId": "120363000000000000@newsletter", "count": 20}),
  ("Histórico", "get", "/chat/list", I, "Conversas salvas (?limit=100&offset=0), as mais recentes primeiro", None),
  ("Histórico", "get", "/chat/messages", I, "Mensagens de uma conversa (?chat=&limit=50&offset=0&before=&after=&fromMe=)", None),
  ("Histórico", "post", "/chat/history", I, "Pede ao celular mensagens mais antigas da conversa (chegam pelo evento HistorySync)", {"chat": N, "count": 50}),
  ("Histórico", "get", "/message/get", I, "Uma mensagem do histórico (?id=)", None),
  ("Histórico", "get", "/message/poll", I, "Votos somados de uma enquete (?id=)", None),
]
R = [r for r in R if "#" not in r[2]]

# padrão Evolution API v2: /<módulo>/<ação>/{instance} com o NOME da instância na URL
V = "nomeada"
V2 = [
  ("Instâncias", "get", "/instance/fetchInstances", "ambas", "Lista as instâncias (filtros ?instanceName= e ?instanceId=)", None),
  ("Instâncias", "get", "/instance/connect/{instance}", V, "QR Code (ou código de pareamento com ?number=)", None),
  ("Instâncias", "get", "/instance/connectionState/{instance}", V, "Estado: open, connecting ou close", None),
  ("Instâncias", "post", "/instance/restart/{instance}", V, "Reconecta", None),
  ("Instâncias", "post", "/instance/setPresence/{instance}", V, "available ou unavailable", {"presence": "available"}),
  ("Instâncias", "delete", "/instance/logout/{instance}", V, "Encerra a sessão no celular", None),
  ("Instâncias", "post", "/settings/set/{instance}", V, "Comportamento", {"rejectCall": True, "msgCall": "Não atendemos ligações", "groupsIgnore": False, "alwaysOnline": False, "readMessages": False}),
  ("Instâncias", "get", "/settings/find/{instance}", V, "Comportamento atual", None),
  ("Instâncias", "post", "/webhook/set/{instance}", V, "Webhook e eventos (nomes MESSAGES_UPSERT, CONNECTION_UPDATE...)",
   {"webhook": {"enabled": True, "url": "https://seu-sistema/webhook", "byEvents": False, "base64": True, "events": ["MESSAGES_UPSERT", "MESSAGES_UPDATE", "CONNECTION_UPDATE", "QRCODE_UPDATED"]}}),
  ("Instâncias", "get", "/webhook/find/{instance}", V, "Webhook atual", None),
  ("Instâncias", "post", "/proxy/set/{instance}", V, "Proxy", {"enabled": True, "host": "proxy.exemplo.com", "port": "8080", "protocol": "http", "username": "", "password": ""}),
  ("Instâncias", "get", "/proxy/find/{instance}", V, "Proxy atual", None),
  ("Envio", "post", "/message/sendText/{instance}", V, "Texto", {"number": N, "text": "Olá!", "delay": 1200, "mentionsEveryOne": False, "mentioned": []}),
  ("Envio", "post", "/message/sendMedia/{instance}", V, "image, video, document ou audio (URL ou base64)", {"number": N, "mediatype": "image", "media": "https://exemplo.com/foto.jpg", "caption": "Legenda", "fileName": "foto.jpg"}),
  ("Envio", "post", "/message/sendPtv/{instance}", V, "Vídeo redondo", {"number": N, "video": "https://exemplo.com/video.mp4"}),
  ("Envio", "post", "/message/sendWhatsAppAudio/{instance}", V, "Nota de voz", {"number": N, "audio": "https://exemplo.com/audio.mp3"}),
  ("Envio", "post", "/message/sendSticker/{instance}", V, "Figurinha", {"number": N, "sticker": "https://exemplo.com/imagem.png"}),
  ("Envio", "post", "/message/sendLocation/{instance}", V, "Localização", {"number": N, "name": "Escritório", "address": "Av. Paulista", "latitude": -23.55, "longitude": -46.63}),
  ("Envio", "post", "/message/sendContact/{instance}", V, "Um ou vários contatos", {"number": N, "contact": [{"fullName": "Suporte", "wuid": "5511988887777", "phoneNumber": "+55 11 98888-7777"}]}),
  ("Envio", "post", "/message/sendReaction/{instance}", V, "Reação", {"key": {"remoteJid": N + "@s.whatsapp.net", "fromMe": False, "id": "3EB0..."}, "reaction": "👍"}),
  ("Envio", "post", "/message/sendPoll/{instance}", V, "Enquete", {"number": N, "name": "Qual horário?", "selectableCount": 1, "values": ["Manhã", "Tarde"]}),
  ("Envio", "post", "/message/sendList/{instance}", V, "Lista", {"number": N, "title": "Cardápio", "description": "Escolha", "buttonText": "Ver", "footerText": "", "sections": [{"title": "Lanches", "rows": [{"title": "X-Burguer", "description": "R$ 20", "rowId": "x1"}]}]}),
  ("Envio", "post", "/message/sendButtons/{instance}", V, "Botões", {"number": N, "title": "Pedido", "description": "Escolha", "footer": "Loja", "buttons": [{"type": "reply", "displayText": "Confirmar", "id": "ok"}]}),
  ("Envio", "post", "/message/sendStatus/{instance}", V, "Status (text, image, video, audio)", {"type": "text", "content": "Novidades!", "backgroundColor": "#1DB954", "font": 1}),
  ("Conversas", "post", "/chat/whatsappNumbers/{instance}", V, "Quais números têm WhatsApp", {"numbers": [N]}),
  ("Conversas", "post", "/chat/markMessageAsRead/{instance}", V, "Marca como lidas", {"readMessages": [{"remoteJid": N + "@s.whatsapp.net", "fromMe": False, "id": "3EB0..."}]}),
  ("Conversas", "post", "/chat/markChatUnread/{instance}", V, "Marca a conversa como não lida", {"chat": N, "lastMessage": {"key": {"remoteJid": N + "@s.whatsapp.net", "fromMe": False, "id": "3EB0..."}}}),
  ("Conversas", "post", "/chat/archiveChat/{instance}", V, "Arquiva ou desarquiva", {"chat": N, "archive": True, "lastMessage": {"key": {"remoteJid": N + "@s.whatsapp.net", "fromMe": False, "id": "3EB0..."}}}),
  ("Conversas", "delete", "/chat/deleteMessageForEveryone/{instance}", V, "Apaga para todos", {"id": "3EB0...", "remoteJid": N + "@s.whatsapp.net", "fromMe": True}),
  ("Conversas", "post", "/chat/updateMessage/{instance}", V, "Edita mensagem enviada", {"number": N, "key": {"remoteJid": N + "@s.whatsapp.net", "fromMe": True, "id": "3EB0..."}, "text": "Corrigido"}),
  ("Conversas", "post", "/chat/sendPresence/{instance}", V, "composing, recording, paused", {"number": N, "presence": "composing", "delay": 3000}),
  ("Conversas", "post", "/chat/updateBlockStatus/{instance}", V, "block ou unblock", {"number": N, "status": "block"}),
  ("Conversas", "post", "/chat/fetchProfilePictureUrl/{instance}", V, "Foto de perfil", {"number": N}),
  ("Conversas", "post", "/chat/fetchProfile/{instance}", V, "Perfil (nome, recado, foto, comercial)", {"number": N}),
  ("Conversas", "post", "/chat/fetchBusinessProfile/{instance}", V, "Perfil comercial", {"number": N}),
  ("Conversas", "post", "/chat/findContacts/{instance}", V, "Contatos da agenda", {"where": {}}),
  ("Conversas", "post", "/chat/findMessages/{instance}", V, "Mensagens do histórico (offset = quantidade por página)", {"where": {"key": {"remoteJid": N + "@s.whatsapp.net"}}, "page": 1, "offset": 50}),
  ("Conversas", "post", "/chat/findChats/{instance}", V, "Conversas do histórico com a última mensagem", {"where": {}}),
  ("Conversas", "post", "/chat/findStatusMessage/{instance}", V, "Status de entrega das mensagens", {"where": {"remoteJid": N + "@s.whatsapp.net", "id": "3EB0..."}}),
  ("Conversas", "post", "/chat/getBase64FromMediaMessage/{instance}", V, "Mídia em base64 (mensagem do webhook ou id recente)", {"message": {"key": {"id": "3EB0..."}}}),
  ("Conversas", "post", "/chat/updateProfileName/{instance}", V, "Nome do perfil", {"name": "Atendimento"}),
  ("Conversas", "post", "/chat/updateProfileStatus/{instance}", V, "Recado", {"status": "Atendimento das 8h às 18h"}),
  ("Conversas", "post", "/chat/updateProfilePicture/{instance}", V, "Foto do perfil", {"picture": "https://exemplo.com/logo.png"}),
  ("Conversas", "delete", "/chat/removeProfilePicture/{instance}", V, "Apaga a foto do perfil", None),
  ("Conversas", "get", "/chat/fetchPrivacySettings/{instance}", V, "Privacidade", None),
  ("Conversas", "post", "/chat/updatePrivacySettings/{instance}", V, "Altera a privacidade", {"readreceipts": "all", "profile": "all", "status": "contacts", "online": "all", "last": "contacts", "groupadd": "contacts"}),
  ("Grupos", "post", "/group/create/{instance}", V, "Cria grupo", {"subject": "Equipe", "description": "Avisos", "participants": [N]}),
  ("Grupos", "post", "/group/updateGroupSubject/{instance}", V, "Nome (?groupJid=)", {"subject": "Equipe 2"}),
  ("Grupos", "post", "/group/updateGroupDescription/{instance}", V, "Descrição (?groupJid=)", {"description": "Avisos"}),
  ("Grupos", "post", "/group/updateGroupPicture/{instance}", V, "Foto (?groupJid=)", {"image": "https://exemplo.com/logo.png"}),
  ("Grupos", "get", "/group/findGroupInfos/{instance}", V, "Dados (?groupJid=)", None),
  ("Grupos", "get", "/group/fetchAllGroups/{instance}", V, "Todos os grupos (?getParticipants=true)", None),
  ("Grupos", "get", "/group/participants/{instance}", V, "Participantes (?groupJid=)", None),
  ("Grupos", "get", "/group/inviteCode/{instance}", V, "Link de convite (?groupJid=)", None),
  ("Grupos", "post", "/group/revokeInviteCode/{instance}", V, "Gera novo link (?groupJid=)", None),
  ("Grupos", "get", "/group/inviteInfo/{instance}", V, "Dados pelo convite (?inviteCode=)", None),
  ("Grupos", "get", "/group/acceptInviteCode/{instance}", V, "Entra pelo convite (?inviteCode=)", None),
  ("Grupos", "post", "/group/sendInvite/{instance}", V, "Envia o convite por mensagem", {"groupJid": "120363000000000000@g.us", "description": "Entre no grupo", "numbers": [N]}),
  ("Grupos", "post", "/group/updateParticipant/{instance}", V, "add, remove, promote, demote (?groupJid=)", {"action": "add", "participants": [N]}),
  ("Grupos", "post", "/group/updateSetting/{instance}", V, "announcement, not_announcement, locked, unlocked (?groupJid=)", {"action": "announcement"}),
  ("Grupos", "post", "/group/toggleEphemeral/{instance}", V, "Mensagens temporárias (?groupJid=)", {"expiration": 604800}),
  ("Grupos", "delete", "/group/leaveGroup/{instance}", V, "Sai do grupo (?groupJid=)", None),
  ("Etiquetas", "get", "/label/findLabels/{instance}", V, "Etiquetas", None),
  ("Etiquetas", "post", "/label/handleLabel/{instance}", V, "add ou remove", {"number": N, "labelId": "1", "action": "add"}),
  ("Chamadas", "post", "/call/offer/{instance}", V, "Faz o número tocar por callDuration segundos e desliga (precisa das chamadas ligadas)", {"number": N, "isVideo": False, "callDuration": 15}),
]
R += [("Padrão v2 · " + t, m, p, a, s_, b) for t, m, p, a, s_, b in V2]

for kind, ex in [("chatwoot", {"enabled": True, "url": "https://chatwoot.exemplo.com", "accountId": "1", "token": "TOKEN", "inboxId": 3}),
                 ("typebot", {"enabled": True, "url": "https://typebot.exemplo.com", "typebot": "meu-fluxo", "triggerType": "all"}),
                 ("n8n", {"enabled": True, "webhookUrl": "https://n8n.exemplo.com/webhook/bob", "basicAuthUser": "", "basicAuthPass": ""})]:
    R += [("Integrações", "post", f"/{kind}/set", I, f"Liga e configura o {kind.capitalize()}", ex),
          ("Integrações", "get", f"/{kind}/find", I, f"Configuração atual do {kind.capitalize()}", None),
          ("Integrações", "delete", f"/{kind}", I, f"Remove o {kind.capitalize()}", None)]

paths = {}
for tag, method, path, auth, summary, body in R:
    op = {"tags": [tag], "summary": summary, "responses": {
        "200": {"description": "Sucesso", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Resposta"}}}},
        "400": {"description": "Pedido inválido", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Erro"}}}},
        "401": {"description": "Chave inválida"}, "409": {"description": "Instância não conectada"}}}
    if auth == A:
        op["security"] = []
    elif auth == G:
        op["security"] = [{"chaveGlobal": []}]
    elif auth == V:
        op["security"] = [{"chaveGlobal": []}, {"tokenInstancia": []}]
    elif auth == I:
        op["security"] = [{"tokenInstancia": []}, {"chaveGlobal": [], "instancia": []}]
    else:
        op["security"] = [{"chaveGlobal": []}, {"tokenInstancia": []}]
    if "{id}" in path:
        op["parameters"] = [{"name": "id", "in": "path", "required": True, "schema": {"type": "string"}, "description": "id ou nome da instância"}]
    if "{instance}" in path:
        op["parameters"] = [{"name": "instance", "in": "path", "required": True, "schema": {"type": "string"}, "description": "nome da instância"}]
    if "{callId}" in path:
        op["parameters"] = [{"name": "callId", "in": "path", "required": True, "schema": {"type": "string"}, "description": "callId da chamada"}]
    if body is not None:
        op["requestBody"] = {"required": True, "content": {"application/json": {"schema": {"type": "object"}, "example": body}}}
    paths.setdefault(path, {})[method] = op

spec = {
  "openapi": "3.0.3",
  "info": {"title": "API WhatsApp BOB", "version": pathlib.Path("VERSION").read_text().strip(),
           "description": "API de WhatsApp livre, escrita em Go sobre a biblioteca whatsmeow. Desenvolvido por Edton Braz.\n\n"
             "**Autenticação**: cabeçalho `apikey`. A chave global (GLOBAL_API_KEY) cria e gerencia instâncias; o token da instância "
             "envia mensagens e conecta. Com a chave global também dá para usar as rotas da instância informando o cabeçalho `instance` (id ou nome).\n\n"
             "**Padrão v2**: as rotas `/<módulo>/<ação>/{instance}` seguem o formato da Evolution API v2, para sistemas já feitos nesse padrão. "
             "Use a chave global ou o token da instância no `apikey`.\n\n"
             "**Eventos**: webhook (POST JSON `{event, instanceId, instanceName, instanceToken, data}`) e WebSocket em `/ws?token=`.",
           "license": {"name": "Apache 2.0", "url": "https://github.com/Edton180/API-Whatsapp-BOB-/blob/main/LICENSE"}},
  "servers": [{"url": "/"}],
  "tags": [{"name": t} for t in dict.fromkeys(r[0] for r in R)],
  "paths": paths,
  "components": {
    "securitySchemes": {
      "chaveGlobal": {"type": "apiKey", "in": "header", "name": "apikey", "description": "GLOBAL_API_KEY"},
      "tokenInstancia": {"type": "apiKey", "in": "header", "name": "apikey", "description": "Token da instância"},
      "instancia": {"type": "apiKey", "in": "header", "name": "instance", "description": "id ou nome da instância (com a chave global)"}},
    "schemas": {
      "Resposta": {"type": "object", "properties": {"data": {}, "message": {"type": "string", "example": "success"}}},
      "Erro": {"type": "object", "properties": {"error": {"type": "string"}, "message": {"type": "string"}}}}},
}
out = pathlib.Path("internal/panel/openapi.yaml")
out.write_text("# Gerado por scripts/gerar-openapi.py\n" + yaml.safe_dump(spec, allow_unicode=True, sort_keys=False, width=140))
import json
pathlib.Path("internal/panel/openapi.json").write_text(json.dumps(spec, ensure_ascii=False))
print(out, len(paths), "rotas")
