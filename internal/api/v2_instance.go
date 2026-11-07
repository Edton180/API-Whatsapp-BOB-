// API WhatsApp BOB - Desenvolvido por Edton Braz.

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/edton180/api-whatsapp-bob/internal/engine"
	"github.com/edton180/api-whatsapp-bob/internal/store"
)

// Nomes de evento do padrão v2 -> categorias de eventos da BOB.
var v2Events = map[string][]string{
	"APPLICATION_STARTUP":       {"CONNECTION"},
	"QRCODE_UPDATED":            {"QRCODE"},
	"CONNECTION_UPDATE":         {"CONNECTION"},
	"MESSAGES_SET":              {"HISTORY_SYNC"},
	"MESSAGES_UPSERT":           {"MESSAGE"},
	"MESSAGES_EDITED":           {"MESSAGE"},
	"MESSAGES_UPDATE":           {"READ_RECEIPT"},
	"MESSAGES_DELETE":           {"MESSAGE", "CHAT"},
	"SEND_MESSAGE":              {"SEND_MESSAGE"},
	"SEND_MESSAGE_UPDATE":       {"READ_RECEIPT"},
	"CONTACTS_SET":              {"CONTACT"},
	"CONTACTS_UPSERT":           {"CONTACT"},
	"CONTACTS_UPDATE":           {"CONTACT"},
	"PRESENCE_UPDATE":           {"PRESENCE", "CHAT_PRESENCE"},
	"CHATS_SET":                 {"HISTORY_SYNC"},
	"CHATS_UPSERT":              {"CHAT"},
	"CHATS_UPDATE":              {"CHAT"},
	"CHATS_DELETE":              {"CHAT"},
	"GROUPS_UPSERT":             {"GROUP"},
	"GROUP_UPDATE":              {"GROUP"},
	"GROUP_PARTICIPANTS_UPDATE": {"GROUP"},
	"LABELS_EDIT":               {"LABEL"},
	"LABELS_ASSOCIATION":        {"LABEL"},
	"CALL":                      {"CALL"},
	"NEWSLETTER":                {"NEWSLETTER"},
}

// Categoria da BOB -> nomes do padrão v2 devolvidos em /webhook/find.
var v2EventNames = map[string][]string{
	"MESSAGE":       {"MESSAGES_UPSERT"},
	"SEND_MESSAGE":  {"SEND_MESSAGE"},
	"READ_RECEIPT":  {"MESSAGES_UPDATE"},
	"PRESENCE":      {"PRESENCE_UPDATE"},
	"CHAT_PRESENCE": {"PRESENCE_UPDATE"},
	"HISTORY_SYNC":  {"MESSAGES_SET", "CHATS_SET"},
	"CALL":          {"CALL"},
	"CONNECTION":    {"CONNECTION_UPDATE"},
	"QRCODE":        {"QRCODE_UPDATED"},
	"CONTACT":       {"CONTACTS_UPSERT"},
	"GROUP":         {"GROUPS_UPSERT", "GROUP_UPDATE", "GROUP_PARTICIPANTS_UPDATE"},
	"LABEL":         {"LABELS_EDIT", "LABELS_ASSOCIATION"},
	"NEWSLETTER":    {"NEWSLETTER"},
	"CHAT":          {"CHATS_UPDATE"},
}

// v2ToCategories converte nomes do padrão v2 (ou categorias da BOB) em categorias.
func v2ToCategories(list []string) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(v string) {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	for _, e := range list {
		e = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(e), ".", "_"))
		if e == "" {
			continue
		}
		if e == "ALL" {
			return []string{"ALL"}
		}
		if cats, ok := v2Events[e]; ok {
			for _, c := range cats {
				add(c)
			}
			continue
		}
		if _, ok := v2EventNames[e]; ok || e == "BLOCKLIST" {
			add(e)
		}
	}
	return out
}

