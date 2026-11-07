package evolutionapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type messageKey struct {
	RemoteJid   string `json:"remoteJid"`
	FromMe      bool   `json:"fromMe"`
	ID          string `json:"id"`
	Participant string `json:"participant"`
}

type lastMessage struct {
	Key              messageKey `json:"key"`
	MessageTimestamp int64      `json:"messageTimestamp"`
}

func (k messageKey) proto() *waCommon.MessageKey {
	mk := &waCommon.MessageKey{
		RemoteJID: proto.String(k.RemoteJid),
		FromMe:    proto.Bool(k.FromMe),
		ID:        proto.String(k.ID),
	}
	if k.Participant != "" {
		mk.Participant = proto.String(k.Participant)
	}
	return mk
}

func (lm *lastMessage) parts() (time.Time, *waCommon.MessageKey) {
	if lm == nil || lm.Key.ID == "" {
		return time.Now(), nil
	}
	ts := time.Now()
	if lm.MessageTimestamp > 0 {
		ts = time.Unix(lm.MessageTimestamp, 0)
	}
	return ts, lm.Key.proto()
}

// POST /chat/whatsappNumbers/:instance
func (h *Handler) WhatsappNumbers(c *gin.Context) {
	var req struct {
		Numbers []string `json:"numbers"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if len(req.Numbers) == 0 {
		badRequest(c, "numbers is required")
		return
	}
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	phones := make([]string, 0, len(req.Numbers))
	for _, n := range req.Numbers {
		d := onlyDigits(strings.Split(n, "@")[0])
		if d != "" {
			phones = append(phones, "+"+d)
		}
	}
	res, err := cli.IsOnWhatsApp(ctx(), phones)
	if err != nil {
		internalError(c, err)
		return
	}
	out := make([]gin.H, 0, len(res))
	for _, r := range res {
		item := gin.H{
			"exists": r.IsIn,
			"jid":    r.JID.String(),
			"number": strings.TrimPrefix(r.Query, "+"),
		}
		if r.VerifiedName != nil && r.VerifiedName.Details != nil {
			item["name"] = r.VerifiedName.Details.GetVerifiedName()
		}
		out = append(out, item)
	}
	c.JSON(http.StatusOK, out)
}

// POST /chat/markMessageAsRead/:instance
func (h *Handler) MarkMessageAsRead(c *gin.Context) {
	var req struct {
		ReadMessages []messageKey `json:"readMessages"`
	}
	if !bindJSON(c, &req) {
		return
	}
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	type group struct {
		chat, sender types.JID
		ids          []types.MessageID
	}
	groups := map[string]*group{}
	for _, k := range req.ReadMessages {
		if k.FromMe || k.ID == "" {
			continue
		}
		chat, err := parseJID(k.RemoteJid)
		if err != nil {
			continue
		}
		sender := chat
		if k.Participant != "" {
			if p, err := parseJID(k.Participant); err == nil {
				sender = p
			}
		}
		key := chat.String() + "|" + sender.String()
		if groups[key] == nil {
			groups[key] = &group{chat: chat, sender: sender}
		}
		groups[key].ids = append(groups[key].ids, k.ID)
	}
	for _, g := range groups {
		if err := cli.MarkRead(ctx(), g.ids, time.Now(), g.chat, g.sender); err != nil {
			internalError(c, err)
			return
		}
	}
	c.JSON(http.StatusCreated, gin.H{"message": "Read messages", "read": "success"})
}

// POST /chat/archiveChat/:instance
func (h *Handler) ArchiveChat(c *gin.Context) {
	var req struct {
		LastMessage *lastMessage `json:"lastMessage"`
		Chat        string       `json:"chat"`
		Archive     bool         `json:"archive"`
	}
	if !bindJSON(c, &req) {
		return
	}
	target := req.Chat
	if target == "" && req.LastMessage != nil {
		target = req.LastMessage.Key.RemoteJid
	}
	jid, err := parseJID(target)
	if err != nil {
		badRequest(c, "chat or lastMessage.key.remoteJid is required")
		return
	}
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	ts, key := req.LastMessage.parts()
	if err := cli.SendAppState(ctx(), appstate.BuildArchive(jid, req.Archive, ts, key)); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"chatId": jid.String(), "archived": req.Archive})
}

// POST /chat/markChatUnread/:instance
func (h *Handler) MarkChatUnread(c *gin.Context) {
	var req struct {
		LastMessage *lastMessage `json:"lastMessage"`
		Chat        string       `json:"chat"`
	}
	if !bindJSON(c, &req) {
		return
	}
	target := req.Chat
	if target == "" && req.LastMessage != nil {
		target = req.LastMessage.Key.RemoteJid
	}
	jid, err := parseJID(target)
	if err != nil {
		badRequest(c, "chat or lastMessage.key.remoteJid is required")
		return
	}
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	ts, key := req.LastMessage.parts()
	if err := cli.SendAppState(ctx(), appstate.BuildMarkChatAsRead(jid, false, ts, key)); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"chatId": jid.String(), "markedChatUnread": true})
}

// DELETE /chat/deleteMessageForEveryone/:instance
func (h *Handler) DeleteMessageForEveryone(c *gin.Context) {
	var req messageKey
	if !bindJSON(c, &req) {
		return
	}
	if req.ID == "" || req.RemoteJid == "" {
		badRequest(c, "id and remoteJid are required")
		return
	}
	cli, inst, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	chat, err := parseJID(req.RemoteJid)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	sender := types.EmptyJID
	if !req.FromMe {
		// Revoking someone else's message only works for group admins.
		if req.Participant == "" {
			badRequest(c, "only messages sent by this instance can be deleted, or set participant for group admin deletes")
			return
		}
		if sender, err = parseJID(req.Participant); err != nil {
			badRequest(c, err.Error())
			return
		}
	}
	msg := cli.BuildRevoke(chat, sender, req.ID)
	h.sendRaw(c, cli, inst, chat, msg, 0)
}

// POST /chat/updateMessage/:instance
func (h *Handler) UpdateMessage(c *gin.Context) {
	var req struct {
		Number string     `json:"number"`
		Key    messageKey `json:"key"`
		Text   string     `json:"text"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.Key.ID == "" || req.Text == "" {
		badRequest(c, "key.id and text are required")
		return
	}
	if !req.Key.FromMe {
		badRequest(c, "only messages sent by this instance can be edited")
		return
	}
	target := req.Number
	if target == "" {
		target = req.Key.RemoteJid
	}
	chat, err := parseJID(target)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	cli, inst, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	msg := cli.BuildEdit(chat, req.Key.ID, &waE2E.Message{Conversation: proto.String(req.Text)})
	h.sendRaw(c, cli, inst, chat, msg, 0)
}

// POST /chat/sendPresence/:instance
func (h *Handler) SendPresence(c *gin.Context) {
	var req struct {
		Number   string `json:"number"`
		Delay    int32  `json:"delay"`
		Presence string `json:"presence"`
	}
	if !bindJSON(c, &req) {
		return
	}
	to, err := parseJID(req.Number)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	switch req.Presence {
	case "composing", "recording":
		media := types.ChatPresenceMediaText
		if req.Presence == "recording" {
			media = types.ChatPresenceMediaAudio
		}
		if err := cli.SendChatPresence(ctx(), to, types.ChatPresenceComposing, media); err != nil {
			internalError(c, err)
			return
		}
		if req.Delay > 0 {
			go func() {
				time.Sleep(time.Duration(req.Delay) * time.Millisecond)
				_ = cli.SendChatPresence(ctx(), to, types.ChatPresencePaused, media)
			}()
		}
	case "paused":
		if err := cli.SendChatPresence(ctx(), to, types.ChatPresencePaused, types.ChatPresenceMediaText); err != nil {
			internalError(c, err)
			return
		}
	case "available", "unavailable":
		if err := cli.SendPresence(ctx(), types.Presence(req.Presence)); err != nil {
			internalError(c, err)
			return
		}
	default:
		badRequest(c, "presence must be one of composing, recording, paused, available, unavailable")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"presence": req.Presence})
}

