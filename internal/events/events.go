// API WhatsApp BOB - Desenvolvido por Edton Braz.

// Package events entrega os eventos das instâncias: webhook, WebSocket e integrações.
package events

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Payload é o formato de todo evento enviado pela BOB.
//
//	{"event": "Message", "instanceId": "...", "instanceName": "...", "instanceToken": "...", "data": {...}}
type Payload struct {
	Event         string `json:"event"`
	InstanceID    string `json:"instanceId"`
	InstanceName  string `json:"instanceName"`
	InstanceToken string `json:"instanceToken"`
	Data          any    `json:"data"`
	Date          string `json:"date_time"`
}

// Categorias aceitas em "subscribe" ao conectar a instância.
var categories = map[string][]string{
	"MESSAGE":       {"Message", "PollVote", "UndecryptableMessage", "MediaRetry"},
	"SEND_MESSAGE":  {"SendMessage"},
	"READ_RECEIPT":  {"Receipt"},
	"PRESENCE":      {"Presence"},
	"CHAT_PRESENCE": {"ChatPresence"},
	"HISTORY_SYNC":  {"HistorySync"},
	"CALL":          {"CallOffer", "CallAccept", "CallTerminate", "CallOfferNotice", "CallRelayLatency"},
	"CONNECTION":    {"Connected", "PairSuccess", "PairError", "LoggedOut", "Disconnected", "ConnectFailure", "TemporaryBan", "StreamReplaced", "ClientOutdated", "KeepAliveTimeout", "KeepAliveRestored", "OfflineSyncCompleted"},
	"QRCODE":        {"QRCode", "QRTimeout", "QRSuccess"},
	"CONTACT":       {"Contact", "PushName", "Picture", "BusinessName", "UserAbout"},
	"GROUP":         {"GroupInfo", "JoinedGroup"},
	"LABEL":         {"LabelEdit", "LabelAssociationChat", "LabelAssociationMessage"},
	"NEWSLETTER":    {"NewsletterJoin", "NewsletterLeave", "NewsletterMuteChange", "NewsletterLiveUpdate"},
	"CHAT":          {"Archive", "Pin", "Mute", "Star", "DeleteChat", "DeleteForMe", "MarkChatAsRead", "ClearChat"},
	"BLOCKLIST":     {"Blocklist"},
}

// Categories devolve os nomes de categoria aceitos.
func Categories() []string {
	out := make([]string, 0, len(categories)+1)
	out = append(out, "ALL")
	for _, k := range []string{"MESSAGE", "SEND_MESSAGE", "READ_RECEIPT", "PRESENCE", "CHAT_PRESENCE", "HISTORY_SYNC", "CALL", "CONNECTION", "QRCODE", "CONTACT", "GROUP", "LABEL", "NEWSLETTER", "CHAT", "BLOCKLIST"} {
		out = append(out, k)
	}
	return out
}

// Allowed diz se o evento entra na lista de inscrições (vazia ou ALL = todos).
func Allowed(subscribed string, event string) bool {
	subscribed = strings.TrimSpace(subscribed)
	if subscribed == "" {
		return true
	}
	for _, cat := range strings.Split(subscribed, ",") {
		cat = strings.ToUpper(strings.TrimSpace(cat))
		if cat == "ALL" {
			return true
		}
		if strings.EqualFold(cat, event) {
			return true
		}
		for _, ev := range categories[cat] {
			if ev == event {
				return true
			}
		}
	}
	return false
}

// NormalizeSubscribe limpa e junta a lista de categorias.
func NormalizeSubscribe(list []string) string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range list {
		v = strings.ToUpper(strings.TrimSpace(v))
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return strings.Join(out, ",")
}

// Hook recebe todos os eventos (integrações Chatwoot, Typebot, N8N...).
type Hook func(instanceID string, body []byte)

// Dispatcher entrega os eventos. Cada destino de webhook tem uma fila própria, então a ordem
// das mensagens é mantida e um destino lento não atrasa os outros.
type Dispatcher struct {
	HTTP  *http.Client
	Hub   *Hub
	hooks []Hook

	mu     sync.Mutex
	queues map[string]chan []byte
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		HTTP:   &http.Client{Timeout: 30 * time.Second},
		Hub:    NewHub(),
		queues: map[string]chan []byte{},
	}
}

// AddHook registra um consumidor de todos os eventos.
func (d *Dispatcher) AddHook(h Hook) { d.hooks = append(d.hooks, h) }

// Emit envia o evento para o webhook (se houver), para o WebSocket e para os hooks.
func (d *Dispatcher) Emit(p Payload, webhookURL string, subscribed string) {
	if p.Date == "" {
		p.Date = time.Now().Format(time.RFC3339)
	}
	body, err := json.Marshal(p)
	if err != nil {
		log.Printf("[eventos] não consegui montar %s: %v", p.Event, err)
		return
	}
	for _, h := range d.hooks {
		h(p.InstanceID, body)
	}
	if !Allowed(subscribed, p.Event) {
		return
	}
	d.Hub.Publish(p.InstanceID, body)
	if webhookURL != "" {
		d.enqueue(webhookURL, body)
	}
}

func (d *Dispatcher) enqueue(url string, body []byte) {
	d.mu.Lock()
	q, ok := d.queues[url]
	if !ok {
		q = make(chan []byte, 2000)
		d.queues[url] = q
		go d.worker(url, q)
	}
	d.mu.Unlock()
	select {
	case q <- body:
	default:
		log.Printf("[webhook] fila cheia para %s, evento descartado", url)
	}
}

func (d *Dispatcher) worker(url string, q chan []byte) {
	for body := range q {
		d.post(url, body)
	}
}

// post tenta entregar até 4 vezes (1 s, 3 s, 9 s entre as tentativas).
func (d *Dispatcher) post(url string, body []byte) {
	wait := time.Second
	for try := 1; try <= 4; try++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "API-WhatsApp-BOB")
		res, err := d.HTTP.Do(req)
		if err == nil {
			res.Body.Close()
		}
		cancel()
		if err == nil && res.StatusCode < 500 {
			return
		}
		if try == 4 {
			if err != nil {
				log.Printf("[webhook] %s não respondeu: %v", url, err)
			} else {
				log.Printf("[webhook] %s respondeu %d", url, res.StatusCode)
			}
			return
		}
		time.Sleep(wait)
		wait *= 3
	}
}
