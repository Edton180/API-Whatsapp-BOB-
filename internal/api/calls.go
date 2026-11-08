// API WhatsApp BOB - Desenvolvido por Edton Braz.

package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pion/webrtc/v4"

	"github.com/edton180/api-whatsapp-bob/internal/calls"
	"github.com/edton180/api-whatsapp-bob/internal/store"
)

// Chamadas de voz e vídeo. O navegador do atendente manda a oferta SDP (WebRTC) e recebe a
// resposta na mesma requisição; o áudio e o vídeo passam pela BOB sem serem gravados.
func (s *Server) registerCalls(r *gin.Engine, inst gin.HandlerFunc) {
	g := r.Group("/call", inst)
	g.GET("/settings", s.callSettings)
	g.POST("/settings", s.callSettingsSave)
	g.GET("/ice-servers", s.callICEServers)
	g.GET("/list", s.callList)
	g.GET("/live", s.callLive)
	g.GET("/info/:callId", s.callInfo)
	g.POST("/start", s.callStart)
	g.POST("/answer", s.callAnswer)
	g.POST("/transfer", s.callTransfer)
	g.POST("/hangup", s.callHangup)
	g.POST("/mute", s.callMute)
	g.POST("/camera", s.callCamera)
	g.POST("/ice", s.callICE)
}

// failCall traduz os erros das chamadas para o código HTTP certo.
func failCall(c *gin.Context, err error) {
	switch {
	case errors.Is(err, calls.ErrDisabled):
		fail(c, http.StatusPreconditionFailed, err)
	case errors.Is(err, calls.ErrNotLive), errors.Is(err, store.ErrCallNotFound):
		fail(c, http.StatusNotFound, err)
	case errors.Is(err, calls.ErrTaken), errors.Is(err, calls.ErrNotYours), errors.Is(err, calls.ErrNoMedia):
		fail(c, http.StatusConflict, err)
	default:
		failEngine(c, err)
	}
}

func (s *Server) callsReady(c *gin.Context) bool {
	if s.Calls == nil {
		fail(c, http.StatusServiceUnavailable, errors.New("chamadas indisponíveis neste servidor (veja WEBRTC_* no .env)"))
		return false
	}
	return true
}

// callSettings: GET /call/settings
func (s *Server) callSettings(c *gin.Context) {
	inst := current(c)
	ok(c, gin.H{
		"enabled":    inst.CallsEnabled,
		"active":     s.Calls != nil && s.Calls.Handles(inst.Id),
		"rejectCall": inst.RejectCall,
		"msgCall":    inst.MsgRejectCall,
		"available":  s.Calls != nil,
	})
}

