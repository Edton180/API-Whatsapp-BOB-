// API WhatsApp BOB - Desenvolvido por Edton Braz.

package api

import (
	"strings"
	"testing"
)

func TestV2InstanceFlow(t *testing.T) {
	h := newServer(t)

	// criar no padrão v2
	r := call(t, h, "POST", "/instance/create", key, map[string]any{
		"instanceName": "loja", "token": "TOKEN-LOJA", "rejectCall": true, "msgCall": "Não atendo ligações",
		"webhook": map[string]any{"url": "https://exemplo.com/hook", "events": []string{"MESSAGES_UPSERT", "CONNECTION_UPDATE"}},
	})
	if r.Code != 201 {
		t.Fatalf("create v2: %d %s", r.Code, r.Raw)
	}
	if r.Body["hash"] != "TOKEN-LOJA" {
		t.Fatalf("hash: %v", r.Body["hash"])
	}
	if inst, _ := r.Body["instance"].(map[string]any); inst["instanceName"] != "loja" {
		t.Fatalf("instance: %v", r.Body["instance"])
	}

	// nome repetido
	if r := call(t, h, "POST", "/instance/create", key, map[string]any{"instanceName": "loja"}); r.Code != 403 {
		t.Fatalf("nome repetido: %d", r.Code)
	}

	// criação nativa continua funcionando
	if r := call(t, h, "POST", "/instance/create", key, map[string]any{"name": "nativa", "token": "TOKEN-NATIVA"}); r.Code != 200 {
		t.Fatalf("create nativo: %d %s", r.Code, r.Raw)
	}

	// fetchInstances com chave global e com token
	r = call(t, h, "GET", "/instance/fetchInstances", key, nil)
	if r.Code != 200 || !strings.Contains(r.Raw, `"name":"loja"`) || !strings.Contains(r.Raw, `"name":"nativa"`) {
		t.Fatalf("fetchInstances global: %d %s", r.Code, r.Raw)
	}
	r = call(t, h, "GET", "/instance/fetchInstances", "TOKEN-LOJA", nil)
	if r.Code != 200 || strings.Contains(r.Raw, "nativa") {
		t.Fatalf("fetchInstances token: %d %s", r.Code, r.Raw)
	}
	if r := call(t, h, "GET", "/instance/fetchInstances", "errado", nil); r.Code != 401 {
		t.Fatalf("fetchInstances sem chave: %d", r.Code)
	}

	// autenticação por nome na URL
	if r := call(t, h, "GET", "/instance/connectionState/loja", "TOKEN-LOJA", nil); r.Code != 200 || !strings.Contains(r.Raw, `"state":"close"`) {
		t.Fatalf("connectionState: %d %s", r.Code, r.Raw)
	}
	if r := call(t, h, "GET", "/instance/connectionState/loja", "TOKEN-NATIVA", nil); r.Code != 401 {
		t.Fatalf("token de outra instância deveria falhar: %d", r.Code)
	}
	if r := call(t, h, "GET", "/instance/connectionState/nao-existe", key, nil); r.Code != 404 {
		t.Fatalf("instância inexistente: %d", r.Code)
	}

	// configurações
	r = call(t, h, "GET", "/settings/find/loja", key, nil)
	if r.Body["rejectCall"] != true || r.Body["msgCall"] != "Não atendo ligações" {
		t.Fatalf("settings/find: %s", r.Raw)
	}
	if r := call(t, h, "POST", "/settings/set/loja", key, map[string]any{"alwaysOnline": true, "groupsIgnore": true}); r.Code != 201 {
		t.Fatalf("settings/set: %d %s", r.Code, r.Raw)
	}
	r = call(t, h, "GET", "/settings/find/loja", key, nil)
	if r.Body["alwaysOnline"] != true || r.Body["groupsIgnore"] != true || r.Body["rejectCall"] != false {
		t.Fatalf("settings após set: %s", r.Raw)
	}

	// webhook
	r = call(t, h, "GET", "/webhook/find/loja", key, nil)
	if r.Body["url"] != "https://exemplo.com/hook" || !strings.Contains(r.Raw, "MESSAGES_UPSERT") || !strings.Contains(r.Raw, "CONNECTION_UPDATE") {
		t.Fatalf("webhook/find: %s", r.Raw)
	}
	r = call(t, h, "POST", "/webhook/set/loja", key, map[string]any{"webhook": map[string]any{
		"enabled": true, "url": "https://outro.com/w", "events": []string{"GROUP_UPDATE", "SEND_MESSAGE"},
	}})
	if r.Code != 201 || r.Body["url"] != "https://outro.com/w" {
		t.Fatalf("webhook/set: %d %s", r.Code, r.Raw)
	}
	if r := call(t, h, "POST", "/webhook/set/loja", key, map[string]any{"url": "ftp://x"}); r.Code != 400 {
		t.Fatalf("webhook com URL inválida: %d", r.Code)
	}
	r = call(t, h, "POST", "/webhook/set/loja", key, map[string]any{"enabled": false})
	if r.Body["enabled"] != false {
		t.Fatalf("webhook desligado: %s", r.Raw)
	}

	// proxy
	if r := call(t, h, "POST", "/proxy/set/loja", key, map[string]any{"enabled": true, "host": "1.2.3.4", "port": "3128", "protocol": "http"}); r.Code != 201 {
		t.Fatalf("proxy/set: %d %s", r.Code, r.Raw)
	}
	if r := call(t, h, "GET", "/proxy/find/loja", key, nil); r.Body["host"] != "1.2.3.4" || r.Body["enabled"] != true {
		t.Fatalf("proxy/find: %s", r.Raw)
	}

	// envio sem estar conectado ao WhatsApp devolve erro claro, não sucesso falso
	r = call(t, h, "POST", "/message/sendText/loja", key, map[string]any{"number": "5511999999999", "text": "oi"})
	if r.Code != 400 || r.Body["status"] != float64(400) {
		t.Fatalf("sendText desconectado: %d %s", r.Code, r.Raw)
	}
	if r := call(t, h, "POST", "/message/sendText/loja", key, map[string]any{"number": "5511999999999"}); r.Code != 400 {
		t.Fatalf("sendText sem texto: %d", r.Code)
	}
	if r := call(t, h, "GET", "/group/participants/loja", key, nil); r.Code != 400 || !strings.Contains(r.Raw, "groupJid") {
		t.Fatalf("participants sem groupJid: %d %s", r.Code, r.Raw)
	}
	if r := call(t, h, "POST", "/call/offer/loja", key, map[string]any{}); r.Code != 400 {
		t.Fatalf("call/offer: %d", r.Code)
	}

	// excluir pelo nome (rota nativa aceita nome ou id)
	if r := call(t, h, "DELETE", "/instance/delete/loja", key, nil); r.Code != 200 {
		t.Fatalf("delete: %d %s", r.Code, r.Raw)
	}
}

