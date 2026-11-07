package integrations

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
)

// Sender envia mensagens pela instância (no servidor, é o serviço de envio da Evolution Go).
type Sender interface {
	Text(inst *instance_model.Instance, number, text string) (string, error)
	Media(inst *instance_model.Instance, number, url, mediaType, caption string) (string, error)
}

// Manager liga os eventos do WhatsApp às integrações.
type Manager struct {
	Store    Store
	Sender   Sender
	Instance func(id string) (*instance_model.Instance, error)
	HTTP     *http.Client
	Sync     bool // testes: processa na hora, sem goroutine

	sent  sync.Map // ids de mensagens enviadas pelas integrações (evita eco)
	locks sync.Map
}

var current *Manager

// SetDefault registra o gerenciador usado pelo motor de eventos.
func SetDefault(m *Manager) { current = m }

// Dispatch é chamado pelo motor para cada evento entregue à instância. Nunca bloqueia o motor.
func Dispatch(inst *instance_model.Instance, payload []byte) {
	if current == nil || inst == nil {
		return
	}
	current.Handle(inst, payload)
}

// Incoming é a mensagem já simplificada.
type Incoming struct {
	ID        string
	RemoteJid string
	Number    string
	PushName  string
	FromMe    bool
	IsGroup   bool
	Text      string
	MediaType string
}

// ParseMessage lê o evento "Message" da Evolution Go.
func ParseMessage(payload []byte) (*Incoming, bool) {
	var ev struct {
		Event string `json:"event"`
		Data  struct {
			Info struct {
				Chat     string `json:"Chat"`
				IsFromMe bool   `json:"IsFromMe"`
				IsGroup  bool   `json:"IsGroup"`
				ID       string `json:"ID"`
				PushName string `json:"PushName"`
			} `json:"Info"`
			Message map[string]json.RawMessage `json:"Message"`
		} `json:"data"`
	}
	if json.Unmarshal(payload, &ev) != nil || ev.Event != "Message" || ev.Data.Info.Chat == "" {
		return nil, false
	}
	in := &Incoming{
		ID:        ev.Data.Info.ID,
		RemoteJid: ev.Data.Info.Chat,
		Number:    strings.SplitN(strings.SplitN(ev.Data.Info.Chat, "@", 2)[0], ":", 2)[0],
		PushName:  ev.Data.Info.PushName,
		FromMe:    ev.Data.Info.IsFromMe,
		IsGroup:   ev.Data.Info.IsGroup || strings.HasSuffix(ev.Data.Info.Chat, "@g.us"),
	}
	if strings.HasPrefix(in.RemoteJid, "status@") || strings.HasSuffix(in.RemoteJid, "@newsletter") {
		return nil, false
	}
	msg := ev.Data.Message
	var s string
	if raw, ok := msg["conversation"]; ok && json.Unmarshal(raw, &s) == nil {
		in.Text = s
	}
	for _, k := range []string{"extendedTextMessage", "imageMessage", "videoMessage", "documentMessage", "audioMessage", "stickerMessage", "buttonsResponseMessage", "listResponseMessage"} {
		raw, ok := msg[k]
		if !ok {
			continue
		}
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		for _, f := range []string{"text", "caption", "selectedDisplayText", "title"} {
			if v, ok := m[f].(string); ok && v != "" && in.Text == "" {
				in.Text = v
			}
		}
		if k != "extendedTextMessage" && k != "buttonsResponseMessage" && k != "listResponseMessage" {
			in.MediaType = strings.TrimSuffix(k, "Message")
		}
	}
	return in, true
}

func (m *Manager) Handle(inst *instance_model.Instance, payload []byte) {
	in, ok := ParseMessage(payload)
	if !ok {
		return
	}
	run := func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[bob-integrations] erro: %v", r)
			}
		}()
		// Uma mensagem por vez por contato, para manter a ordem do bot.
		mu, _ := m.locks.LoadOrStore(inst.Id+"|"+in.RemoteJid, &sync.Mutex{})
		mu.(*sync.Mutex).Lock()
		defer mu.(*sync.Mutex).Unlock()
		m.process(inst, in)
	}
	if m.Sync {
		run()
	} else {
		go run()
	}
}

func (m *Manager) process(inst *instance_model.Instance, in *Incoming) {
	echo := false
	if in.FromMe {
		_, echo = m.sent.LoadAndDelete(in.ID)
	}
	if it, err := m.Store.Get(inst.Id, KindChatwoot); err == nil && it.Enabled && !echo {
		if err := m.chatwootInbound(inst, it, in); err != nil {
			log.Printf("[bob-integrations] chatwoot %s: %v", inst.Id, err)
		}
	}
	if echo {
		return
	}
	for _, kind := range []string{KindTypebot, KindN8N} {
		it, err := m.Store.Get(inst.Id, kind)
		if err != nil || !it.Enabled {
			continue
		}
		if err := m.bot(inst, it, in); err != nil {
			log.Printf("[bob-integrations] %s %s: %v", kind, inst.Id, err)
		}
		return // só um bot responde por mensagem (Typebot tem prioridade)
	}
}

func (m *Manager) remember(id string) {
	if id == "" {
		return
	}
	now := time.Now()
	m.sent.Store(id, now)
	m.sent.Range(func(k, v any) bool { // limpa ids antigos
		if now.Sub(v.(time.Time)) > 10*time.Minute {
			m.sent.Delete(k)
		}
		return true
	})
}

func (m *Manager) sendText(inst *instance_model.Instance, number, text string) error {
	id, err := m.Sender.Text(inst, number, text)
	m.remember(id)
	return err
}

func (m *Manager) sendMedia(inst *instance_model.Instance, number, url, typ, caption string) error {
	id, err := m.Sender.Media(inst, number, url, typ, caption)
	m.remember(id)
	return err
}

func (m *Manager) client() *http.Client {
	if m.HTTP != nil {
		return m.HTTP
	}
	return &http.Client{Timeout: 60 * time.Second}
}

// call faz uma requisição JSON e decodifica a resposta em out (se não for nil).
func (m *Manager) call(method, url string, headers map[string]string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := m.client().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("%s %s: HTTP %d: %s", method, url, res.StatusCode, truncate(string(data), 300))
	}
	if out != nil && len(bytes.TrimSpace(data)) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
