package evolutionapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.mau.fi/whatsmeow/types"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	instance_service "github.com/evolution-foundation/evolution-go/pkg/instance/service"
	"github.com/evolution-foundation/evolution-go/pkg/internal/event_types"
)

const integrationName = "WHATSAPP-BAILEYS" // value Evolution API clients expect

// Evolution API event names -> Evolution Go event types.
var eventMap = map[string]string{
	"APPLICATION_STARTUP":       event_types.CONNECTION,
	"QRCODE_UPDATED":            event_types.QRCODE,
	"CONNECTION_UPDATE":         event_types.CONNECTION,
	"MESSAGES_SET":              event_types.HISTORY_SYNC,
	"MESSAGES_UPSERT":           event_types.MESSAGE,
	"MESSAGES_EDITED":           event_types.MESSAGE,
	"MESSAGES_UPDATE":           event_types.READ_RECEIPT,
	"MESSAGES_DELETE":           event_types.MESSAGE,
	"SEND_MESSAGE":              event_types.SEND_MESSAGE,
	"SEND_MESSAGE_UPDATE":       event_types.READ_RECEIPT,
	"CONTACTS_SET":              event_types.CONTACT,
	"CONTACTS_UPSERT":           event_types.CONTACT,
	"CONTACTS_UPDATE":           event_types.CONTACT,
	"PRESENCE_UPDATE":           event_types.PRESENCE,
	"CHATS_SET":                 event_types.HISTORY_SYNC,
	"CHATS_UPSERT":              event_types.MESSAGE,
	"CHATS_UPDATE":              event_types.MESSAGE,
	"CHATS_DELETE":              event_types.MESSAGE,
	"GROUPS_UPSERT":             event_types.GROUP,
	"GROUP_UPDATE":              event_types.GROUP,
	"GROUP_PARTICIPANTS_UPDATE": event_types.GROUP,
	"LABELS_EDIT":               event_types.LABEL,
	"LABELS_ASSOCIATION":        event_types.LABEL,
	"CALL":                      event_types.CALL,
	"NEWSLETTER":                event_types.NEWSLETTER,
}

// Evolution Go event types -> the Evolution API names reported back.
var reverseEventMap = map[string][]string{
	event_types.MESSAGE:       {"MESSAGES_UPSERT"},
	event_types.SEND_MESSAGE:  {"SEND_MESSAGE"},
	event_types.READ_RECEIPT:  {"MESSAGES_UPDATE"},
	event_types.PRESENCE:      {"PRESENCE_UPDATE"},
	event_types.CHAT_PRESENCE: {"PRESENCE_UPDATE"},
	event_types.HISTORY_SYNC:  {"MESSAGES_SET", "CHATS_SET"},
	event_types.CALL:          {"CALL"},
	event_types.CONNECTION:    {"CONNECTION_UPDATE"},
	event_types.LABEL:         {"LABELS_EDIT", "LABELS_ASSOCIATION"},
	event_types.CONTACT:       {"CONTACTS_UPSERT"},
	event_types.GROUP:         {"GROUPS_UPSERT", "GROUP_UPDATE", "GROUP_PARTICIPANTS_UPDATE"},
	event_types.NEWSLETTER:    {"NEWSLETTER"},
	event_types.QRCODE:        {"QRCODE_UPDATED"},
}

// toEngineEvents converts Evolution API event names (or engine names) to the
// comma-separated engine list stored on the instance.
func toEngineEvents(events []string) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(e string) {
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	for _, e := range events {
		e = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(e), ".", "_"))
		if e == "ALL" {
			for _, t := range event_types.AllEventTypes {
				add(t)
			}
			continue
		}
		if t, ok := eventMap[e]; ok {
			add(t)
			if e == "PRESENCE_UPDATE" {
				add(event_types.CHAT_PRESENCE)
			}
		} else if event_types.IsEventType(e) {
			add(e)
		}
	}
	return out
}