// POST /chat/updateBlockStatus/:instance
func (h *Handler) UpdateBlockStatus(c *gin.Context) {
	var req struct {
		Number string `json:"number"`
		Status string `json:"status"`
	}
	if !bindJSON(c, &req) {
		return
	}
	jid, err := parseJID(req.Number)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	var action events.BlocklistChangeAction
	switch req.Status {
	case "block":
		action = events.BlocklistChangeActionBlock
	case "unblock":
		action = events.BlocklistChangeActionUnblock
	default:
		badRequest(c, "status must be block or unblock")
		return
	}
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	if _, err := cli.UpdateBlocklist(ctx(), jid, action); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"accepted": []string{jid.String()}})
}

// POST /chat/fetchProfilePictureUrl/:instance
func (h *Handler) FetchProfilePictureUrl(c *gin.Context) {
	var req struct {
		Number string `json:"number"`
	}
	if !bindJSON(c, &req) {
		return
	}
	jid, err := parseJID(req.Number)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"wuid": jid.String(), "profilePictureUrl": profilePicture(cli, jid)})
}

func profilePicture(cli *whatsmeow.Client, jid types.JID) interface{} {
	pic, err := cli.GetProfilePictureInfo(ctx(), jid, &whatsmeow.GetProfilePictureParams{})
	if err != nil || pic == nil {
		return nil
	}
	return pic.URL
}

