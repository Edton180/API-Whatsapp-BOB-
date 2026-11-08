// API WhatsApp BOB - Desenvolvido por Edton Braz.

package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/edton180/api-whatsapp-bob/internal/engine"
	"github.com/edton180/api-whatsapp-bob/internal/store"
)

// view junta os dados salvos com o estado da conexão em tempo real.
func (s *Server) view(inst *store.Instance) map[string]any {
	out := map[string]any{}
	b, _ := json.Marshal(inst)
	_ = json.Unmarshal(b, &out)
	delete(out, "qrcode")
	connected, logged := false, false
	if sess := s.Engine.Lookup(inst.Id); sess != nil {
		st := sess.Status()
		connected, _ = st["Connected"].(bool)
		logged, _ = st["LoggedIn"].(bool)
		if n, _ := st["Name"].(string); n != "" {
			out["profileName"] = n
		}
	}
	out["connected"] = connected && logged
	out["loggedIn"] = logged
	state := "close"
	switch {
	case connected && logged:
		state = "open"
	case connected:
		state = "connecting"
	}
	out["state"] = state
	if inst.Jid != "" {
		out["phone"] = strings.SplitN(strings.SplitN(inst.Jid, "@", 2)[0], ":", 2)[0]
	}
	out["advancedSettings"] = inst.Settings()
	out["webhookFilesDefault"] = s.Engine.Cfg.WebhookFiles
	return out
}

type createRequest struct {
	Name             string                  `json:"name"`
	Token            string                  `json:"token"`
	InstanceID       string                  `json:"instanceId"`
	Webhook          string                  `json:"webhook"`
	Events           []string                `json:"events"`
	AdvancedSettings *store.AdvancedSettings `json:"advancedSettings"`
	Proxy            map[string]any          `json:"proxy"`
	WebhookByEvents  bool                    `json:"webhookByEvents"`
	WebhookBase64    *bool                   `json:"webhookBase64"`
}

func (s *Server) instanceCreate(c *gin.Context) {
	var req createRequest
	if !bind(c, &req) {
		return
	}
	inst := &store.Instance{
		Id: req.InstanceID, Name: req.Name, Token: strings.TrimSpace(req.Token), Webhook: strings.TrimSpace(req.Webhook),
		ClientName: s.Engine.Cfg.ClientName, OsName: s.Engine.Cfg.OSName,
	}
	if len(req.Events) > 0 {
		inst.Events = strings.Join(req.Events, ",")
	}
	if req.AdvancedSettings != nil {
		inst.Apply(*req.AdvancedSettings)
	}
	inst.WebhookByEvents, inst.WebhookBase64 = req.WebhookByEvents, store.Base64Setting(req.WebhookBase64)
	if len(req.Proxy) > 0 {
		b, _ := json.Marshal(req.Proxy)
		inst.Proxy = string(b)
	}
	if err := s.Repo.Create(inst); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	ok(c, s.view(inst))
}

func (s *Server) instanceAll(c *gin.Context) {
	list, err := s.Repo.All()
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, inst := range list {
		out = append(out, s.view(inst))
	}
	ok(c, out)
}

func (s *Server) byParam(c *gin.Context) *store.Instance {
	inst, err := s.Repo.ByIDOrName(c.Param("id"))
	if err != nil {
		fail(c, http.StatusNotFound, err)
		return nil
	}
	return inst
}

func (s *Server) instanceInfo(c *gin.Context) {
	if inst := s.byParam(c); inst != nil {
		ok(c, s.view(inst))
	}
}

