// API WhatsApp BOB - Desenvolvido por Edton Braz.

package events

import (
	"sync"
)

// Hub distribui os eventos para as conexões WebSocket abertas.
type Hub struct {
	mu   sync.RWMutex
	subs map[chan []byte]string // canal -> id da instância ("" = todas)
}

func NewHub() *Hub { return &Hub{subs: map[chan []byte]string{}} }

// Subscribe abre um canal de eventos de uma instância ("" recebe de todas).
func (h *Hub) Subscribe(instanceID string) chan []byte {
	ch := make(chan []byte, 256)
	h.mu.Lock()
	h.subs[ch] = instanceID
	h.mu.Unlock()
	return ch
}

func (h *Hub) Unsubscribe(ch chan []byte) {
	h.mu.Lock()
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
	h.mu.Unlock()
}

func (h *Hub) Publish(instanceID string, body []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch, id := range h.subs {
		if id != "" && id != instanceID {
			continue
		}
		select {
		case ch <- body:
		default: // conexão lenta: descarta para não travar a BOB
		}
	}
}

// Count devolve quantas conexões estão abertas.
func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs)
}