func v2FromCategories(stored string) []string {
	if strings.TrimSpace(stored) == "" || strings.EqualFold(stored, "ALL") {
		return []string{"ALL"}
	}
	seen := map[string]bool{}
	out := []string{}
	for _, cat := range strings.Split(stored, ",") {
		for _, n := range v2EventNames[strings.ToUpper(strings.TrimSpace(cat))] {
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	return out
}

func v2Settings(inst *store.Instance) gin.H {
	return gin.H{
		"rejectCall": inst.RejectCall, "msgCall": inst.MsgRejectCall, "groupsIgnore": inst.IgnoreGroups,
		"alwaysOnline": inst.AlwaysOnline, "readMessages": inst.ReadMessages, "ignoreStatus": inst.IgnoreStatus,
		"readStatus": false, "syncFullHistory": false,
	}
}

func (s *Server) v2State(inst *store.Instance) string {
	st, _ := s.view(inst)["state"].(string)
	return st
}

// v2CreateOrNative atende POST /instance/create nos dois formatos: com "instanceName" usa o
// padrão v2; sem ele, a rota nativa da BOB.
func (s *Server) v2CreateOrNative(native gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		var probe map[string]json.RawMessage
		if json.Unmarshal(body, &probe) == nil {
			if _, ok := probe["instanceName"]; ok {
				s.v2Create(c)
				return
			}
		}
		native(c)
	}
}

func (s *Server) v2Create(c *gin.Context) {
	var req struct {
		InstanceName  string `json:"instanceName"`
		Token         string `json:"token"`
		Number        string `json:"number"`
		Qrcode        bool   `json:"qrcode"`
		RejectCall    bool   `json:"rejectCall"`
		MsgCall       string `json:"msgCall"`
		GroupsIgnore  bool   `json:"groupsIgnore"`
		AlwaysOnline  bool   `json:"alwaysOnline"`
		ReadMessages  bool   `json:"readMessages"`
		ProxyHost     string `json:"proxyHost"`
		ProxyPort     string `json:"proxyPort"`
		ProxyProtocol string `json:"proxyProtocol"`
		ProxyUsername string `json:"proxyUsername"`
		ProxyPassword string `json:"proxyPassword"`
		Webhook       *struct {
			URL    string   `json:"url"`
			Events []string `json:"events"`
		} `json:"webhook"`
	}
	if !v2Bind(c, &req) {
		return
	}
	if strings.TrimSpace(req.InstanceName) == "" {
		v2Fail(c, http.StatusBadRequest, "instanceName is required")
		return
	}
	if existing, err := s.Repo.ByIDOrName(req.InstanceName); err == nil && existing != nil {
		v2Fail(c, http.StatusForbidden, "This name \""+req.InstanceName+"\" is already in use.")
		return
	}
	inst := &store.Instance{
		Name: req.InstanceName, Token: strings.TrimSpace(req.Token),
		ClientName: s.Engine.Cfg.ClientName, OsName: s.Engine.Cfg.OSName,
	}
	inst.Apply(store.AdvancedSettings{
		AlwaysOnline: req.AlwaysOnline, RejectCall: req.RejectCall, MsgRejectCall: req.MsgCall,
		ReadMessages: req.ReadMessages, IgnoreGroups: req.GroupsIgnore,
	})
	if req.Webhook != nil && strings.TrimSpace(req.Webhook.URL) != "" {
		inst.Webhook = strings.TrimSpace(req.Webhook.URL)
		inst.Events = strings.Join(v2ToCategories(req.Webhook.Events), ",")
	}
	if req.ProxyHost != "" {
		b, _ := json.Marshal(map[string]string{
			"protocol": req.ProxyProtocol, "host": req.ProxyHost, "port": req.ProxyPort,
			"username": req.ProxyUsername, "password": req.ProxyPassword,
		})
		inst.Proxy = string(b)
	}
	if err := s.Repo.Create(inst); err != nil {
		v2Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	out := gin.H{
		"instance": gin.H{"instanceName": inst.Name, "instanceId": inst.Id, "status": "created"},
		"hash":     inst.Token,
		"settings": v2Settings(inst),
	}
	if inst.Webhook != "" {
		out["webhook"] = gin.H{"url": inst.Webhook, "events": v2FromCategories(inst.Events)}
	}
	if req.Qrcode || req.Number != "" {
		sess, err := s.Engine.Connect(inst, engine.ConnectOptions{})
		if err == nil {
			if req.Number != "" {
				ctx, cancel := context.WithTimeout(c.Request.Context(), 40*time.Second)
				code, perr := sess.Pair(ctx, req.Number)
				cancel()
				if perr == nil {
					out["qrcode"] = gin.H{"pairingCode": code}
				}
			} else {
				out["qrcode"] = s.v2WaitQR(c.Request.Context(), sess)
			}
		}
	}
	c.JSON(http.StatusCreated, out)
}

// v2WaitQR espera alguns segundos pelo primeiro QR Code da sessão.
func (s *Server) v2WaitQR(ctx context.Context, sess *engine.Session) gin.H {
	for i := 0; i < 20; i++ {
		if img, code := sess.QR(); img != "" {
			return gin.H{"pairingCode": nil, "code": code, "base64": img, "count": 1}
		}
		select {
		case <-ctx.Done():
			return gin.H{"code": nil, "base64": nil}
		case <-time.After(500 * time.Millisecond):
		}
	}
	return gin.H{"code": nil, "base64": nil}
}

func (s *Server) v2FetchInstances(c *gin.Context) {
	key := apiKey(c)
	var list []*store.Instance
	if s.isGlobal(key) {
		all, err := s.Repo.All()
		if err != nil {
			v2Fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		list = all
	} else if inst, err := s.Repo.ByToken(key); err == nil && inst != nil && key != "" {
		list = []*store.Instance{inst}
	} else {
		v2Fail(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	name, id := c.Query("instanceName"), c.Query("instanceId")
	out := make([]gin.H, 0, len(list))
	for _, inst := range list {
		if (name != "" && inst.Name != name) || (id != "" && inst.Id != id) {
			continue
		}
		v := s.view(inst)
		row := gin.H{
			"id": inst.Id, "name": inst.Name, "connectionStatus": v["state"], "ownerJid": inst.Jid,
			"profileName": v["profileName"], "number": v["phone"], "token": inst.Token,
			"clientName": inst.ClientName, "disconnectionReasonCode": inst.DisconnectReason,
			"createdAt": inst.CreatedAt, "Setting": v2Settings(inst),
		}
		out = append(out, row)
	}
	c.JSON(http.StatusOK, out)
}

func (s *Server) v2Connect(c *gin.Context) {
	inst := current(c)
	if s.v2State(inst) == "open" {
		c.JSON(http.StatusOK, gin.H{"instance": gin.H{"instanceName": inst.Name, "state": "open"}})
		return
	}
	sess, err := s.Engine.Connect(inst, engine.ConnectOptions{})
	if err != nil {
		v2Engine(c, err)
		return
	}
	if number := strings.TrimSpace(c.Query("number")); number != "" {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 40*time.Second)
		defer cancel()
		code, err := sess.Pair(ctx, number)
		if err != nil {
			v2Engine(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"pairingCode": code, "code": nil, "count": 1})
		return
	}
	c.JSON(http.StatusOK, s.v2WaitQR(c.Request.Context(), sess))
}

func (s *Server) v2ConnectionState(c *gin.Context) {
	inst := current(c)
	c.JSON(http.StatusOK, gin.H{"instance": gin.H{"instanceName": inst.Name, "state": s.v2State(inst)}})
}

func (s *Server) v2Restart(c *gin.Context) {
	inst := current(c)
	var err error
	if s.Engine.Lookup(inst.Id) == nil {
		_, err = s.Engine.Connect(inst, engine.ConnectOptions{})
	} else {
		err = s.session(c).Reconnect()
	}
	if err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"instance": gin.H{"instanceName": inst.Name, "state": s.v2State(inst)}})
}

func (s *Server) v2SetPresence(c *gin.Context) {
	var req struct {
		Presence string `json:"presence"`
	}
	if !v2Bind(c, &req) {
		return
	}
	if err := s.session(c).SetGlobalPresence(c.Request.Context(), req.Presence); err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"presence": req.Presence})
}

