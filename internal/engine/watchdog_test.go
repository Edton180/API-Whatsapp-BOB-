// API WhatsApp BOB - Desenvolvido por Edton Braz.

package engine

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestReconnectIdleSkipsSessionsWithoutClient(t *testing.T) {
	s, _ := historySession(t)
	s.e.Cfg.ReconnectIdleHours = 6
	s.e.mu.Lock()
	s.e.sessions[s.ID] = s
	s.e.mu.Unlock()
	atomicSet := time.Now().Add(-10 * time.Hour)
	s.lastEvent = atomicSet.Unix()
	// sem cliente conectado não há o que reconectar
	if done := s.e.reconnectIdle(time.Now()); len(done) != 0 {
		t.Fatalf("não deveria reconectar sessão sem cliente: %v", done)
	}
	s.e.Cfg.ReconnectIdleHours = 0
	if done := s.e.reconnectIdle(time.Now()); done != nil {
		t.Fatal("com RECONNECT_IDLE_HOURS=0 o vigia fica desligado")
	}
}

func TestTouchAndLastEvent(t *testing.T) {
	s, _ := historySession(t)
	if !s.LastEvent().IsZero() {
		t.Fatal("sessão nova não tem evento")
	}
	s.touch()
	if time.Since(s.LastEvent()) > time.Minute {
		t.Fatal("touch deveria marcar agora")
	}
}

func TestRefreshVersionThrottle(t *testing.T) {
	versionMu.Lock()
	versionAt = time.Now()
	versionMu.Unlock()
	if refreshVersionIfStale() {
		t.Fatal("não deve renovar de novo em menos de 10 minutos")
	}
}

func TestWebhookByEventsAndBase64Setting(t *testing.T) {
	got := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got <- r.URL.Path }))
	defer srv.Close()
	s, repo := historySession(t)
	if err := repo.Update(s.ID, map[string]any{"webhook": srv.URL + "/hook", "webhook_by_events": true, "webhook_base64": "off"}); err != nil {
		t.Fatal(err)
	}
	s.emit("Message", map[string]any{"x": 1})
	select {
	case p := <-got:
		if p != "/hook/messages-upsert" {
			t.Fatalf("endereço: %s", p)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("webhook não chegou")
	}
	inst := s.Instance()
	if inst.Base64(true) {
		t.Fatal("base64 desligado na instância deve vencer o WEBHOOK_FILES global")
	}
}
