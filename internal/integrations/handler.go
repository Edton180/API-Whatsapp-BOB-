package integrations

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	instance_model "github.com/edton180/api-whatsapp-bob/internal/store"
)

// Register liga as rotas. auth é o middleware de token da instância (cabeçalho apikey).
//
//	POST/GET/DELETE /chatwoot  /typebot  /n8n   (também /set e /find)
//	POST /chatwoot/webhook/:instanceId/:secret  (chamado pelo Chatwoot)
func (m *Manager) Register(eng *gin.Engine, auth gin.HandlerFunc) {
	for _, kind := range []string{KindChatwoot, KindTypebot, KindN8N} {
		k := kind
		g := eng.Group("/"+k, auth)
		g.POST("/set", m.set(k))
		g.POST("", m.set(k))
		g.GET("/find", m.find(k))
		g.GET("", m.find(k))
		g.DELETE("", m.remove(k))
		if k != KindChatwoot {
			g.POST("/session/clear", m.clearSession(k))
		}
	}
	eng.POST("/chatwoot/webhook/:instanceId/:secret", m.chatwootWebhook)
}

func instanceFrom(c *gin.Context) *instance_model.Instance {
	v, ok := c.Get("instance")
	if !ok {
		return nil
	}
	inst, _ := v.(*instance_model.Instance)
	return inst
}

type setRequest struct {
	Enabled *bool `json:"enabled"`
}

func validURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func (m *Manager) set(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		inst := instanceFrom(c)
		if inst == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
			return
		}
		raw, err := c.GetRawData()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		var req setRequest
		_ = json.Unmarshal(raw, &req)
		enabled := req.Enabled == nil || *req.Enabled

		// Segredos em branco (ou mascarados) mantêm o valor já salvo: o painel não recebe os segredos.
		old, _ := m.Store.Get(inst.Id, kind)
		keep := func(v string, prev func() string) string {
			if (v == "" || v == "********") && old != nil {
				return prev()
			}
			return v
		}
		var cfg any
		switch kind {
		case KindChatwoot:
			var cw ChatwootConfig
			_ = json.Unmarshal(raw, &cw)
			cw.Token = keep(cw.Token, func() string { p, _ := decode[ChatwootConfig](old); return p.Token })
			if !validURL(cw.Url) || !validURL(cw.Url) || cw.AccountId == "" || cw.Token == "" || cw.InboxId == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "informe url, accountId, token e inboxId do Chatwoot"})
				return
			}
			if old, err := m.Store.Get(inst.Id, kind); err == nil {
				if prev, err := decode[ChatwootConfig](old); err == nil {
					cw.WebhookSecret = prev.WebhookSecret
				}
			}
			if cw.WebhookSecret == "" {
				cw.WebhookSecret = randomSecret()
			}
			cfg = cw
		case KindTypebot:
			var tb TypebotConfig
			err := json.Unmarshal(raw, &tb)
			tb.ApiToken = keep(tb.ApiToken, func() string { p, _ := decode[TypebotConfig](old); return p.ApiToken })
			if err != nil || !validURL(tb.Url) || tb.Typebot == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "informe url e typebot (id público do fluxo)"})
				return
			}
			cfg = tb
		case KindN8N:
			var n N8NConfig
			err := json.Unmarshal(raw, &n)
			n.BasicPass = keep(n.BasicPass, func() string { p, _ := decode[N8NConfig](old); return p.BasicPass })
			if err != nil || !validURL(n.WebhookUrl) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "informe webhookUrl do N8N"})
				return
			}
			cfg = n
		}
		b, _ := json.Marshal(cfg)
		it := &Integration{InstanceId: inst.Id, Kind: kind, Enabled: enabled, Config: string(b), UpdatedAt: time.Now()}
		if err := m.Store.Save(it); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, m.view(c, inst, it))
	}
}

func (m *Manager) find(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		inst := instanceFrom(c)
		if inst == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
			return
		}
		it, err := m.Store.Get(inst.Id, kind)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"kind": kind, "enabled": false, "configured": false})
			return
		}
		c.JSON(http.StatusOK, m.view(c, inst, it))
	}
}

func (m *Manager) remove(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		inst := instanceFrom(c)
		if inst == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
			return
		}
		if err := m.Store.Delete(inst.Id, kind); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"kind": kind, "deleted": true})
	}
}

func (m *Manager) clearSession(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		inst := instanceFrom(c)
		var req struct {
			RemoteJid string `json:"remoteJid"`
		}
		if inst == nil || c.ShouldBindJSON(&req) != nil || req.RemoteJid == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "informe remoteJid"})
			return
		}
		_ = m.Store.DeleteSession(inst.Id, kind, req.RemoteJid)
		c.JSON(http.StatusOK, gin.H{"cleared": true})
	}
}

// view esconde segredos e, no Chatwoot, mostra o endereço do webhook a configurar na caixa de entrada.
func (m *Manager) view(c *gin.Context, inst *instance_model.Instance, it *Integration) gin.H {
	var cfg map[string]any
	_ = json.Unmarshal([]byte(it.Config), &cfg)
	for _, k := range []string{"token", "apiToken", "basicAuthPass", "webhookSecret"} {
		if v, ok := cfg[k].(string); ok && v != "" {
			cfg[k] = "********"
		}
	}
	out := gin.H{"kind": it.Kind, "enabled": it.Enabled, "configured": true, "config": cfg, "updatedAt": it.UpdatedAt}
	if it.Kind == KindChatwoot {
		if cw, err := decode[ChatwootConfig](it); err == nil {
			out["webhookUrl"] = publicBase(c) + "/chatwoot/webhook/" + inst.Id + "/" + cw.WebhookSecret
		}
	}
	return out
}

func publicBase(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := c.GetHeader("X-Forwarded-Host")
	if host == "" {
		host = c.Request.Host
	}
	return scheme + "://" + host
}

func randomSecret() string {
	b := make([]byte, 20)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (m *Manager) chatwootWebhook(c *gin.Context) {
	id := c.Param("instanceId")
	it, err := m.Store.Get(id, KindChatwoot)
	if err != nil || !it.Enabled {
		c.JSON(http.StatusNotFound, gin.H{"error": "integração não encontrada"})
		return
	}
	cfg, err := decode[ChatwootConfig](it)
	if err != nil || cfg.WebhookSecret == "" || subtle.ConstantTimeCompare([]byte(cfg.WebhookSecret), []byte(c.Param("secret"))) != 1 {
		c.JSON(http.StatusForbidden, gin.H{"error": "segredo inválido"})
		return
	}
	var ev ChatwootEvent
	if err := c.ShouldBindJSON(&ev); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	inst, err := m.Instance(id)
	if err != nil || inst == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "instância não encontrada"})
		return
	}
	if err := m.ChatwootOutbound(inst, it, ev); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
