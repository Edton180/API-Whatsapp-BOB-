// API WhatsApp BOB - Desenvolvido por Edton Braz.

package api

import (
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"

	"github.com/edton180/api-whatsapp-bob/internal/config"
	"github.com/edton180/api-whatsapp-bob/internal/engine"
	"github.com/edton180/api-whatsapp-bob/internal/events"
	"github.com/edton180/api-whatsapp-bob/internal/integrations"
	"github.com/edton180/api-whatsapp-bob/internal/store"
)

func TestHistoryRoutes(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{APIKey: key, DataDir: dir, QRMaxCount: 5, StoreMessages: true}
	db, err := store.Open("", dir)
	if err != nil {
		t.Fatal(err)
	}
	repo := &store.Repo{DB: db}
	eng, err := engine.New(cfg, repo, events.NewDispatcher())
	if err != nil {
		t.Fatal(err)
	}
	is, _ := integrations.NewGormStore(db)
	h := (&Server{Engine: eng, Repo: repo, Integrations: &integrations.Manager{Store: is, Instance: repo.ByID}}).Router()

	inst := &store.Instance{Name: "hist", Token: "TOKEN-HIST"}
	if err := repo.Create(inst); err != nil {
		t.Fatal(err)
	}
	const chat = "5511999998888@s.whatsapp.net"
	raw, _ := proto.Marshal(&waE2E.Message{Conversation: proto.String("oi")})
	for i, id := range []string{"M1", "M2", "M3"} {
		m := &store.Message{InstanceID: inst.Id, ID: id, Chat: chat, Sender: chat, FromMe: id == "M2", Type: "conversation", Text: "oi", Status: "READ", Timestamp: int64(10 + i), Raw: raw}
		_ = repo.SaveMessage(m)
		_ = repo.TouchChat(inst.Id, chat, "Cliente", m, !m.FromMe)
	}

	r := call(t, h, "GET", "/chat/messages?chat="+chat+"&limit=2", "TOKEN-HIST", nil)
	if r.Code != 200 || data(r)["total"] != float64(3) || len(data(r)["messages"].([]any)) != 2 {
		t.Fatalf("chat/messages: %d %s", r.Code, r.Raw)
	}
	if !strings.Contains(r.Raw, `"conversation":"oi"`) || !strings.Contains(r.Raw, `"remoteJid":"`+chat+`"`) {
		t.Fatalf("formato da mensagem: %s", r.Raw)
	}
	if r := call(t, h, "GET", "/chat/list", "TOKEN-HIST", nil); r.Code != 200 || !strings.Contains(r.Raw, `"unreadCount":2`) {
		t.Fatalf("chat/list: %d %s", r.Code, r.Raw)
	}
	if r := call(t, h, "GET", "/message/get?id=M2", "TOKEN-HIST", nil); r.Code != 200 || data(r)["key"].(map[string]any)["fromMe"] != true {
		t.Fatalf("message/get: %d %s", r.Code, r.Raw)
	}
	if r := call(t, h, "GET", "/message/get?id=nada", "TOKEN-HIST", nil); r.Code != 404 {
		t.Fatalf("message/get inexistente: %d", r.Code)
	}

	// padrão v2
	r = call(t, h, "POST", "/chat/findMessages/hist", "TOKEN-HIST", map[string]any{
		"where": map[string]any{"key": map[string]any{"remoteJid": chat, "fromMe": false}}, "page": 1, "offset": 1,
	})
	msgs, _ := r.Body["messages"].(map[string]any)
	if r.Code != 200 || msgs["total"] != float64(2) || msgs["pages"] != float64(2) || len(msgs["records"].([]any)) != 1 {
		t.Fatalf("findMessages: %d %s", r.Code, r.Raw)
	}
	if r := call(t, h, "POST", "/chat/findChats/hist", "TOKEN-HIST", map[string]any{}); r.Code != 200 || !strings.Contains(r.Raw, `"lastMessage"`) || !strings.Contains(r.Raw, `"id":"M3"`) {
		t.Fatalf("findChats: %d %s", r.Code, r.Raw)
	}
	if r := call(t, h, "POST", "/chat/findStatusMessage/hist", "TOKEN-HIST", map[string]any{"where": map[string]any{"remoteJid": chat, "id": "M2"}}); r.Code != 200 || !strings.Contains(r.Raw, `"status":"READ"`) {
		t.Fatalf("findStatusMessage: %d %s", r.Code, r.Raw)
	}

	// pedir histórico com a instância desconectada
	if r := call(t, h, "POST", "/chat/history", "TOKEN-HIST", map[string]any{"chat": chat}); r.Code != 409 {
		t.Fatalf("chat/history desconectada: %d %s", r.Code, r.Raw)
	}
}

func TestHistoryOff(t *testing.T) {
	h := newServer(t)
	call(t, h, "POST", "/instance/create", key, map[string]any{"name": "sem", "token": "TOKEN-SEM"})
	if r := call(t, h, "GET", "/chat/list", "TOKEN-SEM", nil); r.Code != 400 || !strings.Contains(r.Raw, "STORE_MESSAGES") {
		t.Fatalf("histórico desligado: %d %s", r.Code, r.Raw)
	}
}