func TestV2EventMapping(t *testing.T) {
	got := strings.Join(v2ToCategories([]string{"MESSAGES_UPSERT", "messages.update", "PRESENCE_UPDATE", "GROUP_UPDATE", "GROUPS_UPSERT", "xyz"}), ",")
	if got != "MESSAGE,READ_RECEIPT,PRESENCE,CHAT_PRESENCE,GROUP" {
		t.Fatalf("v2ToCategories = %s", got)
	}
	if got := v2ToCategories([]string{"MESSAGES_UPSERT", "ALL"}); len(got) != 1 || got[0] != "ALL" {
		t.Fatalf("ALL = %v", got)
	}
	if got := strings.Join(v2FromCategories("MESSAGE,READ_RECEIPT"), ","); got != "MESSAGES_UPSERT,MESSAGES_UPDATE" {
		t.Fatalf("v2FromCategories = %s", got)
	}
}

func TestParseColor(t *testing.T) {
	cases := map[string]uint32{"#1DB954": 0xFF1DB954, "80FF0000": 0x80FF0000, "": 0, "zz": 0}
	for in, want := range cases {
		if got := parseColor(in); got != want {
			t.Errorf("parseColor(%q) = %X, want %X", in, got, want)
		}
	}
}

func TestExtraRoutesExist(t *testing.T) {
	h := newServer(t)
	if r := call(t, h, "POST", "/instance/create", key, map[string]any{"name": "x", "token": "TK"}); r.Code != 200 {
		t.Fatalf("create: %d", r.Code)
	}
	for _, p := range []string{
		"/group/settings", "/group/ephemeral", "/group/inviteinfo", "/group/requests", "/group/requests/update",
		"/group/photo/remove", "/chat/ephemeral", "/community/create", "/community/link", "/community/unlink",
		"/community/groups", "/user/business", "/user/privacy", "/instance/presence", "/call/reject",
		"/newsletter/create", "/newsletter/info", "/newsletter/follow", "/newsletter/unfollow",
		"/newsletter/mute", "/newsletter/unmute", "/newsletter/messages",
	} {
		r := call(t, h, "POST", p, "TK", map[string]any{})
		if r.Code == 404 || r.Code == 401 {
			t.Errorf("%s: %d %s", p, r.Code, r.Raw)
		}
	}
	if r := call(t, h, "DELETE", "/user/profilePicture", "TK", nil); r.Code == 404 {
		t.Errorf("DELETE /user/profilePicture: %d", r.Code)
	}
}