func fromEngineEvents(stored string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, e := range strings.Split(stored, ",") {
		for _, name := range reverseEventMap[strings.TrimSpace(e)] {
			if !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	return out
}

func (h *Handler) connectionState(inst *instance_model.Instance) string {
	cli := h.ClientPointer[inst.Id]
	switch {
	case cli == nil:
		return "close"
	case cli.IsConnected() && cli.IsLoggedIn():
		return "open"
	case cli.IsConnected():
		return "connecting"
	default:
		return "close"
	}
}

func settingsOut(inst *instance_model.Instance) gin.H {
	return gin.H{
		"rejectCall":      inst.RejectCall,
		"msgCall":         inst.MsgRejectCall,
		"groupsIgnore":    inst.IgnoreGroups,
		"alwaysOnline":    inst.AlwaysOnline,
		"readMessages":    inst.ReadMessages,
		"readStatus":      false,
		"syncFullHistory": false,
		"ignoreStatus":    inst.IgnoreStatus,
	}
}

func (h *Handler) instanceOut(inst *instance_model.Instance) gin.H {
	out := gin.H{
		"id":                      inst.Id,
		"name":                    inst.Name,
		"connectionStatus":        h.connectionState(inst),
		"ownerJid":                inst.Jid,
		"integration":             integrationName,
		"token":                   inst.Token,
		"clientName":              inst.ClientName,
		"disconnectionReasonCode": inst.DisconnectReason,
		"createdAt":               inst.CreatedAt,
		"Setting":                 settingsOut(inst),
	}
	if inst.Jid != "" {
		if j, err := types.ParseJID(inst.Jid); err == nil {
			out["number"] = j.User
		}
	}
	if cli := h.ClientPointer[inst.Id]; cli != nil && cli.IsLoggedIn() && cli.Store.ID != nil {
		out["profileName"] = cli.Store.PushName
		out["ownerJid"] = cli.Store.ID.ToNonAD().String()
	}
	return out
}

// ---------------------------------------------------------------------------
// POST /instance/create
// ---------------------------------------------------------------------------

type createRequest struct {
	InstanceName    string `json:"instanceName"`
	Token           string `json:"token"`
	Number          string `json:"number"`
	Qrcode          bool   `json:"qrcode"`
	Integration     string `json:"integration"`
	RejectCall      bool   `json:"rejectCall"`
	MsgCall         string `json:"msgCall"`
	GroupsIgnore    bool   `json:"groupsIgnore"`
	AlwaysOnline    bool   `json:"alwaysOnline"`
	ReadMessages    bool   `json:"readMessages"`
	ReadStatus      bool   `json:"readStatus"`
	SyncFullHistory bool   `json:"syncFullHistory"`
	ProxyHost       string `json:"proxyHost"`
	ProxyPort       string `json:"proxyPort"`
	ProxyProtocol   string `json:"proxyProtocol"`
	ProxyUsername   string `json:"proxyUsername"`
	ProxyPassword   string `json:"proxyPassword"`
	Webhook         *struct {
		URL      string   `json:"url"`
		ByEvents bool     `json:"byEvents"`
		Base64   bool     `json:"base64"`
		Events   []string `json:"events"`
	} `json:"webhook"`
}

// CreateOrNative serves POST /instance/create for both APIs: bodies with
// "instanceName" use the Evolution API contract, anything else goes to the
// native Evolution Go handler. Both require the global key (checked upstream).
func (h *Handler) CreateOrNative(native gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			badRequest(c, "failed to read body")
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		var probe map[string]json.RawMessage
		if json.Unmarshal(body, &probe) == nil {
			if _, ok := probe["instanceName"]; ok {
				h.createInstance(c)
				return
			}
		}
		native(c)
	}
}

func (h *Handler) createInstance(c *gin.Context) {
	var req createRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.InstanceName == "" {
		badRequest(c, "instanceName is required")
		return
	}
	if req.Token == "" {
		req.Token = strings.ToUpper(strings.ReplaceAll(uuid.New().String(), "-", ""))
	}
	data := &instance_service.CreateStruct{
		Name:  req.InstanceName,
		Token: req.Token,
		AdvancedSettings: &instance_model.AdvancedSettings{
			AlwaysOnline:  req.AlwaysOnline,
			RejectCall:    req.RejectCall,
			MsgRejectCall: req.MsgCall,
			ReadMessages:  req.ReadMessages,
			IgnoreGroups:  req.GroupsIgnore,
		},
	}
	if req.ProxyHost != "" {
		data.Proxy = &instance_service.ProxyConfig{
			Protocol: req.ProxyProtocol,
			Host:     req.ProxyHost,
			Port:     req.ProxyPort,
			Username: req.ProxyUsername,
			Password: req.ProxyPassword,
		}
	} else if h.Config.ProxyHost != "" {
		data.Proxy = &instance_service.ProxyConfig{
			Protocol: h.Config.ProxyProtocol,
			Host:     h.Config.ProxyHost,
			Port:     h.Config.ProxyPort,
			Username: h.Config.ProxyUsername,
			Password: h.Config.ProxyPassword,
		}
	}
	inst, err := h.InstanceService.Create(data)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			errorResponse(c, http.StatusForbidden, "This name \""+req.InstanceName+"\" is already in use.")
			return
		}
		internalError(c, err)
		return
	}

	resp := gin.H{
		"instance": gin.H{
			"instanceName": inst.Name,
			"instanceId":   inst.Id,
			"integration":  integrationName,
			"status":       "created",
		},
		"hash":     inst.Token,
		"settings": settingsOut(inst),
	}

	webhookURL := ""
	events := []string{event_types.MESSAGE}
	if req.Webhook != nil && req.Webhook.URL != "" {
		webhookURL = req.Webhook.URL
		if len(req.Webhook.Events) > 0 {
			events = toEngineEvents(req.Webhook.Events)
		}
		resp["webhook"] = gin.H{"webhookUrl": webhookURL, "events": fromEngineEvents(strings.Join(events, ","))}
	}

	if req.Qrcode || req.Number != "" || webhookURL != "" {
		if _, _, _, err := h.InstanceService.Connect(&instance_service.ConnectStruct{
			WebhookUrl: webhookURL,
			Subscribe:  events,
		}, inst); err != nil {
			internalError(c, err)
			return
		}
	}
	if req.Number != "" {
		if pair, err := h.InstanceService.Pair(&instance_service.PairStruct{Phone: onlyDigits(req.Number)}, inst); err == nil {
			resp["qrcode"] = gin.H{"pairingCode": pair.PairingCode}
		}
	} else if req.Qrcode {
		if qr, err := h.InstanceService.GetQr(inst); err == nil {
			resp["qrcode"] = gin.H{"code": qr.Code, "base64": qr.Qrcode}
		}
	}
	c.JSON(http.StatusCreated, resp)
}