// POST /chat/fetchProfile/:instance
func (h *Handler) FetchProfile(c *gin.Context) {
	var req struct {
		Number string `json:"number"`
	}
	_ = c.ShouldBindJSON(&req)
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	var jid types.JID
	if req.Number == "" {
		jid = cli.Store.ID.ToNonAD()
	} else {
		var err error
		if jid, err = parseJID(req.Number); err != nil {
			badRequest(c, err.Error())
			return
		}
	}
	resp := gin.H{
		"wuid":         jid.String(),
		"numberExists": false,
		"picture":      profilePicture(cli, jid),
		"isBusiness":   false,
	}
	if infos, err := cli.GetUserInfo(ctx(), []types.JID{jid}); err == nil {
		if info, ok := infos[jid]; ok {
			resp["numberExists"] = true
			resp["status"] = gin.H{"status": info.Status}
			if info.VerifiedName != nil && info.VerifiedName.Details != nil {
				resp["name"] = info.VerifiedName.Details.GetVerifiedName()
				resp["isBusiness"] = true
			}
		}
	}
	if contact, err := cli.Store.Contacts.GetContact(ctx(), jid); err == nil && contact.Found {
		if _, has := resp["name"]; !has {
			name := contact.FullName
			if name == "" {
				name = contact.PushName
			}
			resp["name"] = name
		}
	}
	if jid.User == cli.Store.ID.User {
		resp["name"] = cli.Store.PushName
	}
	if bp, err := cli.GetBusinessProfile(ctx(), jid); err == nil && bp != nil {
		resp["isBusiness"] = true
		resp["email"] = bp.Email
		resp["address"] = bp.Address
		resp["description"] = bp.ProfileOptions["description"]
		resp["website"] = bp.ProfileOptions["website"]
	}
	c.JSON(http.StatusOK, resp)
}

