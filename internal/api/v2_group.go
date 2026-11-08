// API WhatsApp BOB - Desenvolvido por Edton Braz.

package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mau.fi/whatsmeow/types"

	"github.com/edton180/api-whatsapp-bob/internal/calls"
	"github.com/edton180/api-whatsapp-bob/internal/engine"
)

func v2Participants(ps []types.GroupParticipant) []gin.H {
	out := make([]gin.H, 0, len(ps))
	for _, p := range ps {
		var admin any
		switch {
		case p.IsSuperAdmin:
			admin = "superadmin"
		case p.IsAdmin:
			admin = "admin"
		}
		row := gin.H{"id": p.JID.String(), "admin": admin}
		if !p.PhoneNumber.IsEmpty() {
			row["phoneNumber"] = p.PhoneNumber.String()
		}
		if !p.LID.IsEmpty() {
			row["lid"] = p.LID.String()
		}
		out = append(out, row)
	}
	return out
}

func v2Group(g *types.GroupInfo, withParticipants bool) gin.H {
	out := gin.H{
		"id":           g.JID.String(),
		"subject":      g.Name,
		"subjectOwner": g.NameSetBy.String(),
		"subjectTime":  g.NameSetAt.Unix(),
		"size":         len(g.Participants),
		"creation":     g.GroupCreated.Unix(),
		"owner":        g.OwnerJID.String(),
		"desc":         g.Topic,
		"descId":       g.TopicID,
		"restrict":     g.IsLocked,
		"announce":     g.IsAnnounce,
		"isCommunity":  g.IsParent,
		"ephemeral":    g.DisappearingTimer,
		"joinApproval": g.IsJoinApprovalRequired,
	}
	if !g.LinkedParentJID.IsEmpty() {
		out["linkedParent"] = g.LinkedParentJID.String()
	}
	if withParticipants {
		out["participants"] = v2Participants(g.Participants)
	}
	return out
}

func (s *Server) v2GroupCreate(c *gin.Context) {
	var req struct {
		Subject      string   `json:"subject"`
		Description  string   `json:"description"`
		Participants []string `json:"participants"`
	}
	if !v2Bind(c, &req) {
		return
	}
	sess := s.session(c)
	res, err := sess.GroupCreate(c.Request.Context(), req.Subject, req.Participants)
	if err != nil {
		v2Engine(c, err)
		return
	}
	g := res.(*types.GroupInfo)
	if req.Description != "" {
		if err := sess.GroupDescription(c.Request.Context(), g.JID.String(), req.Description); err == nil {
			g.Topic = req.Description
		}
	}
	c.JSON(http.StatusCreated, v2Group(g, true))
}

func (s *Server) v2GroupSubject(c *gin.Context) {
	g, ok := groupParam(c)
	if !ok {
		return
	}
	var req struct {
		Subject string `json:"subject"`
	}
	if !v2Bind(c, &req) {
		return
	}
	if err := s.session(c).GroupName(c.Request.Context(), g, req.Subject); err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"update": "success"})
}

func (s *Server) v2GroupDescription(c *gin.Context) {
	g, ok := groupParam(c)
	if !ok {
		return
	}
	var req struct {
		Description string `json:"description"`
	}
	if !v2Bind(c, &req) {
		return
	}
	if err := s.session(c).GroupDescription(c.Request.Context(), g, req.Description); err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"update": "success"})
}

func (s *Server) v2GroupPicture(c *gin.Context) {
	g, ok := groupParam(c)
	if !ok {
		return
	}
	var req struct {
		Image string `json:"image"`
	}
	if !v2Bind(c, &req) {
		return
	}
	if !strings.Contains(g, "@") {
		g += "@g.us"
	}
	if _, err := s.session(c).ProfilePicture(c.Request.Context(), req.Image, g); err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"update": "success"})
}