// GET /instance/fetchInstances?instanceName=&instanceId=
func (h *Handler) FetchInstances(c *gin.Context) {
	key := apiKey(c)
	if key == "" {
		unauthorized(c)
		return
	}
	var list []*instance_model.Instance
	if key == h.Config.GlobalApiKey {
		all, err := h.InstanceService.GetAll()
		if err != nil {
			internalError(c, err)
			return
		}
		list = all
	} else {
		inst, err := h.InstanceRepository.GetInstanceByToken(key)
		if err != nil || inst == nil {
			unauthorized(c)
			return
		}
		list = []*instance_model.Instance{inst}
	}
	name, id := c.Query("instanceName"), c.Query("instanceId")
	out := make([]gin.H, 0, len(list))
	for _, inst := range list {
		if (name != "" && inst.Name != name) || (id != "" && inst.Id != id) {
			continue
		}
		out = append(out, h.instanceOut(inst))
	}
	c.JSON(http.StatusOK, out)
}

// startIfNeeded starts the session keeping the stored webhook and events.
func (h *Handler) startIfNeeded(inst *instance_model.Instance) error {
	if h.ClientPointer[inst.Id] != nil {
		return nil
	}
	events := strings.Split(inst.Events, ",")
	if inst.Events == "" {
		events = nil
	}
	_, _, _, err := h.InstanceService.Connect(&instance_service.ConnectStruct{
		WebhookUrl:      inst.Webhook,
		Subscribe:       events,
		RabbitmqEnable:  inst.RabbitmqEnable,
		WebSocketEnable: inst.WebSocketEnable,
		NatsEnable:      inst.NatsEnable,
	}, inst)
	return err
}