// POST /chat/fetchBusinessProfile/:instance
func (h *Handler) FetchBusinessProfile(c *gin.Context) {
	var req struct {
		Number string `json:"number"`
	}
	if !bindJSON(c, &req) {
		return
	}
	jid, err := parseJID(req.Number)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	bp, err := cli.GetBusinessProfile(ctx(), jid)
	if err != nil {
		internalError(c, err)
		return
	}
	if bp == nil {
		c.JSON(http.StatusOK, gin.H{"wid": jid.String(), "isBusiness": false})
		return
	}
	categories := make([]string, 0, len(bp.Categories))
	for _, cat := range bp.Categories {
		categories = append(categories, cat.Name)
	}
	c.JSON(http.StatusOK, gin.H{
		"wid":           jid.String(),
		"isBusiness":    true,
		"address":       bp.Address,
		"email":         bp.Email,
		"categories":    categories,
		"description":   bp.ProfileOptions["description"],
		"website":       bp.ProfileOptions["website"],
		"businessHours": bp.BusinessHours,
		"timezone":      bp.BusinessHoursTimeZone,
	})
}

// POST /chat/findContacts/:instance
func (h *Handler) FindContacts(c *gin.Context) {
	var req struct {
		Where struct {
			ID        string `json:"id"`
			RemoteJid string `json:"remoteJid"`
		} `json:"where"`
	}
	_ = c.ShouldBindJSON(&req)
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	filter := req.Where.RemoteJid
	if filter == "" {
		filter = req.Where.ID
	}
	contacts, err := cli.Store.Contacts.GetAllContacts(ctx())
	if err != nil {
		internalError(c, err)
		return
	}
	out := make([]gin.H, 0, len(contacts))
	for jid, info := range contacts {
		if filter != "" && jid.String() != filter && jid.User != onlyDigits(filter) {
			continue
		}
		name := info.FullName
		if name == "" {
			name = info.FirstName
		}
		out = append(out, gin.H{
			"id":           jid.String(),
			"remoteJid":    jid.String(),
			"pushName":     info.PushName,
			"name":         name,
			"businessName": info.BusinessName,
		})
	}
	c.JSON(http.StatusOK, out)
}

