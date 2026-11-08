// API WhatsApp BOB - Desenvolvido por Edton Braz.

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/edton180/api-whatsapp-bob/internal/calls"
	"github.com/edton180/api-whatsapp-bob/internal/config"
	"github.com/edton180/api-whatsapp-bob/internal/engine"
	"github.com/edton180/api-whatsapp-bob/internal/events"
	"github.com/edton180/api-whatsapp-bob/internal/integrations"
	"github.com/edton180/api-whatsapp-bob/internal/panel"
	"github.com/edton180/api-whatsapp-bob/internal/store"
)

const key = "chave-de-teste"

func newServer(t *testing.T) http.Handler {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{APIKey: key, DataDir: dir, PanelEnabled: true, QRMaxCount: 5, OSName: "BOB",
		WebRTC: config.WebRTC{ICEServers: []string{"stun:stun.l.google.com:19302"}}}
	db, err := store.Open("", dir)
	if err != nil {
		t.Fatal(err)
	}
	repo := &store.Repo{DB: db}
	eng, err := engine.New(cfg, repo, events.NewDispatcher())
	if err != nil {
		t.Fatal(err)
	}
	is, err := integrations.NewGormStore(db)
	if err != nil {
		t.Fatal(err)
	}
	mgr := &integrations.Manager{Store: is, Instance: repo.ByID}
	cm, err := calls.New(eng, repo, cfg.WebRTC)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cm.Close() })
	eng.Calls = cm
	return (&Server{Engine: eng, Repo: repo, Integrations: mgr, Calls: cm}).Router()
}

type resp struct {
	Code int
	Body map[string]any
	Raw  string
}