// GET /instance/connect/:instance?number=
func (h *Handler) Connect(c *gin.Context) {
	inst := getInstance(c)
	if h.connectionState(inst) == "open" {
		c.JSON(http.StatusOK, gin.H{"instance": gin.H{"instanceName": inst.Name, "state": "open"}})
		return
	}
	if err := h.startIfNeeded(inst); err != nil {
		internalError(c, err)
		return
	}
	if number := onlyDigits(c.Query("number")); number != "" {
		pair, err := h.InstanceService.Pair(&instance_service.PairStruct{Phone: number}, inst)
		if err != nil {
			internalError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"pairingCode": pair.PairingCode, "code": nil, "count": 1})
		return
	}
	time.Sleep(500 * time.Millisecond)
	qr, err := h.InstanceService.GetQr(inst)
	if err != nil {
		if strings.Contains(err.Error(), "already logged in") {
			c.JSON(http.StatusOK, gin.H{"instance": gin.H{"instanceName": inst.Name, "state": "open"}})
			return
		}
		internalError(c, err)
		return
	}
	resp := gin.H{"pairingCode": nil, "code": qr.Code, "base64": qr.Qrcode, "count": 1}
	if qr.PasskeyStage != "" {
		resp["passkeyStage"] = qr.PasskeyStage
		resp["passkeyOpenUrl"] = qr.PasskeyOpenURL
		resp["passkeyCode"] = qr.PasskeyCode
	}
	c.JSON(http.StatusOK, resp)
}

// GET /instance/connectionState/:instance
func (h *Handler) ConnectionState(c *gin.Context) {
	inst := getInstance(c)
	c.JSON(http.StatusOK, gin.H{"instance": gin.H{"instanceName": inst.Name, "state": h.connectionState(inst)}})
}

// POST /instance/restart/:instance
func (h *Handler) Restart(c *gin.Context) {
	inst := getInstance(c)
	var err error
	if h.ClientPointer[inst.Id] == nil {
		err = h.startIfNeeded(inst)
	} else {
		err = h.InstanceService.Reconnect(inst)
	}
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"instance": gin.H{"instanceName": inst.Name, "state": h.connectionState(inst)}})
}