func (s *Server) v2GroupInfo(c *gin.Context) {
	g, ok := groupParam(c)
	if !ok {
		return
	}
	sess := s.session(c)
	res, err := sess.GroupInfo(c.Request.Context(), g)
	if err != nil {
		v2Engine(c, err)
		return
	}
	info := res.(*types.GroupInfo)
	out := v2Group(info, true)
	if pic, err := sess.Avatar(c.Request.Context(), info.JID.String(), false); err == nil {
		out["pictureUrl"] = pic["url"]
	}
	c.JSON(http.StatusOK, out)
}

func (s *Server) v2GroupAll(c *gin.Context) {
	res, err := s.session(c).Groups(c.Request.Context())
	if err != nil {
		v2Engine(c, err)
		return
	}
	withParticipants := c.Query("getParticipants") == "true"
	list, _ := res.([]*types.GroupInfo)
	out := make([]gin.H, 0, len(list))
	for _, g := range list {
		out = append(out, v2Group(g, withParticipants))
	}
	c.JSON(http.StatusOK, out)
}

func (s *Server) v2GroupParticipants(c *gin.Context) {
	g, ok := groupParam(c)
	if !ok {
		return
	}
	res, err := s.session(c).GroupInfo(c.Request.Context(), g)
	if err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"participants": v2Participants(res.(*types.GroupInfo).Participants)})
}

func (s *Server) v2GroupInviteCode(c *gin.Context) {
	g, ok := groupParam(c)
	if !ok {
		return
	}
	link, err := s.session(c).GroupInvite(c.Request.Context(), g, false)
	if err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"inviteUrl": link, "inviteCode": engine.InviteCode(link)})
}

func (s *Server) v2GroupRevokeInvite(c *gin.Context) {
	g, ok := groupParam(c)
	if !ok {
		return
	}
	link, err := s.session(c).GroupInvite(c.Request.Context(), g, true)
	if err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"revoked": true, "inviteCode": engine.InviteCode(link)})
}

func (s *Server) v2GroupInviteInfo(c *gin.Context) {
	info, err := s.session(c).GroupInviteInfo(c.Request.Context(), c.Query("inviteCode"))
	if err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusOK, v2Group(info, true))
}

func (s *Server) v2GroupAcceptInvite(c *gin.Context) {
	jid, err := s.session(c).GroupJoin(c.Request.Context(), engine.InviteCode(c.Query("inviteCode")))
	if err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"accepted": true, "groupJid": jid})
}

func (s *Server) v2GroupSendInvite(c *gin.Context) {
	var req struct {
		GroupJid    string   `json:"groupJid"`
		Description string   `json:"description"`
		Numbers     []string `json:"numbers"`
	}
	if !v2Bind(c, &req) {
		return
	}
	if req.GroupJid == "" || len(req.Numbers) == 0 {
		v2Fail(c, http.StatusBadRequest, "groupJid and numbers are required")
		return
	}
	sess := s.session(c)
	ctx := c.Request.Context()
	link, err := sess.GroupInvite(ctx, req.GroupJid, false)
	if err != nil {
		v2Engine(c, err)
		return
	}
	text := link
	if req.Description != "" {
		text = req.Description + "\n\n" + link
	}
	for _, n := range req.Numbers {
		to, err := sess.ResolveJID(ctx, n)
		if err != nil {
			v2Engine(c, err)
			return
		}
		if _, err := sess.Send(ctx, to, engine.Text(text), engine.SendOptions{}); err != nil {
			v2Engine(c, err)
			return
		}
	}
	c.JSON(http.StatusCreated, gin.H{"send": true, "inviteUrl": link})
}