// POST /chat/getBase64FromMediaMessage/:instance
//
// Body: {"message": {"key": {...}, "message": {<proto message>}}}. The media
// is downloaded from WhatsApp using the keys inside the message itself.
func (h *Handler) GetBase64FromMediaMessage(c *gin.Context) {
	var req struct {
		Message struct {
			Key     messageKey      `json:"key"`
			Message json.RawMessage `json:"message"`
		} `json:"message"`
		ConvertToMp4 bool `json:"convertToMp4"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if len(req.Message.Message) == 0 {
		badRequest(c, "message.message (the full WhatsApp message received in the webhook) is required")
		return
	}
	msg := &waE2E.Message{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(req.Message.Message, msg); err != nil {
		badRequest(c, "invalid message: "+err.Error())
		return
	}
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	var (
		d                      whatsmeow.DownloadableMessage
		mediaType, mime, fname string
		caption                string
	)
	switch {
	case msg.GetImageMessage() != nil:
		m := msg.GetImageMessage()
		d, mediaType, mime, caption = m, "imageMessage", m.GetMimetype(), m.GetCaption()
	case msg.GetVideoMessage() != nil:
		m := msg.GetVideoMessage()
		d, mediaType, mime, caption = m, "videoMessage", m.GetMimetype(), m.GetCaption()
	case msg.GetPtvMessage() != nil:
		m := msg.GetPtvMessage()
		d, mediaType, mime = m, "ptvMessage", m.GetMimetype()
	case msg.GetAudioMessage() != nil:
		m := msg.GetAudioMessage()
		d, mediaType, mime = m, "audioMessage", m.GetMimetype()
	case msg.GetDocumentMessage() != nil:
		m := msg.GetDocumentMessage()
		d, mediaType, mime, fname, caption = m, "documentMessage", m.GetMimetype(), m.GetFileName(), m.GetCaption()
	case msg.GetDocumentWithCaptionMessage().GetMessage().GetDocumentMessage() != nil:
		m := msg.GetDocumentWithCaptionMessage().GetMessage().GetDocumentMessage()
		d, mediaType, mime, fname, caption = m, "documentMessage", m.GetMimetype(), m.GetFileName(), m.GetCaption()
	case msg.GetStickerMessage() != nil:
		m := msg.GetStickerMessage()
		d, mediaType, mime = m, "stickerMessage", m.GetMimetype()
	default:
		badRequest(c, "message has no downloadable media")
		return
	}
	data, err := cli.Download(ctx(), d)
	if err != nil {
		internalError(c, err)
		return
	}
	if fname == "" {
		fname = req.Message.Key.ID
	}
	c.JSON(http.StatusCreated, gin.H{
		"mediaType": mediaType,
		"fileName":  fname,
		"caption":   caption,
		"size":      gin.H{"fileLength": len(data)},
		"mimetype":  mime,
		"base64":    base64.StdEncoding.EncodeToString(data),
	})
}

// POST /chat/updateProfileName/:instance
func (h *Handler) UpdateProfileName(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.Name == "" {
		badRequest(c, "name is required")
		return
	}
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	if err := cli.SendAppState(ctx(), appstate.BuildSettingPushName(req.Name)); err != nil {
		internalError(c, err)
		return
	}
	cli.Store.PushName = req.Name
	_ = cli.Store.Save(ctx())
	c.JSON(http.StatusCreated, gin.H{"update": "success"})
}

// POST /chat/updateProfileStatus/:instance
func (h *Handler) UpdateProfileStatus(c *gin.Context) {
	var req struct {
		Status string `json:"status"`
	}
	if !bindJSON(c, &req) {
		return
	}
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	if err := cli.SetStatusMessage(ctx(), req.Status); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"update": "success"})
}

// POST /chat/updateProfilePicture/:instance
func (h *Handler) UpdateProfilePicture(c *gin.Context) {
	var req struct {
		Picture string `json:"picture"`
	}
	if !bindJSON(c, &req) {
		return
	}
	img, err := loadImage(req.Picture)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	if _, err := cli.SetGroupPhoto(ctx(), types.EmptyJID, img); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"update": "success"})
}

// DELETE /chat/removeProfilePicture/:instance
func (h *Handler) RemoveProfilePicture(c *gin.Context) {
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	if _, err := cli.SetGroupPhoto(ctx(), types.EmptyJID, nil); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"update": "success"})
}

// GET /chat/fetchPrivacySettings/:instance
func (h *Handler) FetchPrivacySettings(c *gin.Context) {
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	s := cli.GetPrivacySettings(ctx())
	c.JSON(http.StatusOK, gin.H{
		"readreceipts": s.ReadReceipts,
		"profile":      s.Profile,
		"status":       s.Status,
		"online":       s.Online,
		"last":         s.LastSeen,
		"groupadd":     s.GroupAdd,
		"calladd":      s.CallAdd,
	})
}

// POST /chat/updatePrivacySettings/:instance
func (h *Handler) UpdatePrivacySettings(c *gin.Context) {
	var req map[string]string
	if !bindJSON(c, &req) {
		return
	}
	names := map[string]types.PrivacySettingType{
		"readreceipts": types.PrivacySettingTypeReadReceipts,
		"profile":      types.PrivacySettingTypeProfile,
		"status":       types.PrivacySettingTypeStatus,
		"online":       types.PrivacySettingTypeOnline,
		"last":         types.PrivacySettingTypeLastSeen,
		"groupadd":     types.PrivacySettingTypeGroupAdd,
		"calladd":      types.PrivacySettingTypeCallAdd,
	}
	cli, _, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	for k, v := range req {
		t, known := names[strings.ToLower(k)]
		if !known || v == "" {
			continue
		}
		if _, err := cli.SetPrivacySetting(ctx(), t, types.PrivacySetting(v)); err != nil {
			internalError(c, err)
			return
		}
	}
	c.JSON(http.StatusCreated, gin.H{"update": "success", "data": req})
}

// loadImage accepts an image URL or base64 string.
func loadImage(src string) ([]byte, error) {
	if src == "" {
		return nil, errBadInput("picture is required")
	}
	if isURL(src) {
		return downloadURL(src)
	}
	return decodeMedia(src)
}

type errBadInput string

func (e errBadInput) Error() string { return string(e) }