// POST /instance/setPresence/:instance
func (h *Handler) SetPresence(c *gin.Context) {
	var req struct {
		Presence string `json:"presence"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.Presence != "available" && req.Presence != "unavailable" {
		badRequest(c, "presence must be available or unavailable")
		return
	}
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	if err := cli.SendPresence(ctx(), types.Presence(req.Presence)); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"presence": req.Presence})
}

// DELETE /instance/logout/:instance
func (h *Handler) Logout(c *gin.Context) {
	inst := getInstance(c)
	if _, err := h.InstanceService.Logout(inst); err != nil {
		badRequest(c, "The \""+inst.Name+"\" instance is not connected")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "SUCCESS", "error": false, "response": gin.H{"message": "Instance logged out"}})
}

// DeleteOrNative serves DELETE /instance/delete/:instanceId for both APIs:
// an instance id goes to the native handler, an instance name is deleted with
// the Evolution API response.
func (h *Handler) DeleteOrNative(native gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		ref := c.Param("instanceId")
		if inst, err := h.InstanceRepository.GetInstanceByID(ref); err == nil && inst != nil {
			native(c)
			return
		}
		inst, err := h.InstanceRepository.GetInstanceByName(ref)
		if err != nil || inst == nil {
			notFound(c, "The \""+ref+"\" instance does not exist")
			return
		}
		if err := h.InstanceService.Delete(inst.Id); err != nil {
			internalError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "SUCCESS", "error": false, "response": gin.H{"message": "Instance deleted"}})
	}
}

// ---------------------------------------------------------------------------
// Settings: POST /settings/set/:instance, GET /settings/find/:instance
// ---------------------------------------------------------------------------

func (h *Handler) SettingsSet(c *gin.Context) {
	var req struct {
		RejectCall      bool   `json:"rejectCall"`
		MsgCall         string `json:"msgCall"`
		GroupsIgnore    bool   `json:"groupsIgnore"`
		AlwaysOnline    bool   `json:"alwaysOnline"`
		ReadMessages    bool   `json:"readMessages"`
		ReadStatus      bool   `json:"readStatus"`
		SyncFullHistory bool   `json:"syncFullHistory"`
		IgnoreStatus    *bool  `json:"ignoreStatus"`
	}
	if !bindJSON(c, &req) {
		return
	}
	inst := getInstance(c)
	s := &instance_model.AdvancedSettings{
		AlwaysOnline:  req.AlwaysOnline,
		RejectCall:    req.RejectCall,
		MsgRejectCall: req.MsgCall,
		ReadMessages:  req.ReadMessages,
		IgnoreGroups:  req.GroupsIgnore,
		IgnoreStatus:  inst.IgnoreStatus,
	}
	if req.IgnoreStatus != nil {
		s.IgnoreStatus = *req.IgnoreStatus
	}
	if err := h.InstanceService.UpdateAdvancedSettings(inst.Id, s); err != nil {
		internalError(c, err)
		return
	}
	inst.AlwaysOnline, inst.RejectCall, inst.MsgRejectCall = s.AlwaysOnline, s.RejectCall, s.MsgRejectCall
	inst.ReadMessages, inst.IgnoreGroups, inst.IgnoreStatus = s.ReadMessages, s.IgnoreGroups, s.IgnoreStatus
	c.JSON(http.StatusCreated, gin.H{"settings": gin.H{"instanceName": inst.Name, "settings": settingsOut(inst)}})
}

func (h *Handler) SettingsFind(c *gin.Context) {
	c.JSON(http.StatusOK, settingsOut(getInstance(c)))
}

// ---------------------------------------------------------------------------
// Webhook: POST /webhook/set/:instance, GET /webhook/find/:instance
// ---------------------------------------------------------------------------

func (h *Handler) WebhookSet(c *gin.Context) {
	type webhookBody struct {
		Enabled  *bool             `json:"enabled"`
		URL      string            `json:"url"`
		Headers  map[string]string `json:"headers"`
		ByEvents bool              `json:"byEvents"`
		Base64   bool              `json:"base64"`
		Events   []string          `json:"events"`
	}
	var req struct {
		Webhook *webhookBody `json:"webhook"`
		webhookBody
	}
	if !bindJSON(c, &req) {
		return
	}
	body := req.webhookBody
	if req.Webhook != nil {
		body = *req.Webhook
	}
	enabled := body.Enabled == nil || *body.Enabled
	inst := getInstance(c)
	if enabled {
		if !isURL(body.URL) {
			badRequest(c, "webhook.url must be an http(s) URL")
			return
		}
		inst.Webhook = body.URL
		events := toEngineEvents(body.Events)
		if len(events) == 0 {
			events = []string{event_types.MESSAGE}
		}
		inst.Events = strings.Join(events, ",")
	} else {
		inst.Webhook = ""
	}
	if err := h.InstanceRepository.Update(inst); err != nil {
		internalError(c, err)
		return
	}
	_ = h.WhatsmeowService.UpdateInstanceSettings(inst.Id) // no-op when not running
	c.JSON(http.StatusCreated, h.webhookOut(inst))
}

func (h *Handler) webhookOut(inst *instance_model.Instance) gin.H {
	return gin.H{
		"enabled":         inst.Webhook != "",
		"url":             inst.Webhook,
		"events":          fromEngineEvents(inst.Events),
		"webhookByEvents": false,
		"webhookBase64":   false,
	}
}

func (h *Handler) WebhookFind(c *gin.Context) {
	c.JSON(http.StatusOK, h.webhookOut(getInstance(c)))
}

// ---------------------------------------------------------------------------
// Proxy: POST /proxy/set/:instance, GET /proxy/find/:instance
// ---------------------------------------------------------------------------

func (h *Handler) ProxySet(c *gin.Context) {
	var req struct {
		Enabled  bool   `json:"enabled"`
		Host     string `json:"host"`
		Port     string `json:"port"`
		Protocol string `json:"protocol"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !bindJSON(c, &req) {
		return
	}
	inst := getInstance(c)
	var err error
	if !req.Enabled {
		err = h.InstanceService.RemoveProxy(inst.Id)
	} else {
		if req.Host == "" || req.Port == "" {
			badRequest(c, "host and port are required")
			return
		}
		err = h.InstanceService.SetProxyFromStruct(inst.Id, &instance_service.SetProxyStruct{
			Protocol: req.Protocol, Host: req.Host, Port: req.Port, Username: req.Username, Password: req.Password,
		})
	}
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"enabled": req.Enabled, "host": req.Host, "port": req.Port, "protocol": req.Protocol, "username": req.Username})
}

func (h *Handler) ProxyFind(c *gin.Context) {
	inst, err := h.InstanceRepository.GetInstanceByID(getInstance(c).Id)
	if err != nil {
		internalError(c, err)
		return
	}
	var p instance_service.ProxyConfig
	_ = json.Unmarshal([]byte(inst.Proxy), &p)
	c.JSON(http.StatusOK, gin.H{
		"enabled":  p.Host != "",
		"host":     p.Host,
		"port":     p.Port,
		"protocol": p.Protocol,
		"username": p.Username,
	})
}
