package evolutionapi

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

func participantsOut(ps []types.GroupParticipant) []gin.H {
	out := make([]gin.H, 0, len(ps))
	for _, p := range ps {
		var admin interface{}
		switch {
		case p.IsSuperAdmin:
			admin = "superadmin"
		case p.IsAdmin:
			admin = "admin"
		}
		item := gin.H{"id": p.JID.String(), "admin": admin}
		if !p.PhoneNumber.IsEmpty() {
			item["phoneNumber"] = p.PhoneNumber.String()
		}
		if !p.LID.IsEmpty() {
			item["lid"] = p.LID.String()
		}
		out = append(out, item)
	}
	return out
}

func groupOut(cli *whatsmeow.Client, g *types.GroupInfo, withParticipants, withPicture bool) gin.H {
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
	}
	if g.IsParent || !g.LinkedParentJID.IsEmpty() {
		out["linkedParent"] = g.LinkedParentJID.String()
	}
	if withPicture && cli != nil {
		out["pictureUrl"] = profilePicture(cli, g.JID)
	}
	if withParticipants {
		out["participants"] = participantsOut(g.Participants)
	}
	return out
}

func groupJIDParam(c *gin.Context) (types.JID, bool) {
	raw := c.Query("groupJid")
	if raw == "" {
		badRequest(c, "groupJid query parameter is required")
		return types.EmptyJID, false
	}
	if !strings.Contains(raw, "@") {
		raw += "@g.us"
	}
	jid, err := types.ParseJID(raw)
	if err != nil || jid.Server != types.GroupServer {
		badRequest(c, "invalid groupJid")
		return types.EmptyJID, false
	}
	return jid, true
}

func inviteCodeFrom(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	return s
}

