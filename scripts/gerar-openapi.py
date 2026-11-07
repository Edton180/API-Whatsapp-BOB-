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
  ("Instâncias", "post", "/instance/{id}/webhook", "ambas", "Troca o webhook e os eventos sem reconectar", {"webhook": "https://seu-sistema/webhook", "subscribe": ["MESSAGE", "READ_RECEIPT", "CONNECTION"]}),
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
    elif auth == I:
        op["security"] = [{"tokenInstancia": []}, {"chaveGlobal": [], "instancia": []}]
    else:
        op["security"] = [{"chaveGlobal": []}, {"tokenInstancia": []}]
    if "{id}" in path:
        op["parameters"] = [{"name": "id", "in": "path", "required": True, "schema": {"type": "string"}, "description": "id ou nome da instância"}]
    if body is not None:
        op["requestBody"] = {"required": True, "content": {"application/json": {"schema": {"type": "object"}, "example": body}}}
    paths.setdefault(path, {})[method] = op

spec = {
  "openapi": "3.0.3",
  "info": {"title": "API WhatsApp BOB", "version": pathlib.Path("VERSION").read_text().strip(),
           "description": "API de WhatsApp livre, escrita em Go sobre a biblioteca whatsmeow. Desenvolvido por Edton Braz.\n\n"
             "**Autenticação**: cabeçalho `apikey`. A chave global (GLOBAL_API_KEY) cria e gerencia instâncias; o token da instância "
             "envia mensagens e conecta. Com a chave global também dá para usar as rotas da instância informando o cabeçalho `instance` (id ou nome).\n\n"
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
