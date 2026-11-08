// API WhatsApp BOB - Desenvolvido por Edton Braz.

package events

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestAllowed(t *testing.T) {
	cases := []struct {
		sub, ev string
		want    bool
	}{
		{"", "Message", true},
		{"ALL", "Receipt", true},
		{"MESSAGE", "Message", true},
		{"MESSAGE", "Receipt", false},
		{"message, read_receipt", "Receipt", true},
		{"CONNECTION", "LoggedOut", true},
		{"QRCODE", "QRCode", true},
		{"Message", "Message", true},
	}
	for _, c := range cases {
		if got := Allowed(c.sub, c.ev); got != c.want {
			t.Errorf("Allowed(%q, %q) = %v", c.sub, c.ev, got)
		}
	}
	if NormalizeSubscribe([]string{"message", " MESSAGE", "", "call"}) != "MESSAGE,CALL" {
		t.Error("NormalizeSubscribe")
	}
}

func TestWebhookOrderAndFormat(t *testing.T) {
	var mu sync.Mutex
	got := []Payload{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var p Payload
		_ = json.Unmarshal(b, &p)
		mu.Lock()
		got = append(got, p)
		mu.Unlock()
	}))
	defer srv.Close()
	d := NewDispatcher()
	hooked := 0
	d.AddHook(func(string, []byte) { hooked++ })
	for i := 0; i < 20; i++ {
		d.Emit(Payload{Event: "Message", InstanceID: "i1", InstanceName: "vendas", Data: map[string]int{"n": i}}, srv.URL, "MESSAGE")
	}
	d.Emit(Payload{Event: "Receipt", InstanceID: "i1"}, srv.URL, "MESSAGE") // filtrado
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n >= 20 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 20 {
		t.Fatalf("esperava 20 eventos, chegaram %d", len(got))
	}
	for i, p := range got {
		n := int(p.Data.(map[string]any)["n"].(float64))
		if n != i || p.Event != "Message" || p.InstanceName != "vendas" || p.Date == "" {
			t.Fatalf("evento %d fora de ordem ou incompleto: %+v", i, p)
		}
	}
	if hooked != 21 {
		t.Fatalf("hooks devem receber todos os eventos, receberam %d", hooked)
	}
}

func TestHub(t *testing.T) {
	h := NewHub()
	a, all := h.Subscribe("a"), h.Subscribe("")
	h.Publish("b", []byte("x"))
	if len(a) != 0 || len(all) != 1 {
		t.Fatal("filtro do WebSocket por instância")
	}
	h.Unsubscribe(a)
	h.Unsubscribe(all)
	if h.Count() != 0 {
		t.Fatal("Unsubscribe")
	}
}

func TestV2Names(t *testing.T) {
	cases := map[string]string{"Message": "messages-upsert", "Connected": "connection-update", "QRCode": "qrcode-updated", "Receipt": "messages-update", "SendMessage": "send-message"}
	for ev, path := range cases {
		if got := V2Path(ev); got != path {
			t.Errorf("%s: %s, esperado %s", ev, got, path)
		}
	}
	tgt := Target{URL: "https://x.com/hook/", ByEvents: true}
	if u := tgt.URLFor("Message"); u != "https://x.com/hook/messages-upsert" {
		t.Fatalf("url por evento: %s", u)
	}
	if u := (Target{URL: "https://x.com/hook"}).URLFor("Message"); u != "https://x.com/hook" {
		t.Fatalf("url única: %s", u)
	}
}
