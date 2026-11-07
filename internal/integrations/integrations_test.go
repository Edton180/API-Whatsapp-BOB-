package integrations

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	instance_model "github.com/edton180/api-whatsapp-bob/internal/store"
)

type sent struct{ To, Text, URL, Type string }

type fakeSender struct {
	mu  sync.Mutex
	out []sent
	n   int
}

func (f *fakeSender) Text(_ *instance_model.Instance, to, text string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	f.out = append(f.out, sent{To: to, Text: text})
	return fmt.Sprintf("OUT%d", f.n), nil
}

func (f *fakeSender) Media(_ *instance_model.Instance, to, url, typ, caption string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	f.out = append(f.out, sent{To: to, Text: caption, URL: url, Type: typ})
	return fmt.Sprintf("OUT%d", f.n), nil
}

var inst = &instance_model.Instance{Id: "11111111-1111-1111-1111-111111111111", Name: "bob", Token: "tok"}

func msg(id, text string, fromMe bool) []byte {
	b, _ := json.Marshal(map[string]any{
		"event": "Message",
		"data": map[string]any{
			"Info":    map[string]any{"Chat": "5594988887777@s.whatsapp.net", "IsFromMe": fromMe, "IsGroup": false, "ID": id, "PushName": "Cliente"},
			"Message": map[string]any{"conversation": text},
		},
	})
	return b
}

func newManager(t *testing.T) (*Manager, *fakeSender, *gin.Engine) {
	gin.SetMode(gin.TestMode)
	fs := &fakeSender{}
	m := &Manager{Store: NewMemoryStore(), Sender: fs, Sync: true,
		Instance: func(id string) (*instance_model.Instance, error) { return inst, nil }}
	r := gin.New()
	m.Register(r, func(c *gin.Context) {
		if c.GetHeader("apikey") != inst.Token {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Set("instance", inst)
	})
	return m, fs, r
}

func do(r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("apikey", "tok")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestParseMessage(t *testing.T) {
	in, ok := ParseMessage(msg("A1", "oi", false))
	if !ok || in.Number != "5594988887777" || in.Text != "oi" || in.FromMe {
		t.Fatalf("parse errado: %+v", in)
	}
	img := []byte(`{"event":"Message","data":{"Info":{"Chat":"55@s.whatsapp.net","ID":"X"},"Message":{"imageMessage":{"caption":"foto"}}}}`)
	in, _ = ParseMessage(img)
	if in.MediaType != "image" || in.Text != "foto" {
		t.Fatalf("mídia: %+v", in)
	}
	if _, ok := ParseMessage([]byte(`{"event":"Receipt"}`)); ok {
		t.Fatal("evento que não é mensagem não deve passar")
	}
	if _, ok := ParseMessage([]byte(`{"event":"Message","data":{"Info":{"Chat":"status@broadcast"}}}`)); ok {
		t.Fatal("status não deve passar")
	}
}

func TestTypebotSessao(t *testing.T) {
	var calls []string
	tb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		calls = append(calls, r.URL.Path+" "+string(body))
		if strings.HasSuffix(r.URL.Path, "/startChat") {
			fmt.Fprint(w, `{"sessionId":"S1","messages":[{"type":"text","content":{"richText":[{"type":"p","children":[{"text":"Olá "},{"text":"Cliente","bold":true}]}]}},{"type":"image","content":{"url":"https://x/img.png"}}],"input":{"type":"choice input","items":[{"content":"Vendas"},{"content":"Suporte"}]}}`)
			return
		}
		fmt.Fprint(w, `{"messages":[{"type":"text","content":{"richText":[{"type":"p","children":[{"text":"Certo"}]}]}}]}`)
	}))
	defer tb.Close()
	m, fs, r := newManager(t)

	if w := do(r, "POST", "/typebot/set", map[string]any{"url": tb.URL, "typebot": "meu-fluxo", "keywordFinish": "#sair"}); w.Code != 200 {
		t.Fatalf("set: %d %s", w.Code, w.Body)
	}
	m.Handle(inst, msg("A1", "oi", false))
	if len(fs.out) != 3 || fs.out[0].Text != "Olá *Cliente*" || fs.out[1].URL != "https://x/img.png" || fs.out[2].Text != "1. Vendas\n2. Suporte" {
		t.Fatalf("respostas do início: %+v", fs.out)
	}
	m.Handle(inst, msg("A2", "1", false))
	if !strings.Contains(calls[1], "/api/v1/sessions/S1/continueChat") || fs.out[3].Text != "Certo" {
		t.Fatalf("continua sessão: %v %+v", calls, fs.out)
	}
	m.Handle(inst, msg("A3", "#sair", false))
	if s, _ := m.Store.GetSession(inst.Id, KindTypebot, "5594988887777@s.whatsapp.net"); s != nil {
		t.Fatal("palavra de saída deve encerrar a sessão")
	}
	// Mensagem enviada pelo próprio celular não aciona o bot
	m.Handle(inst, msg("A4", "oi", true))
	if len(calls) != 2 {
		t.Fatalf("fromMe acionou o bot: %v", calls)
	}
}

