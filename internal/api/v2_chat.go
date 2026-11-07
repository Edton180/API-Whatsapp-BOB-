// API WhatsApp BOB - Desenvolvido por Edton Braz.

package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/edton180/api-whatsapp-bob/internal/engine"
)

type v2Key struct {
	RemoteJid   string `json:"remoteJid"`
	FromMe      bool   `json:"fromMe"`
	ID          string `json:"id"`
	Participant string `json:"participant"`
}

type v2LastMessage struct {
	Key              v2Key `json:"key"`
	MessageTimestamp int64 `json:"messageTimestamp"`
}

func (lm *v2LastMessage) engine() *engine.LastMessage {
	if lm == nil || lm.Key.ID == "" {
		return nil
	}
	out := &engine.LastMessage{ID: lm.Key.ID, FromMe: lm.Key.FromMe, Sender: lm.Key.Participant}
	if lm.MessageTimestamp > 0 {
		out.Timestamp = time.Unix(lm.MessageTimestamp, 0)
	}
	return out
}

func (s *Server) v2WhatsappNumbers(c *gin.Context) {
	var req struct {
		Numbers []string `json:"numbers"`
	}
	if !v2Bind(c, &req) {
		return
	}
	res, err := s.session(c).Check(c.Request.Context(), req.Numbers)
	if err != nil {
		v2Engine(c, err)
		return
	}
	out := make([]gin.H, 0, len(res))
	for _, r := range res {
		row := gin.H{"exists": r["IsInWhatsapp"], "jid": r["JID"], "number": strings.TrimPrefix(r["Query"].(string), "+")}
		if n, ok := r["VerifiedName"]; ok {
			row["name"] = n
		}
		out = append(out, row)
	}
	c.JSON(http.StatusOK, out)
}

func (s *Server) v2MarkRead(c *gin.Context) {
	var req struct {
		ReadMessages []v2Key `json:"readMessages"`
	}
	if !v2Bind(c, &req) {
		return
	}
	byChat := map[string][]string{}
	for _, k := range req.ReadMessages {
		if !k.FromMe && k.ID != "" && k.RemoteJid != "" {
			byChat[k.RemoteJid] = append(byChat[k.RemoteJid], k.ID)
		}
	}
	sess := s.session(c)
	for chat, ids := range byChat {
		if err := sess.MarkRead(c.Request.Context(), chat, ids); err != nil {
			v2Engine(c, err)
			return
		}
	}
	c.JSON(http.StatusCreated, gin.H{"message": "Read messages", "read": "success"})
}

func (s *Server) v2ArchiveChat(c *gin.Context) {
	var req struct {
		LastMessage *v2LastMessage `json:"lastMessage"`
		Chat        string         `json:"chat"`
		Archive     bool           `json:"archive"`
	}
	if !v2Bind(c, &req) {
		return
	}
	chat := req.Chat
	if chat == "" && req.LastMessage != nil {
		chat = req.LastMessage.Key.RemoteJid
	}
	action := "unarchive"
	if req.Archive {
		action = "archive"
	}
	if err := s.session(c).ChatWithLast(c.Request.Context(), action, chat, req.LastMessage.engine()); err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"chatId": chat, "archived": req.Archive})
}

func (s *Server) v2MarkChatUnread(c *gin.Context) {
	var req struct {
		LastMessage *v2LastMessage `json:"lastMessage"`
		Chat        string         `json:"chat"`
	}
	if !v2Bind(c, &req) {
		return
	}
	chat := req.Chat
	if chat == "" && req.LastMessage != nil {
		chat = req.LastMessage.Key.RemoteJid
	}
	if err := s.session(c).ChatWithLast(c.Request.Context(), "unread", chat, req.LastMessage.engine()); err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"chatId": chat, "markedChatUnread": true})
}

func (s *Server) v2DeleteForEveryone(c *gin.Context) {
	var req v2Key
	if !v2Bind(c, &req) {
		return
	}
	if req.ID == "" || req.RemoteJid == "" {
		v2Fail(c, http.StatusBadRequest, "id and remoteJid are required")
		return
	}
	sender := ""
	if !req.FromMe {
		if req.Participant == "" {
			v2Fail(c, http.StatusBadRequest, "para apagar mensagem de outra pessoa (admin de grupo) informe participant")
			return
		}
		sender = req.Participant
	}
	res, err := s.session(c).RevokeAs(c.Request.Context(), req.RemoteJid, req.ID, sender)
	if err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusCreated, v2Message(current(c), res))
}