func (s *Server) v2GroupUpdateParticipant(c *gin.Context) {
	g, ok := groupParam(c)
	if !ok {
		return
	}
	var req struct {
		Action       string   `json:"action"`
		Participants []string `json:"participants"`
	}
	if !v2Bind(c, &req) {
		return
	}
	res, err := s.session(c).GroupParticipants(c.Request.Context(), g, req.Action, req.Participants)
	if err != nil {
		v2Engine(c, err)
		return
	}
	list, _ := res.([]types.GroupParticipant)
	out := make([]gin.H, 0, len(list))
	for _, p := range list {
		status := "200"
		if p.Error != 0 {
			status = strconv.Itoa(p.Error)
		}
		out = append(out, gin.H{"jid": p.JID.String(), "status": status})
	}
	c.JSON(http.StatusCreated, gin.H{"updateParticipants": out})
}

func (s *Server) v2GroupUpdateSetting(c *gin.Context) {
	g, ok := groupParam(c)
	if !ok {
		return
	}
	var req struct {
		Action string `json:"action"`
	}
	if !v2Bind(c, &req) {
		return
	}
	if err := s.session(c).GroupSetting(c.Request.Context(), g, req.Action); err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"updateSetting": true})
}

func (s *Server) v2GroupEphemeral(c *gin.Context) {
	g, ok := groupParam(c)
	if !ok {
		return
	}
	var req struct {
		Expiration int64 `json:"expiration"`
	}
	if !v2Bind(c, &req) {
		return
	}
	if !strings.Contains(g, "@") {
		g += "@g.us"
	}
	if err := s.session(c).Ephemeral(c.Request.Context(), g, req.Expiration); err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true})
}

func (s *Server) v2GroupLeave(c *gin.Context) {
	g, ok := groupParam(c)
	if !ok {
		return
	}
	if err := s.session(c).GroupLeave(c.Request.Context(), g); err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"groupJid": g, "leave": true})
}

// ---------------------------------------------------------------- etiquetas e chamadas

func (s *Server) v2FindLabels(c *gin.Context) {
	list, err := s.Repo.Labels(current(c).Id)
	if err != nil {
		v2Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]gin.H, 0, len(list))
	for _, l := range list {
		out = append(out, gin.H{"id": l.LabelID, "name": l.Name, "color": l.Color})
	}
	c.JSON(http.StatusOK, out)
}

func (s *Server) v2HandleLabel(c *gin.Context) {
	var req struct {
		Number  string `json:"number"`
		LabelID string `json:"labelId"`
		Action  string `json:"action"`
	}
	if !v2Bind(c, &req) {
		return
	}
	if req.Action != "add" && req.Action != "remove" {
		v2Fail(c, http.StatusBadRequest, "action must be add or remove")
		return
	}
	if err := s.session(c).Label(c.Request.Context(), req.Number, req.LabelID, req.Action == "add"); err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"numberJid": req.Number, "labelId": req.LabelID, "add": req.Action == "add", "remove": req.Action == "remove"})
}

// v2CallOffer: POST /call/offer/:instance {"number": "5511...", "isVideo": false, "callDuration": 15}
// faz o número tocar pelo tempo pedido e desliga (sem atendente). Precisa das chamadas ligadas
// na instância (/call/settings). Para conversar, use /call/start com o navegador.
func (s *Server) v2CallOffer(c *gin.Context) {
	var req struct {
		Number       string `json:"number"`
		IsVideo      bool   `json:"isVideo"`
		CallDuration int    `json:"callDuration"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Number == "" {
		v2Fail(c, http.StatusBadRequest, "informe number")
		return
	}
	if s.Calls == nil {
		v2Fail(c, http.StatusServiceUnavailable, "chamadas indisponíveis neste servidor")
		return
	}
	call, err := s.Calls.Ring(c.Request.Context(), s.session(c), req.Number, req.IsVideo, time.Duration(req.CallDuration)*time.Second)
	if err != nil {
		if errors.Is(err, calls.ErrDisabled) {
			v2Fail(c, http.StatusPreconditionFailed, err.Error())
			return
		}
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": call["callId"], "status": "ringing", "isVideo": req.IsVideo, "number": req.Number, "call": call})
}