func TestN8NRespostaEGatilho(t *testing.T) {
	var got map[string]any
	var auth string
	n8n := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got)
		fmt.Fprint(w, `[{"output":"Primeira\n\n\nSegunda"}]`)
	}))
	defer n8n.Close()
	m, fs, r := newManager(t)
	do(r, "POST", "/n8n/set", map[string]any{"webhookUrl": n8n.URL, "triggerType": "keyword", "triggerValue": "pedido", "basicAuthUser": "u", "basicAuthPass": "p"})

	m.Handle(inst, msg("B1", "bom dia", false))
	if got != nil {
		t.Fatal("sem a palavra-chave o N8N não deve ser chamado")
	}
	m.Handle(inst, msg("B2", "quero fazer um pedido", false))
	if got["chatInput"] != "quero fazer um pedido" || got["remoteJid"] != "5594988887777@s.whatsapp.net" || got["instanceName"] != "bob" {
		t.Fatalf("corpo enviado ao N8N: %v", got)
	}
	if !strings.HasPrefix(auth, "Basic ") {
		t.Fatal("autenticação básica ausente")
	}
	if len(fs.out) != 2 || fs.out[0].Text != "Primeira" || fs.out[1].Text != "Segunda" {
		t.Fatalf("respostas: %+v", fs.out)
	}
	// Com sessão aberta, a próxima mensagem vai ao N8N mesmo sem a palavra-chave
	got = nil
	m.Handle(inst, msg("B3", "dois", false))
	if got == nil {
		t.Fatal("sessão aberta deve continuar no N8N")
	}
	w := do(r, "GET", "/n8n/find", nil)
	if !strings.Contains(w.Body.String(), `"basicAuthPass":"********"`) {
		t.Fatalf("segredo exposto: %s", w.Body)
	}
}

func TestChatwootIdaEVolta(t *testing.T) {
	var mu sync.Mutex
	var messages []map[string]any
	contacts := 0
	cw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("api_access_token") != "cwtoken" {
			w.WriteHeader(401)
			return
		}
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/contacts/search"):
			if contacts == 0 {
				fmt.Fprint(w, `{"payload":[]}`)
			} else {
				fmt.Fprint(w, `{"payload":[{"id":7,"phone_number":"+5594988887777","identifier":"5594988887777@s.whatsapp.net","contact_inboxes":[{"source_id":"src","inbox":{"id":3}}]}]}`)
			}
		case p == "/api/v1/accounts/1/contacts" && r.Method == "POST":
			contacts++
			fmt.Fprint(w, `{"payload":{"contact":{"id":7},"contact_inbox":{"source_id":"src"}}}`)
		case strings.HasSuffix(p, "/contacts/7/conversations"):
			fmt.Fprint(w, `{"payload":[]}`)
		case p == "/api/v1/accounts/1/conversations":
			fmt.Fprint(w, `{"id":42}`)
		case strings.HasSuffix(p, "/conversations/42/messages"):
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			messages = append(messages, b)
			fmt.Fprint(w, `{"id":1}`)
		default:
			w.WriteHeader(404)
		}
	}))
	defer cw.Close()
	m, fs, r := newManager(t)

	w := do(r, "POST", "/chatwoot/set", map[string]any{"url": cw.URL, "accountId": "1", "token": "cwtoken", "inboxId": 3, "signMsg": true})
	if w.Code != 200 {
		t.Fatalf("set: %d %s", w.Code, w.Body)
	}
	var view struct {
		WebhookUrl string `json:"webhookUrl"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &view)
	if !strings.Contains(view.WebhookUrl, "/chatwoot/webhook/"+inst.Id+"/") {
		t.Fatalf("webhook: %s", view.WebhookUrl)
	}

	m.Handle(inst, msg("C1", "Olá atendente", false))
	m.Handle(inst, msg("C2", "mandei do celular", true))
	if len(messages) != 2 || messages[0]["content"] != "Olá atendente" || messages[0]["message_type"] != "incoming" || messages[1]["message_type"] != "outgoing" {
		t.Fatalf("mensagens no Chatwoot: %v", messages)
	}
	if contacts != 1 {
		t.Fatalf("contato deve ser criado uma vez: %d", contacts)
	}

	path := strings.SplitN(view.WebhookUrl, "://", 2)[1]
	path = path[strings.Index(path, "/"):]
	ev := map[string]any{
		"event": "message_created", "message_type": "outgoing", "private": false, "content": "Posso ajudar?",
		"sender": map[string]any{"name": "Ana"}, "inbox": map[string]any{"id": 3},
		"conversation": map[string]any{"id": 42, "meta": map[string]any{"sender": map[string]any{"phone_number": "+5594988887777", "identifier": "5594988887777@s.whatsapp.net"}}},
	}
	if w := do(r, "POST", path, ev); w.Code != 200 {
		t.Fatalf("webhook: %d %s", w.Code, w.Body)
	}
	if len(fs.out) != 1 || fs.out[0].Text != "*Ana:*\nPosso ajudar?" || fs.out[0].To != "5594988887777@s.whatsapp.net" {
		t.Fatalf("envio ao WhatsApp: %+v", fs.out)
	}
	// O eco da mensagem enviada não volta ao Chatwoot
	m.Handle(inst, msg("OUT1", "*Ana:*\nPosso ajudar?", true))
	if len(messages) != 2 {
		t.Fatalf("eco voltou ao Chatwoot: %v", messages)
	}
	// Mensagem importada do celular (source_id WAID:) e nota privada não são reenviadas
	ev["source_id"] = "WAID:C2"
	do(r, "POST", path, ev)
	delete(ev, "source_id")
	ev["private"] = true
	do(r, "POST", path, ev)
	if len(fs.out) != 1 {
		t.Fatalf("reenvio indevido: %+v", fs.out)
	}
	if w := do(r, "POST", "/chatwoot/webhook/"+inst.Id+"/errado", ev); w.Code != 403 {
		t.Fatalf("segredo errado deve dar 403: %d", w.Code)
	}
}

func TestSetValidaCampos(t *testing.T) {
	_, _, r := newManager(t)
	if w := do(r, "POST", "/chatwoot/set", map[string]any{"url": "ftp://x"}); w.Code != 400 {
		t.Fatalf("esperava 400: %d", w.Code)
	}
	req := httptest.NewRequest("GET", "/typebot/find", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("sem apikey deve dar 401: %d", w.Code)
	}
}