func call(t *testing.T, h http.Handler, method, path, apikey string, body any, headers ...string) resp {
	t.Helper()
	var r *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		r = httptest.NewRequest(method, path, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	if apikey != "" {
		r.Header.Set("apikey", apikey)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	out := resp{Code: w.Code, Raw: w.Body.String()}
	_ = json.Unmarshal(w.Body.Bytes(), &out.Body)
	return out
}

func data(r resp) map[string]any { m, _ := r.Body["data"].(map[string]any); return m }

func TestPublicRoutes(t *testing.T) {
	h := newServer(t)
	if r := call(t, h, "GET", "/server/ok", "", nil); r.Body["status"] != "ok" {
		t.Fatal("/server/ok")
	}
	if r := call(t, h, "GET", "/license/status", "", nil); r.Body["status"] != "active" {
		t.Fatal("licença deve responder active para compatibilidade")
	}
	r := call(t, h, "GET", "/", "", nil)
	if r.Body["name"] != "API WhatsApp BOB" || strings.Contains(strings.ToLower(r.Raw), "evolution") {
		t.Fatalf("página inicial: %s", r.Raw)
	}
	if r := call(t, h, "GET", "/", "", nil, "Accept", "text/html"); r.Code != http.StatusFound {
		t.Fatal("navegador deve ir para o painel")
	}
	for _, p := range []string{"/painel/", "/painel/app.js", "/painel/logo.jpg", "/openapi.yaml", "/openapi.json", "/swagger/index.html"} {
		if r := call(t, h, "GET", p, "", nil); r.Code != http.StatusOK {
			t.Fatalf("%s respondeu %d", p, r.Code)
		}
	}
	if r := call(t, h, "GET", "/donate/pix", "", nil); !strings.Contains(r.Raw, "br.gov.bcb.pix") {
		t.Fatal("pix")
	}
}

func TestInstanceLifecycle(t *testing.T) {
	h := newServer(t)
	if r := call(t, h, "GET", "/instance/all", "errada", nil); r.Code != http.StatusUnauthorized {
		t.Fatal("chave errada deve dar 401")
	}
	r := call(t, h, "POST", "/instance/create", key, map[string]any{"name": "vendas", "webhook": "https://x.test/hook", "events": []string{"MESSAGE"}, "advancedSettings": map[string]any{"rejectCall": true}})
	if r.Code != 200 {
		t.Fatalf("create: %s", r.Raw)
	}
	inst := data(r)
	id, token := inst["id"].(string), inst["token"].(string)
	if id == "" || len(token) != 32 || inst["state"] != "close" || inst["rejectCall"] != true {
		t.Fatalf("instância criada: %v", inst)
	}
	if r := call(t, h, "POST", "/instance/create", key, map[string]any{"name": "vendas"}); r.Code != 400 {
		t.Fatal("nome repetido deve falhar")
	}
	if r := call(t, h, "GET", "/instance/info/vendas", key, nil); data(r)["id"] != id {
		t.Fatal("info pelo nome")
	}
	list := call(t, h, "GET", "/instance/all", key, nil).Body["data"].([]any)
	if len(list) != 1 {
		t.Fatal("all")
	}

	// token da instância
	st := call(t, h, "GET", "/instance/status", token, nil)
	if data(st)["LoggedIn"] != false || data(st)["Connected"] != false {
		t.Fatalf("status: %s", st.Raw)
	}
	if r := call(t, h, "GET", "/instance/status", "token-errado", nil); r.Code != 401 {
		t.Fatal("token errado")
	}
	// chave global + cabeçalho instance
	if r := call(t, h, "GET", "/instance/status", key, nil, "instance", "vendas"); r.Code != 200 {
		t.Fatal("chave global com cabeçalho instance")
	}
	if r := call(t, h, "GET", "/instance/qr", token, nil); r.Code != 404 {
		t.Fatal("sem QR gerado deve dar 404")
	}
	if r := call(t, h, "POST", "/send/text", token, map[string]any{"number": "5511999998888", "text": "oi"}); r.Code != http.StatusConflict {
		t.Fatalf("enviar sem conexão deve dar 409: %d %s", r.Code, r.Raw)
	}
	if r := call(t, h, "POST", "/send/text", token, map[string]any{"number": "5511999998888"}); r.Code != 400 {
		t.Fatal("texto vazio")
	}

	// configurações, webhook e proxy
	if r := call(t, h, "PUT", "/instance/"+id+"/advanced-settings", token, map[string]any{"readMessages": true, "ignoreGroups": true}); r.Code != 200 {
		t.Fatal(r.Raw)
	}
	adv := data(call(t, h, "GET", "/instance/"+id+"/advanced-settings", key, nil))
	if adv["readMessages"] != true || adv["rejectCall"] != true {
		t.Fatalf("configurações: %v", adv)
	}
	if r := call(t, h, "POST", "/instance/"+id+"/webhook", key, map[string]any{"webhook": "https://y.test", "subscribe": []string{"MESSAGE", "CONNECTION"}}); r.Code != 200 {
		t.Fatal(r.Raw)
	}
	if got := data(call(t, h, "GET", "/instance/info/"+id, key, nil)); got["webhook"] != "https://y.test" || got["events"] != "MESSAGE,CONNECTION" {
		t.Fatalf("webhook: %v", got)
	}
	if r := call(t, h, "POST", "/instance/proxy/"+id, key, map[string]any{"host": "proxy.test", "port": "3128"}); r.Code != 200 {
		t.Fatal(r.Raw)
	}
	if r := call(t, h, "DELETE", "/instance/proxy/"+id, key, nil); r.Code != 200 {
		t.Fatal(r.Raw)
	}
	if r := call(t, h, "GET", "/instance/logs/"+id, key, nil); r.Code != 200 {
		t.Fatal(r.Raw)
	}

	// integrações: segredo não volta e é mantido quando vem em branco
	cw := map[string]any{"url": "https://cw.test", "accountId": "1", "token": "segredo", "inboxId": 3}
	if r := call(t, h, "POST", "/chatwoot/set", token, cw); r.Code != 200 {
		t.Fatal(r.Raw)
	}
	cw["token"] = ""
	if r := call(t, h, "POST", "/chatwoot/set", token, cw); r.Code != 200 {
		t.Fatalf("token em branco deve manter o atual: %s", r.Raw)
	}
	f := call(t, h, "GET", "/chatwoot/find", token, nil)
	if strings.Contains(f.Raw, "segredo") || f.Body["configured"] != true {
		t.Fatalf("find: %s", f.Raw)
	}

	if r := call(t, h, "DELETE", "/instance/delete/"+id, key, nil); r.Code != 200 {
		t.Fatal(r.Raw)
	}
	if r := call(t, h, "GET", "/instance/status", token, nil); r.Code != 401 {
		t.Fatal("instância apagada")
	}
}

// Toda rota documentada precisa existir no servidor.
func TestEveryDocumentedRouteExists(t *testing.T) {
	h := newServer(t)
	var spec struct {
		Paths map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(panel.OpenAPIJSON(), &spec); err != nil {
		t.Fatal(err)
	}
	if len(spec.Paths) < 70 {
		t.Fatalf("poucas rotas documentadas: %d", len(spec.Paths))
	}
	for path, ops := range spec.Paths {
		for method := range ops {
			p := strings.ReplaceAll(path, "{id}", "nao-existe")
			r := call(t, h, strings.ToUpper(method), p, "x", nil)
			if strings.Contains(r.Raw, "rota não encontrada") {
				t.Errorf("%s %s documentada mas não existe", strings.ToUpper(method), path)
			}
		}
	}
}