func (s *Server) v2UpdateMessage(c *gin.Context) {
	var req struct {
		Number string `json:"number"`
		Key    v2Key  `json:"key"`
		Text   string `json:"text"`
	}
	if !v2Bind(c, &req) {
		return
	}
	chat := req.Number
	if chat == "" {
		chat = req.Key.RemoteJid
	}
	res, err := s.session(c).Edit(c.Request.Context(), chat, req.Key.ID, req.Text)
	if err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusCreated, v2Message(current(c), res))
}

func (s *Server) v2SendPresence(c *gin.Context) {
	var req struct {
		Number   string `json:"number"`
		Delay    int    `json:"delay"`
		Presence string `json:"presence"`
	}
	if !v2Bind(c, &req) {
		return
	}
	if err := s.session(c).Presence(c.Request.Context(), req.Number, req.Presence, false, req.Delay); err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"presence": req.Presence})
}

func (s *Server) v2UpdateBlockStatus(c *gin.Context) {
	var req struct {
		Number string `json:"number"`
		Status string `json:"status"`
	}
	if !v2Bind(c, &req) {
		return
	}
	if req.Status != "block" && req.Status != "unblock" {
		v2Fail(c, http.StatusBadRequest, "status must be block or unblock")
		return
	}
	if _, err := s.session(c).Block(c.Request.Context(), req.Number, req.Status == "block"); err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"accepted": []string{req.Number}})
}

func (s *Server) v2ProfilePictureURL(c *gin.Context) {
	var req struct {
		Number string `json:"number"`
	}
	if !v2Bind(c, &req) {
		return
	}
	res, err := s.session(c).Avatar(c.Request.Context(), req.Number, false)
	if err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"wuid": req.Number, "profilePictureUrl": res["url"]})
}

func (s *Server) v2FetchProfile(c *gin.Context) {
	var req struct {
		Number string `json:"number"`
	}
	_ = c.ShouldBindJSON(&req)
	sess := s.session(c)
	ctx := c.Request.Context()
	cli, err := sess.Client()
	if err != nil {
		v2Engine(c, err)
		return
	}
	number := req.Number
	if number == "" && cli.Store.ID != nil {
		number = cli.Store.ID.ToNonAD().String()
	}
	jid, err := sess.ResolveJID(ctx, number)
	if err != nil {
		v2Engine(c, err)
		return
	}
	out := gin.H{"wuid": jid.String(), "numberExists": false, "isBusiness": false}
	if pic, err := sess.Avatar(ctx, jid.String(), false); err == nil {
		out["picture"] = pic["url"]
	}
	if info, err := sess.UserInfo(ctx, []string{jid.String()}); err == nil && len(info) > 0 {
		out["numberExists"] = true
		b, _ := json.Marshal(info)
		var m map[string]struct {
			Status string
		}
		if json.Unmarshal(b, &m) == nil {
			for _, u := range m {
				out["status"] = gin.H{"status": u.Status}
			}
		}
	}
	if ct, err := cli.Store.Contacts.GetContact(ctx, jid); err == nil && ct.Found {
		name := ct.FullName
		if name == "" {
			name = ct.PushName
		}
		out["name"] = name
	}
	if cli.Store.ID != nil && jid.User == cli.Store.ID.User {
		out["name"] = cli.Store.PushName
	}
	if bp, err := sess.BusinessProfile(ctx, jid.String()); err == nil && bp != nil {
		out["isBusiness"] = true
		out["email"] = bp.Email
		out["address"] = bp.Address
		out["description"] = bp.ProfileOptions["description"]
		out["website"] = bp.ProfileOptions["website"]
	}
	c.JSON(http.StatusOK, out)
}

func (s *Server) v2BusinessProfile(c *gin.Context) {
	var req struct {
		Number string `json:"number"`
	}
	if !v2Bind(c, &req) {
		return
	}
	bp, err := s.session(c).BusinessProfile(c.Request.Context(), req.Number)
	if err != nil {
		v2Engine(c, err)
		return
	}
	if bp == nil {
		c.JSON(http.StatusOK, gin.H{"wid": req.Number, "isBusiness": false})
		return
	}
	cats := make([]string, 0, len(bp.Categories))
	for _, ct := range bp.Categories {
		cats = append(cats, ct.Name)
	}
	c.JSON(http.StatusOK, gin.H{
		"wid": bp.JID.String(), "isBusiness": true, "address": bp.Address, "email": bp.Email,
		"categories": cats, "description": bp.ProfileOptions["description"], "website": bp.ProfileOptions["website"],
		"businessHours": bp.BusinessHours, "timezone": bp.BusinessHoursTimeZone,
	})
}

