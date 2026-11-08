// API WhatsApp BOB - Desenvolvido por Edton Braz.

package engine

// Cuidados com a conexão que vieram dos problemas mais relatados pela comunidade de APIs
// não oficiais:
//   - o WhatsApp recusa cliente com versão velha (erro 405, "cliente desatualizado") e o QR
//     Code não aparece: a versão do WhatsApp Web é renovada sozinha e a sessão reconecta;
//   - números "zumbis", que aparecem conectados mas param de receber eventos: quem fica
//     tempo demais sem nenhum evento é reconectado (sem QR Code).

import (
	"log"
	"sync"
	"sync/atomic"
	"time"
)

var versionMu sync.Mutex
var versionAt time.Time

// refreshVersionIfStale renova a versão do WhatsApp Web, no máximo uma vez a cada 10 minutos.
func refreshVersionIfStale() bool {
	versionMu.Lock()
	defer versionMu.Unlock()
	if time.Since(versionAt) < 10*time.Minute {
		return false
	}
	versionAt = time.Now()
	refreshVersion()
	return true
}

// touch marca que a sessão recebeu um evento do WhatsApp agora.
func (s *Session) touch() { atomic.StoreInt64(&s.lastEvent, time.Now().Unix()) }

// LastEvent devolve quando chegou o último evento do WhatsApp (zero se nenhum).
func (s *Session) LastEvent() time.Time {
	if v := atomic.LoadInt64(&s.lastEvent); v > 0 {
		return time.Unix(v, 0)
	}
	return time.Time{}
}

// onOutdated trata o erro 405: renova a versão e reconecta a sessão.
func (s *Session) onOutdated() {
	s.logf("ERROR", "o WhatsApp recusou a versão do cliente (405); renovando a versão do WhatsApp Web e reconectando")
	refreshVersionIfStale()
	go func() {
		time.Sleep(5 * time.Second)
		if err := s.Reconnect(); err != nil {
			s.logf("ERROR", "reconexão depois do 405 falhou: %v", err)
		}
	}()
}

// maintain roda para sempre: renova a versão do WhatsApp Web a cada 6 horas e reconecta os
// números conectados que estão sem eventos há mais de RECONNECT_IDLE_HOURS.
func (e *Engine) maintain() {
	tick := time.NewTicker(10 * time.Minute)
	defer tick.Stop()
	lastVersion := time.Now()
	for range tick.C {
		if time.Since(lastVersion) >= 6*time.Hour {
			lastVersion = time.Now()
			refreshVersionIfStale()
		}
		e.reconnectIdle(time.Now())
	}
}

// reconnectIdle reconecta as sessões logadas e conectadas sem eventos há tempo demais.
func (e *Engine) reconnectIdle(now time.Time) []string {
	hours := e.Cfg.ReconnectIdleHours
	if hours <= 0 {
		return nil
	}
	limit := time.Duration(hours) * time.Hour
	e.mu.RLock()
	list := make([]*Session, 0, len(e.sessions))
	for _, s := range e.sessions {
		list = append(list, s)
	}
	e.mu.RUnlock()
	done := []string{}
	for _, s := range list {
		c := s.client()
		if c == nil || !c.IsLoggedIn() || !c.IsConnected() {
			continue
		}
		last := s.LastEvent()
		if last.IsZero() || now.Sub(last) < limit {
			continue
		}
		s.logf("WARN", "sem eventos do WhatsApp há %s; reconectando (sem QR Code)", now.Sub(last).Round(time.Minute))
		s.touch()
		if err := s.Reconnect(); err != nil {
			log.Printf("[motor] %s: reconexão falhou: %v", s.Instance().Name, err)
		}
		done = append(done, s.ID)
	}
	return done
}
