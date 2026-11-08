// API WhatsApp BOB - Desenvolvido por Edton Braz.

package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestCallRoutes(t *testing.T) {
	h := newServer(t)
	r := call(t, h, "POST", "/instance/create", key, map[string]any{"name": "fone", "token": "TOKEN-FONE"})
	if r.Code != 200 {
		t.Fatal(r.Raw)
	}
	tok := "TOKEN-FONE"

	r = call(t, h, "GET", "/call/settings", tok, nil)
	if r.Code != 200 || data(r)["enabled"] != false || data(r)["available"] != true {
		t.Fatalf("settings: %s", r.Raw)
	}
	if r := call(t, h, "POST", "/call/settings", tok, map[string]any{}); r.Code != 400 {
		t.Fatalf("sem enabled: %d", r.Code)
	}
	// Instância sem número conectado: liga sem reconectar.
	r = call(t, h, "POST", "/call/settings", tok, map[string]any{"enabled": true})
	if r.Code != 200 || data(r)["enabled"] != true || data(r)["reconnected"] != false {
		t.Fatalf("ligar: %s", r.Raw)
	}
	if r := call(t, h, "GET", "/instance/info/fone", key, nil); data(r)["callsEnabled"] != true {
		t.Fatalf("callsEnabled não foi gravado: %s", r.Raw)
	}

	r = call(t, h, "GET", "/call/ice-servers?agent=ana", tok, nil)
	if r.Code != 200 || !strings.Contains(r.Raw, "stun:") || strings.Contains(r.Raw, "credential") {
		t.Fatalf("ice: %s", r.Raw)
	}

	r = call(t, h, "GET", "/call/list?status=missed&number=%2B55%20(11)%2099999-0000", tok, nil)
	if r.Code != 200 || data(r)["total"] != float64(0) {
		t.Fatalf("list: %s", r.Raw)
	}
	if r := call(t, h, "GET", "/call/live", tok, nil); r.Code != 200 {
		t.Fatalf("live: %s", r.Raw)
	}
	if r := call(t, h, "GET", "/call/info/NAOEXISTE", tok, nil); r.Code != http.StatusNotFound {
		t.Fatalf("info: %d", r.Code)
	}

	// Ações sem a oferta do navegador ou numa chamada que não existe.
	if r := call(t, h, "POST", "/call/answer", tok, map[string]any{"callId": "X"}); r.Code != 400 {
		t.Fatalf("answer sem sdp: %d", r.Code)
	}
	if r := call(t, h, "POST", "/call/answer", tok, map[string]any{"callId": "X", "sdp": "v=0"}); r.Code != http.StatusNotFound {
		t.Fatalf("answer: %d %s", r.Code, r.Raw)
	}
	for _, p := range []string{"/call/hangup", "/call/mute", "/call/camera"} {
		if r := call(t, h, "POST", p, tok, map[string]any{"callId": "X"}); r.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", p, r.Code)
		}
	}
	// Ligar precisa de um número conectado.
	if r := call(t, h, "POST", "/call/start", tok, map[string]any{"number": "5511999990000", "sdp": "v=0\r\nm=audio 9 x 0\r\na=rtpmap:0 PCMU/8000"}); r.Code == 200 {
		t.Fatalf("start sem conexão: %s", r.Raw)
	}
	// Sem chamadas ligadas: 412 explica onde ligar.
	call(t, h, "POST", "/call/settings", tok, map[string]any{"enabled": false})
	if r := call(t, h, "POST", "/call/offer/fone", tok, map[string]any{"number": "5511999990000"}); r.Code != http.StatusPreconditionFailed {
		t.Fatalf("offer v2: %d %s", r.Code, r.Raw)
	}
	// Outra instância não vê as chamadas desta.
	if r := call(t, h, "GET", "/call/list", "token-errado", nil); r.Code != http.StatusUnauthorized {
		t.Fatalf("token errado: %d", r.Code)
	}
}