func (s *Server) v2FindContacts(c *gin.Context) {
	var req struct {
		Where struct {
			ID        string `json:"id"`
			RemoteJid string `json:"remoteJid"`
		} `json:"where"`
	}
	_ = c.ShouldBindJSON(&req)
	list, err := s.session(c).Contacts(c.Request.Context())
	if err != nil {
		v2Engine(c, err)
		return
	}
	filter := req.Where.RemoteJid
	if filter == "" {
		filter = req.Where.ID
	}
	out := make([]gin.H, 0, len(list))
	for _, ct := range list {
		jid, _ := ct["Jid"].(string)
		if filter != "" && jid != filter && strings.SplitN(jid, "@", 2)[0] != strings.SplitN(filter, "@", 2)[0] {
			continue
		}
		name, _ := ct["FullName"].(string)
		if name == "" {
			name, _ = ct["FirstName"].(string)
		}
		out = append(out, gin.H{"id": jid, "remoteJid": jid, "pushName": ct["PushName"], "name": name, "businessName": ct["BusinessName"]})
	}
	c.JSON(http.StatusOK, out)
}

// v2Base64FromMedia aceita {"message": {"key": {"id"}, "message": {...}}}. Sem o conteúdo,
// usa o id das mensagens recentes guardadas em memória.
func (s *Server) v2Base64FromMedia(c *gin.Context) {
	var req struct {
		Message struct {
			Key     v2Key           `json:"key"`
			Message json.RawMessage `json:"message"`
		} `json:"message"`
	}
	if !v2Bind(c, &req) {
		return
	}
	data, mime, err := s.session(c).DownloadMedia(c.Request.Context(), req.Message.Message, req.Message.Key.ID)
	if err != nil {
		v2Engine(c, err)
		return
	}
	var body map[string]json.RawMessage
	_ = json.Unmarshal(req.Message.Message, &body)
	mediaType := ""
	for k := range body {
		if strings.HasSuffix(k, "Message") {
			mediaType = k
		}
	}
	c.JSON(http.StatusCreated, gin.H{
		"mediaType": mediaType,
		"fileName":  req.Message.Key.ID,
		"size":      gin.H{"fileLength": len(data)},
		"mimetype":  mime,
		"base64":    base64.StdEncoding.EncodeToString(data),
	})
}

func (s *Server) v2UpdateProfileName(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if !v2Bind(c, &req) {
		return
	}
	if err := s.session(c).ProfileName(c.Request.Context(), req.Name); err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"update": "success"})
}

func (s *Server) v2UpdateProfileStatus(c *gin.Context) {
	var req struct {
		Status string `json:"status"`
	}
	if !v2Bind(c, &req) {
		return
	}
	if err := s.session(c).ProfileStatus(c.Request.Context(), req.Status); err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"update": "success"})
}

func (s *Server) v2UpdateProfilePicture(c *gin.Context) {
	var req struct {
		Picture string `json:"picture"`
	}
	if !v2Bind(c, &req) {
		return
	}
	if _, err := s.session(c).ProfilePicture(c.Request.Context(), req.Picture, ""); err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"update": "success"})
}

func (s *Server) v2RemoveProfilePicture(c *gin.Context) {
	if err := s.session(c).RemoveProfilePicture(c.Request.Context(), ""); err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"update": "success"})
}

func (s *Server) v2FetchPrivacy(c *gin.Context) {
	cli, err := s.session(c).Client()
	if err != nil {
		v2Engine(c, err)
		return
	}
	p := cli.GetPrivacySettings(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{
		"readreceipts": p.ReadReceipts, "profile": p.Profile, "status": p.Status,
		"online": p.Online, "last": p.LastSeen, "groupadd": p.GroupAdd, "calladd": p.CallAdd,
	})
}

func (s *Server) v2UpdatePrivacy(c *gin.Context) {
	var req map[string]string
	if !v2Bind(c, &req) {
		return
	}
	if _, err := s.session(c).SetPrivacy(c.Request.Context(), req); err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"update": "success", "data": req})
}