// POST /group/create/:instance
func (h *Handler) GroupCreate(c *gin.Context) {
	var req struct {
		Subject      string   `json:"subject"`
		Description  string   `json:"description"`
		Participants []string `json:"participants"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.Subject == "" || len(req.Participants) == 0 {
		badRequest(c, "subject and participants are required")
		return
	}
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	ps := make([]types.JID, 0, len(req.Participants))
	for _, p := range req.Participants {
		jid, err := parseJID(p)
		if err != nil {
			badRequest(c, err.Error())
			return
		}
		ps = append(ps, jid)
	}
	g, err := cli.CreateGroup(ctx(), whatsmeow.ReqCreateGroup{Name: req.Subject, Participants: ps})
	if err != nil {
		internalError(c, err)
		return
	}
	if req.Description != "" {
		if err := cli.SetGroupTopic(ctx(), g.JID, "", "", req.Description); err == nil {
			g.Topic = req.Description
		}
	}
	c.JSON(http.StatusCreated, groupOut(cli, g, true, false))
}

// POST /group/updateGroupSubject/:instance?groupJid=
func (h *Handler) GroupUpdateSubject(c *gin.Context) {
	jid, ok := groupJIDParam(c)
	if !ok {
		return
	}
	var req struct {
		Subject string `json:"subject"`
	}
	if !bindJSON(c, &req) {
		return
	}
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	if err := cli.SetGroupName(ctx(), jid, req.Subject); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"update": "success"})
}

// POST /group/updateGroupDescription/:instance?groupJid=
func (h *Handler) GroupUpdateDescription(c *gin.Context) {
	jid, ok := groupJIDParam(c)
	if !ok {
		return
	}
	var req struct {
		Description string `json:"description"`
	}
	if !bindJSON(c, &req) {
		return
	}
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	prev := ""
	if info, err := cli.GetGroupInfo(ctx(), jid); err == nil {
		prev = info.TopicID
	}
	if err := cli.SetGroupTopic(ctx(), jid, prev, "", req.Description); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"update": "success"})
}

// POST /group/updateGroupPicture/:instance?groupJid=
func (h *Handler) GroupUpdatePicture(c *gin.Context) {
	jid, ok := groupJIDParam(c)
	if !ok {
		return
	}
	var req struct {
		Image string `json:"image"`
	}
	if !bindJSON(c, &req) {
		return
	}
	img, err := loadImage(req.Image)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	if _, err := cli.SetGroupPhoto(ctx(), jid, img); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"update": "success"})
}

// GET /group/findGroupInfos/:instance?groupJid=
func (h *Handler) GroupFindInfos(c *gin.Context) {
	jid, ok := groupJIDParam(c)
	if !ok {
		return
	}
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	g, err := cli.GetGroupInfo(ctx(), jid)
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, groupOut(cli, g, true, true))
}

// GET /group/fetchAllGroups/:instance?getParticipants=true
func (h *Handler) GroupFetchAll(c *gin.Context) {
	withParticipants := c.Query("getParticipants") == "true"
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	groups, err := cli.GetJoinedGroups(ctx())
	if err != nil {
		internalError(c, err)
		return
	}
	out := make([]gin.H, 0, len(groups))
	for _, g := range groups {
		out = append(out, groupOut(cli, g, withParticipants, false))
	}
	c.JSON(http.StatusOK, out)
}

// GET /group/participants/:instance?groupJid=
func (h *Handler) GroupParticipants(c *gin.Context) {
	jid, ok := groupJIDParam(c)
	if !ok {
		return
	}
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	g, err := cli.GetGroupInfo(ctx(), jid)
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"participants": participantsOut(g.Participants)})
}

// GET /group/inviteCode/:instance?groupJid=
func (h *Handler) GroupInviteCode(c *gin.Context) {
	h.groupInvite(c, false)
}

// POST /group/revokeInviteCode/:instance?groupJid=
func (h *Handler) GroupRevokeInviteCode(c *gin.Context) {
	h.groupInvite(c, true)
}

func (h *Handler) groupInvite(c *gin.Context, reset bool) {
	jid, ok := groupJIDParam(c)
	if !ok {
		return
	}
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	link, err := cli.GetGroupInviteLink(ctx(), jid, reset)
	if err != nil {
		internalError(c, err)
		return
	}
	if reset {
		c.JSON(http.StatusCreated, gin.H{"revoked": true, "inviteCode": inviteCodeFrom(link)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"inviteUrl": link, "inviteCode": inviteCodeFrom(link)})
}

// GET /group/inviteInfo/:instance?inviteCode=
func (h *Handler) GroupInviteInfo(c *gin.Context) {
	code := inviteCodeFrom(c.Query("inviteCode"))
	if code == "" {
		badRequest(c, "inviteCode query parameter is required")
		return
	}
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	g, err := cli.GetGroupInfoFromLink(ctx(), code)
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, groupOut(cli, g, true, false))
}

// GET /group/acceptInviteCode/:instance?inviteCode=
func (h *Handler) GroupAcceptInviteCode(c *gin.Context) {
	code := inviteCodeFrom(c.Query("inviteCode"))
	if code == "" {
		badRequest(c, "inviteCode query parameter is required")
		return
	}
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	jid, err := cli.JoinGroupWithLink(ctx(), code)
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"accepted": true, "groupJid": jid.String()})
}

// POST /group/sendInvite/:instance
func (h *Handler) GroupSendInvite(c *gin.Context) {
	var req struct {
		GroupJid    string   `json:"groupJid"`
		Description string   `json:"description"`
		Numbers     []string `json:"numbers"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.GroupJid == "" || len(req.Numbers) == 0 {
		badRequest(c, "groupJid and numbers are required")
		return
	}
	raw := req.GroupJid
	if !strings.Contains(raw, "@") {
		raw += "@g.us"
	}
	gjid, err := types.ParseJID(raw)
	if err != nil {
		badRequest(c, "invalid groupJid")
		return
	}
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	link, err := cli.GetGroupInviteLink(ctx(), gjid, false)
	if err != nil {
		internalError(c, err)
		return
	}
	text := link
	if req.Description != "" {
		text = req.Description + "\n\n" + link
	}
	for _, n := range req.Numbers {
		to, err := parseJID(n)
		if err != nil {
			badRequest(c, err.Error())
			return
		}
		if _, err := cli.SendMessage(ctx(), to, &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String(text), MatchedText: proto.String(link)},
		}); err != nil {
			internalError(c, err)
			return
		}
	}
	c.JSON(http.StatusCreated, gin.H{"send": true, "inviteUrl": link})
}