func (s *Server) v2Logout(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	if err := s.session(c).Logout(ctx); err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "SUCCESS", "error": false, "response": gin.H{"message": "Instance logged out"}})
}

// ---------------------------------------------------------------- configurações

func (s *Server) v2SettingsSet(c *gin.Context) {
	inst := current(c)
	var req struct {
		RejectCall   bool   `json:"rejectCall"`
		MsgCall      string `json:"msgCall"`
		GroupsIgnore bool   `json:"groupsIgnore"`
		AlwaysOnline bool   `json:"alwaysOnline"`
		ReadMessages bool   `json:"readMessages"`
		IgnoreStatus *bool  `json:"ignoreStatus"`
	}
	if !v2Bind(c, &req) {
		return
	}
	st := store.AdvancedSettings{
		AlwaysOnline: req.AlwaysOnline, RejectCall: req.RejectCall, MsgRejectCall: req.MsgCall,
		ReadMessages: req.ReadMessages, IgnoreGroups: req.GroupsIgnore, IgnoreStatus: inst.IgnoreStatus,
	}
	if req.IgnoreStatus != nil {
		st.IgnoreStatus = *req.IgnoreStatus
	}
	if err := s.saveSettings(inst, st); err != nil {
		v2Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusCreated, gin.H{"settings": gin.H{"instanceName": inst.Name, "settings": v2Settings(inst)}})
}