// callSettingsSave: POST /call/settings {"enabled": true}. Liga ou desliga as chamadas e
// reconecta a instância com a mesma sessão (não precisa ler o QR Code de novo).
func (s *Server) callSettingsSave(c *gin.Context) {
	if !s.callsReady(c) {
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if !bind(c, &req) {
		return
	}
	if req.Enabled == nil {
		fail(c, http.StatusBadRequest, errors.New(`informe "enabled": true ou false`))
		return
	}
	inst := current(c)
	if err := s.Repo.Update(inst.Id, map[string]any{"calls_enabled": *req.Enabled}); err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	inst.CallsEnabled = *req.Enabled
	reconnected := false
	if sess := s.Engine.Lookup(inst.Id); sess != nil && inst.Jid != "" {
		sess.Logf("INFO", "chamadas %s: reconectando", map[bool]string{true: "ligadas", false: "desligadas"}[*req.Enabled])
		if err := sess.Rebuild(); err != nil {
			failEngine(c, err)
			return
		}
		reconnected = true
	}
	ok(c, gin.H{"enabled": inst.CallsEnabled, "reconnected": reconnected})
}

// callICEServers: GET /call/ice-servers?agent=... devolve STUN e TURN com credencial
// temporária para o RTCPeerConnection do navegador.
func (s *Server) callICEServers(c *gin.Context) {
	cfg := s.Engine.Cfg.WebRTC
	ttl := cfg.TURNTTL
	if ttl <= 0 {
		ttl = 600
	}
	ok(c, gin.H{"iceServers": calls.ICEServers(cfg, c.Query("agent"), time.Now()), "ttl": ttl})
}

// callList: GET /call/list?status=missed&number=5511...&video=true&since=<unix>&limit=50&offset=0
func (s *Server) callList(c *gin.Context) {
	q := store.CallQuery{Status: c.Query("status"), Limit: queryInt(c, "limit", 50), Offset: queryInt(c, "offset", 0)}
	if n := strings.TrimSpace(c.Query("number")); n != "" {
		if !strings.Contains(n, "@") {
			n = digitsOnly(n) + "@s.whatsapp.net"
		}
		q.Remote = n
	}
	if v := c.Query("video"); v != "" {
		b := v == "true" || v == "1"
		q.Video = &b
	}
	if v, err := strconv.ParseInt(c.Query("since"), 10, 64); err == nil && v > 0 {
		q.Since = time.Unix(v, 0)
	}
	list, total, err := s.Repo.Calls(current(c).Id, q)
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for i := range list {
		out = append(out, calls.CallJSON(&list[i]))
	}
	ok(c, gin.H{"total": total, "limit": q.Limit, "offset": q.Offset, "calls": out})
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// callLive: GET /call/live lista as chamadas em andamento (tocando ou atendidas).
func (s *Server) callLive(c *gin.Context) {
	if s.Calls == nil {
		ok(c, gin.H{"calls": []any{}})
		return
	}
	ok(c, gin.H{"calls": s.Calls.Live(current(c).Id)})
}

// callInfo: GET /call/info/:callId
func (s *Server) callInfo(c *gin.Context) {
	inst := current(c)
	rec, err := s.Repo.CallByID(inst.Id, c.Param("callId"))
	if err != nil {
		failCall(c, err)
		return
	}
	out := calls.CallJSON(rec)
	if s.Calls != nil {
		if st := s.Calls.State(inst.Id, rec.CallID); st != nil {
			out["live"] = true
			for k, v := range st {
				out[k] = v
			}
		}
	}
	ok(c, out)
}

type callMediaRequest struct {
	CallID string `json:"callId"`
	Number string `json:"number"`
	Video  bool   `json:"video"`
	SDP    string `json:"sdp"`
	Agent  string `json:"agent"`
}

func (r *callMediaRequest) media() calls.Media { return calls.Media{SDP: r.SDP, Agent: r.Agent} }

func bindMedia(c *gin.Context, needCall bool) (*callMediaRequest, bool) {
	var req callMediaRequest
	if !bind(c, &req) {
		return nil, false
	}
	if strings.TrimSpace(req.SDP) == "" {
		fail(c, http.StatusBadRequest, errors.New(`informe "sdp" com a oferta WebRTC do navegador`))
		return nil, false
	}
	if needCall && req.CallID == "" {
		fail(c, http.StatusBadRequest, errors.New(`informe "callId"`))
		return nil, false
	}
	return &req, true
}

// callStart: POST /call/start {"number": "5511...", "video": false, "sdp": "...", "agent": "Ana"}
func (s *Server) callStart(c *gin.Context) {
	if !s.callsReady(c) {
		return
	}
	req, okReq := bindMedia(c, false)
	if !okReq {
		return
	}
	if req.Number == "" {
		fail(c, http.StatusBadRequest, errors.New(`informe "number"`))
		return
	}
	res, err := s.Calls.Start(c.Request.Context(), s.session(c), req.Number, req.Video, req.media())
	if err != nil {
		failCall(c, err)
		return
	}
	ok(c, res)
}

// callAnswer: POST /call/answer {"callId": "...", "sdp": "...", "agent": "Ana"}. Só o primeiro
// atendente leva a chamada; os outros recebem 409.
func (s *Server) callAnswer(c *gin.Context) {
	if !s.callsReady(c) {
		return
	}
	req, okReq := bindMedia(c, true)
	if !okReq {
		return
	}
	res, err := s.Calls.Answer(current(c).Id, req.CallID, req.media())
	if err != nil {
		failCall(c, err)
		return
	}
	ok(c, res)
}

// callTransfer: POST /call/transfer {"callId": "...", "sdp": "...", "agent": "Bruno"} passa a
// chamada para o navegador de outro atendente sem derrubar o contato.
func (s *Server) callTransfer(c *gin.Context) {
	if !s.callsReady(c) {
		return
	}
	req, okReq := bindMedia(c, true)
	if !okReq {
		return
	}
	res, err := s.Calls.Transfer(current(c).Id, req.CallID, req.media())
	if err != nil {
		failCall(c, err)
		return
	}
	ok(c, res)
}

type callIDRequest struct {
	CallID string `json:"callId"`
	Muted  bool   `json:"muted"`
	On     bool   `json:"on"`
}

func bindCallID(c *gin.Context) (*callIDRequest, bool) {
	var req callIDRequest
	if !bind(c, &req) {
		return nil, false
	}
	if req.CallID == "" {
		fail(c, http.StatusBadRequest, errors.New(`informe "callId"`))
		return nil, false
	}
	return &req, true
}

// callHangup: POST /call/hangup {"callId": "..."}
func (s *Server) callHangup(c *gin.Context) {
	if !s.callsReady(c) {
		return
	}
	if req, okReq := bindCallID(c); okReq {
		callReply(c, s.Calls.Hangup(current(c).Id, req.CallID))
	}
}

// callMute: POST /call/mute {"callId": "...", "muted": true}
func (s *Server) callMute(c *gin.Context) {
	if !s.callsReady(c) {
		return
	}
	if req, okReq := bindCallID(c); okReq {
		callReply(c, s.Calls.Mute(current(c).Id, req.CallID, req.Muted))
	}
}

// callCamera: POST /call/camera {"callId": "...", "on": true}
func (s *Server) callCamera(c *gin.Context) {
	if !s.callsReady(c) {
		return
	}
	if req, okReq := bindCallID(c); okReq {
		callReply(c, s.Calls.Camera(current(c).Id, req.CallID, req.On))
	}
}

// callICE: POST /call/ice {"callId": "...", "candidate": "candidate:...", "sdpMid": "0", "sdpMLineIndex": 0}
func (s *Server) callICE(c *gin.Context) {
	if !s.callsReady(c) {
		return
	}
	var req struct {
		CallID        string  `json:"callId"`
		Candidate     string  `json:"candidate"`
		SDPMid        *string `json:"sdpMid"`
		SDPMLineIndex *uint16 `json:"sdpMLineIndex"`
	}
	if !bind(c, &req) {
		return
	}
	if req.CallID == "" || req.Candidate == "" {
		fail(c, http.StatusBadRequest, errors.New(`informe "callId" e "candidate"`))
		return
	}
	callReply(c, s.Calls.AddICE(current(c).Id, req.CallID, webrtc.ICECandidateInit{Candidate: req.Candidate, SDPMid: req.SDPMid, SDPMLineIndex: req.SDPMLineIndex}))
}

func callReply(c *gin.Context, err error) {
	if err != nil {
		failCall(c, err)
		return
	}
	ok(c, gin.H{"ok": true})
}