// POST /group/updateParticipant/:instance?groupJid=
func (h *Handler) GroupUpdateParticipant(c *gin.Context) {
	jid, ok := groupJIDParam(c)
	if !ok {
		return
	}
	var req struct {
		Action       string   `json:"action"`
		Participants []string `json:"participants"`
	}
	if !bindJSON(c, &req) {
		return
	}
	var action whatsmeow.ParticipantChange
	switch req.Action {
	case "add":
		action = whatsmeow.ParticipantChangeAdd
	case "remove":
		action = whatsmeow.ParticipantChangeRemove
	case "promote":
		action = whatsmeow.ParticipantChangePromote
	case "demote":
		action = whatsmeow.ParticipantChangeDemote
	default:
		badRequest(c, "action must be one of add, remove, promote, demote")
		return
	}
	ps := make([]types.JID, 0, len(req.Participants))
	for _, p := range req.Participants {
		pj, err := parseJID(p)
		if err != nil {
			badRequest(c, err.Error())
			return
		}
		ps = append(ps, pj)
	}
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	res, err := cli.UpdateGroupParticipants(ctx(), jid, ps, action)
	if err != nil {
		internalError(c, err)
		return
	}
	updated := make([]gin.H, 0, len(res))
	for _, p := range res {
		status := "200"
		if p.Error != 0 {
			status = fmt.Sprint(p.Error)
		}
		updated = append(updated, gin.H{"jid": p.JID.String(), "status": status})
	}
	c.JSON(http.StatusCreated, gin.H{"updateParticipants": updated})
}

// POST /group/updateSetting/:instance?groupJid=
func (h *Handler) GroupUpdateSetting(c *gin.Context) {
	jid, ok := groupJIDParam(c)
	if !ok {
		return
	}
	var req struct {
		Action string `json:"action"`
	}
	if !bindJSON(c, &req) {
		return
	}
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	var err error
	switch req.Action {
	case "announcement":
		err = cli.SetGroupAnnounce(ctx(), jid, true)
	case "not_announcement":
		err = cli.SetGroupAnnounce(ctx(), jid, false)
	case "locked":
		err = cli.SetGroupLocked(ctx(), jid, true)
	case "unlocked":
		err = cli.SetGroupLocked(ctx(), jid, false)
	default:
		badRequest(c, "action must be one of announcement, not_announcement, locked, unlocked")
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"updateSetting": true})
}

// POST /group/toggleEphemeral/:instance?groupJid=
func (h *Handler) GroupToggleEphemeral(c *gin.Context) {
	jid, ok := groupJIDParam(c)
	if !ok {
		return
	}
	var req struct {
		Expiration int64 `json:"expiration"`
	}
	if !bindJSON(c, &req) {
		return
	}
	switch req.Expiration {
	case 0, 86400, 604800, 7776000:
	default:
		badRequest(c, "expiration must be 0, 86400 (24h), 604800 (7d) or 7776000 (90d)")
		return
	}
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	if err := cli.SetDisappearingTimer(ctx(), jid, time.Duration(req.Expiration)*time.Second, time.Now()); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true})
}

// DELETE /group/leaveGroup/:instance?groupJid=
func (h *Handler) GroupLeave(c *gin.Context) {
	jid, ok := groupJIDParam(c)
	if !ok {
		return
	}
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	if err := cli.LeaveGroup(ctx(), jid); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"groupJid": jid.String(), "leave": true})
}