// saveSettings grava as opções avançadas e aplica na sessão em execução.
func (s *Server) saveSettings(inst *store.Instance, st store.AdvancedSettings) error {
	inst.Apply(st)
	if err := s.Repo.Update(inst.Id, map[string]any{
		"always_online": st.AlwaysOnline, "reject_call": st.RejectCall, "msg_reject_call": st.MsgRejectCall,
		"read_messages": st.ReadMessages, "ignore_groups": st.IgnoreGroups, "ignore_status": st.IgnoreStatus,
	}); err != nil {
		return err
	}
	if s.Engine.Lookup(inst.Id) != nil {
		if fresh, err := s.Repo.ByID(inst.Id); err == nil {
			s.Engine.Session(fresh)
		}
	}
	return nil
}

func (s *Server) v2SettingsFind(c *gin.Context) { c.JSON(http.StatusOK, v2Settings(current(c))) }

func (s *Server) v2WebhookSet(c *gin.Context) {
	type hook struct {
		Enabled *bool    `json:"enabled"`
		URL     string   `json:"url"`
		Events  []string `json:"events"`
	}
	var req struct {
		Webhook *hook `json:"webhook"`
		hook
	}
	if !v2Bind(c, &req) {
		return
	}
	h := req.hook
	if req.Webhook != nil {
		h = *req.Webhook
	}
	inst := current(c)
	url, events := "", inst.Events
	if h.Enabled == nil || *h.Enabled {
		url = strings.TrimSpace(h.URL)
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			v2Fail(c, http.StatusBadRequest, "webhook.url must be an http(s) URL")
			return
		}
		events = strings.Join(v2ToCategories(h.Events), ",")
	}
	if err := s.Repo.Update(inst.Id, map[string]any{"webhook": url, "events": events}); err != nil {
		v2Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	inst.Webhook, inst.Events = url, events
	if s.Engine.Lookup(inst.Id) != nil {
		if fresh, err := s.Repo.ByID(inst.Id); err == nil {
			s.Engine.Session(fresh)
		}
	}
	c.JSON(http.StatusCreated, v2Webhook(inst))
}

func v2Webhook(inst *store.Instance) gin.H {
	return gin.H{"enabled": inst.Webhook != "", "url": inst.Webhook, "events": v2FromCategories(inst.Events)}
}

func (s *Server) v2WebhookFind(c *gin.Context) { c.JSON(http.StatusOK, v2Webhook(current(c))) }

func (s *Server) v2ProxySet(c *gin.Context) {
	var req struct {
		Enabled  bool   `json:"enabled"`
		Host     string `json:"host"`
		Port     string `json:"port"`
		Protocol string `json:"protocol"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !v2Bind(c, &req) {
		return
	}
	inst := current(c)
	raw := ""
	if req.Enabled {
		if req.Host == "" || req.Port == "" {
			v2Fail(c, http.StatusBadRequest, "host and port are required")
			return
		}
		b, _ := json.Marshal(map[string]string{
			"protocol": req.Protocol, "host": req.Host, "port": req.Port, "username": req.Username, "password": req.Password,
		})
		raw = string(b)
	}
	if err := s.Repo.Update(inst.Id, map[string]any{"proxy": raw}); err != nil {
		v2Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if sess := s.Engine.Lookup(inst.Id); sess != nil {
		if err := sess.ApplyProxy(raw); err != nil {
			v2Engine(c, err)
			return
		}
	}
	c.JSON(http.StatusCreated, gin.H{"enabled": req.Enabled, "host": req.Host, "port": req.Port, "protocol": req.Protocol, "username": req.Username})
}

func (s *Server) v2ProxyFind(c *gin.Context) {
	inst, err := s.Repo.ByID(current(c).Id)
	if err != nil {
		v2Engine(c, err)
		return
	}
	var p map[string]string
	_ = json.Unmarshal([]byte(inst.Proxy), &p)
	c.JSON(http.StatusOK, gin.H{"enabled": p["host"] != "", "host": p["host"], "port": p["port"], "protocol": p["protocol"], "username": p["username"]})
}