func (s *Server) instanceDelete(c *gin.Context) {
	inst := s.byParam(c)
	if inst == nil {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	if err := s.Engine.Delete(ctx, inst); err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	ok(c, gin.H{"deleted": inst.Id})
}

func (s *Server) instanceForceReconnect(c *gin.Context) {
	inst := s.byParam(c)
	if inst == nil {
		return
	}
	sess := s.Engine.Session(inst)
	if err := sess.Reconnect(); err != nil {
		failEngine(c, err)
		return
	}
	ok(c, sess.Status())
}

func (s *Server) instanceProxySet(c *gin.Context) {
	inst := s.byParam(c)
	if inst == nil {
		return
	}
	var p struct {
		Protocol string `json:"protocol"`
		Host     string `json:"host"`
		Port     string `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !bind(c, &p) {
		return
	}
	if strings.TrimSpace(p.Host) == "" {
		fail(c, http.StatusBadRequest, errors.New("informe o host do proxy"))
		return
	}
	b, _ := json.Marshal(p)
	if err := s.Repo.Update(inst.Id, map[string]any{"proxy": string(b)}); err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	if sess := s.Engine.Lookup(inst.Id); sess != nil {
		if err := sess.ApplyProxy(string(b)); err != nil {
			failEngine(c, err)
			return
		}
	}
	ok(c, gin.H{"proxy": p.Host + ":" + p.Port})
}

func (s *Server) instanceProxyDelete(c *gin.Context) {
	inst := s.byParam(c)
	if inst == nil {
		return
	}
	_ = s.Repo.Update(inst.Id, map[string]any{"proxy": ""})
	if sess := s.Engine.Lookup(inst.Id); sess != nil {
		_ = sess.ApplyProxy("")
	}
	ok(c, gin.H{"proxy": nil})
}

func (s *Server) instanceLogs(c *gin.Context) {
	inst := s.byParam(c)
	if inst == nil {
		return
	}
	if sess := s.Engine.Lookup(inst.Id); sess != nil {
		ok(c, sess.Logs())
		return
	}
	ok(c, []engine.LogEntry{})
}

func (s *Server) settingsGet(c *gin.Context) { ok(c, current(c).Settings()) }

func (s *Server) settingsPut(c *gin.Context) {
	inst := current(c)
	settings := inst.Settings()
	if !bind(c, &settings) {
		return
	}
	inst.Apply(settings)
	if err := s.Repo.Update(inst.Id, map[string]any{
		"always_online": settings.AlwaysOnline, "reject_call": settings.RejectCall, "msg_reject_call": settings.MsgRejectCall,
		"read_messages": settings.ReadMessages, "ignore_groups": settings.IgnoreGroups, "ignore_status": settings.IgnoreStatus,
	}); err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	if sess := s.Engine.Lookup(inst.Id); sess != nil {
		fresh, _ := s.Repo.ByID(inst.Id)
		if fresh != nil {
			s.Engine.Session(fresh)
		}
	}
	ok(c, settings)
}

// webhookSet troca o webhook e os eventos sem reconectar (usado pelo painel).
func (s *Server) webhookSet(c *gin.Context) {
	inst := current(c)
	var req struct {
		Webhook   string   `json:"webhook"`
		Subscribe []string `json:"subscribe"`
		ByEvents  *bool    `json:"byEvents"`
		Base64    *bool    `json:"base64"`
	}
	if !bind(c, &req) {
		return
	}
	events := strings.Join(req.Subscribe, ",")
	fields := map[string]any{"webhook": strings.TrimSpace(req.Webhook), "events": events}
	if req.ByEvents != nil {
		fields["webhook_by_events"] = *req.ByEvents
	}
	if req.Base64 != nil {
		fields["webhook_base64"] = store.Base64Setting(req.Base64)
	}
	if err := s.Repo.Update(inst.Id, fields); err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	fresh, err := s.Repo.ByID(inst.Id)
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	if s.Engine.Lookup(inst.Id) != nil {
		s.Engine.Session(fresh)
	}
	ok(c, gin.H{"webhook": fresh.Webhook, "events": fresh.Events, "byEvents": fresh.WebhookByEvents,
		"base64": fresh.Base64(s.Engine.Cfg.WebhookFiles)})
}

// ---------------------------------------------------------------- conexão

type connectRequest struct {
	WebhookURL *string  `json:"webhookUrl"`
	Subscribe  []string `json:"subscribe"`
	Immediate  bool     `json:"immediate"`
	Phone      string   `json:"phone"`
	Websocket  *bool    `json:"websocket"`
}

func (s *Server) connect(c *gin.Context) {
	var req connectRequest
	if c.Request.ContentLength != 0 && !bind(c, &req) {
		return
	}
	inst := current(c)
	opt := engine.ConnectOptions{Webhook: req.WebhookURL, Subscribe: req.Subscribe, Websocket: req.Websocket}
	sess, err := s.Engine.Connect(inst, opt)
	if err != nil {
		failEngine(c, err)
		return
	}
	fresh, _ := s.Repo.ByID(inst.Id)
	if fresh == nil {
		fresh = inst
	}
	out := gin.H{"jid": fresh.Jid, "webhookUrl": fresh.Webhook, "eventString": fresh.Events}
	if strings.TrimSpace(req.Phone) != "" && fresh.Jid == "" {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 40*time.Second)
		defer cancel()
		code, err := sess.Pair(ctx, req.Phone)
		if err != nil {
			failEngine(c, err)
			return
		}
		out["pairingCode"] = code
	}
	ok(c, out)
}

func (s *Server) status(c *gin.Context) { ok(c, s.session(c).Status()) }

func (s *Server) qr(c *gin.Context) {
	sess := s.session(c)
	st := sess.Status()
	if logged, _ := st["LoggedIn"].(bool); logged {
		fail(c, http.StatusConflict, engine.ErrLogged)
		return
	}
	img, code := sess.QR()
	if img == "" {
		fail(c, http.StatusNotFound, errors.New("QR Code ainda não gerado: chame /instance/connect e tente de novo em alguns segundos"))
		return
	}
	ok(c, gin.H{"qrcode": img, "code": code, "Qrcode": img, "Code": code})
}

func (s *Server) pair(c *gin.Context) {
	var req struct {
		Phone     string   `json:"phone"`
		Subscribe []string `json:"subscribe"`
	}
	if !bind(c, &req) {
		return
	}
	inst := current(c)
	if len(req.Subscribe) > 0 {
		_, _ = s.Engine.Connect(inst, engine.ConnectOptions{Subscribe: req.Subscribe})
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 40*time.Second)
	defer cancel()
	code, err := s.Engine.Session(inst).Pair(ctx, req.Phone)
	if err != nil {
		failEngine(c, err)
		return
	}
	ok(c, gin.H{"PairingCode": code, "pairingCode": code})
}

func (s *Server) disconnect(c *gin.Context) {
	s.session(c).Disconnect()
	ok(c, gin.H{"disconnected": true})
}

func (s *Server) reconnect(c *gin.Context) {
	sess := s.session(c)
	if err := sess.Reconnect(); err != nil {
		failEngine(c, err)
		return
	}
	ok(c, sess.Status())
}

func (s *Server) logout(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	if err := s.session(c).Logout(ctx); err != nil {
		failEngine(c, err)
		return
	}
	ok(c, gin.H{"loggedOut": true})
}
