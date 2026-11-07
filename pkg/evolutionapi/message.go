package evolutionapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	send_service "github.com/evolution-foundation/evolution-go/pkg/sendMessage/service"
)

// Options shared by every /message/send* route in Evolution API v2.
type sendOptions struct {
	Number           string   `json:"number"`
	Delay            int32    `json:"delay"`
	LinkPreview      *bool    `json:"linkPreview"`
	MentionsEveryOne bool     `json:"mentionsEveryOne"`
	Mentioned        []string `json:"mentioned"`
	Quoted           *quoted  `json:"quoted"`
}

type quoted struct {
	Key struct {
		ID          string `json:"id"`
		RemoteJid   string `json:"remoteJid"`
		FromMe      bool   `json:"fromMe"`
		Participant string `json:"participant"`
	} `json:"key"`
	Message json.RawMessage `json:"message"`
}

func (o *sendOptions) quotedStruct() send_service.QuotedStruct {
	if o.Quoted == nil {
		return send_service.QuotedStruct{}
	}
	return send_service.QuotedStruct{MessageID: o.Quoted.Key.ID, Participant: o.Quoted.Key.Participant}
}

func (o *sendOptions) mentioned() []string {
	out := make([]string, 0, len(o.Mentioned))
	for _, m := range o.Mentioned {
		if j, err := normalizeNumber(m); err == nil {
			out = append(out, j)
		}
	}
	return out
}

// normalize validates and rewrites o.Number into a JID.
func (o *sendOptions) normalize(c *gin.Context) bool {
	if strings.TrimSpace(o.Number) == "" {
		badRequest(c, "number is required")
		return false
	}
	n, err := normalizeNumber(o.Number)
	if err != nil {
		badRequest(c, err.Error())
		return false
	}
	o.Number = n
	return true
}

// messageResponse renders a sent message in the Evolution API v2 shape.
func messageResponse(inst *instance_model.Instance, info types.MessageInfo, msg *waE2E.Message) gin.H {
	var msgObj map[string]interface{}
	messageType := "unknown"
	if msg != nil {
		if b, err := (protojson.MarshalOptions{EmitUnpopulated: false}).Marshal(msg); err == nil {
			_ = json.Unmarshal(b, &msgObj)
		}
		messageType = messageTypeOf(msgObj)
	}
	remote := info.Chat.String()
	if remote == "" {
		remote = info.Sender.String()
	}
	ts := info.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	return gin.H{
		"key": gin.H{
			"remoteJid": remote,
			"fromMe":    true,
			"id":        info.ID,
		},
		"pushName":         info.PushName,
		"status":           "PENDING",
		"message":          msgObj,
		"messageType":      messageType,
		"messageTimestamp": ts.Unix(),
		"instanceId":       inst.Id,
		"source":           "api",
	}
}

func messageTypeOf(m map[string]interface{}) string {
	for k := range m {
		if k != "messageContextInfo" {
			return k
		}
	}
	return "unknown"
}

func sent(c *gin.Context, inst *instance_model.Instance, res *send_service.MessageSendStruct, err error) {
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, messageResponse(inst, res.Info, res.Message))
}

// ---------------------------------------------------------------------------
// POST /message/sendText/:instance
// ---------------------------------------------------------------------------

func (h *Handler) SendText(c *gin.Context) {
	var req struct {
		sendOptions
		Text string `json:"text"`
	}
	if !bindJSON(c, &req) || !req.normalize(c) {
		return
	}
	if req.Text == "" {
		badRequest(c, "text is required")
		return
	}
	inst := getInstance(c)
	res, err := h.SendService.SendText(&send_service.TextStruct{
		Number:       req.Number,
		Text:         req.Text,
		Delay:        req.Delay,
		MentionedJID: req.mentioned(),
		MentionAll:   req.MentionsEveryOne,
		Quoted:       req.quotedStruct(),
	}, inst)
	sent(c, inst, res, err)
}

// ---------------------------------------------------------------------------
// POST /message/sendMedia/:instance  (image | video | document | audio)
// POST /message/sendPtv/:instance    (round video note)
// POST /message/sendWhatsAppAudio/:instance (voice note / PTT)
// ---------------------------------------------------------------------------

func (h *Handler) sendMedia(c *gin.Context, inst *instance_model.Instance, opts sendOptions, mediaType, media, caption, fileName string) {
	if media == "" {
		badRequest(c, "media is required")
		return
	}
	data := &send_service.MediaStruct{
		Number:       opts.Number,
		Type:         mediaType,
		Caption:      caption,
		Filename:     fileName,
		Delay:        opts.Delay,
		MentionedJID: opts.mentioned(),
		MentionAll:   opts.MentionsEveryOne,
		Quoted:       opts.quotedStruct(),
	}
	if isURL(media) {
		data.Url = media
		res, err := h.SendService.SendMediaUrl(data, inst)
		sent(c, inst, res, err)
		return
	}
	file, err := decodeMedia(media)
	if err != nil {
		badRequest(c, "media must be a URL or a base64 string")
		return
	}
	res, err := h.SendService.SendMediaFile(data, file, inst)
	sent(c, inst, res, err)
}

func (h *Handler) SendMedia(c *gin.Context) {
	var req struct {
		sendOptions
		Mediatype string `json:"mediatype"`
		Mimetype  string `json:"mimetype"`
		Caption   string `json:"caption"`
		Media     string `json:"media"`
		FileName  string `json:"fileName"`
	}
	if !bindJSON(c, &req) || !req.normalize(c) {
		return
	}
	mt := strings.ToLower(req.Mediatype)
	switch mt {
	case "image", "video", "document", "audio", "ptv":
	default:
		badRequest(c, "mediatype must be one of image, video, document, audio")
		return
	}
	h.sendMedia(c, getInstance(c), req.sendOptions, mt, req.Media, req.Caption, req.FileName)
}

func (h *Handler) SendPtv(c *gin.Context) {
	var req struct {
		sendOptions
		Video string `json:"video"`
	}
	if !bindJSON(c, &req) || !req.normalize(c) {
		return
	}
	h.sendMedia(c, getInstance(c), req.sendOptions, "ptv", req.Video, "", "")
}

func (h *Handler) SendWhatsAppAudio(c *gin.Context) {
	var req struct {
		sendOptions
		Audio string `json:"audio"`
	}
	if !bindJSON(c, &req) || !req.normalize(c) {
		return
	}
	// The engine always sends "audio" as a PTT voice note, matching
	// Evolution API's sendWhatsAppAudio.
	h.sendMedia(c, getInstance(c), req.sendOptions, "audio", req.Audio, "", "")
}

// ---------------------------------------------------------------------------
// POST /message/sendSticker/:instance
// ---------------------------------------------------------------------------

func (h *Handler) SendSticker(c *gin.Context) {
	var req struct {
		sendOptions
		Sticker string `json:"sticker"`
	}
	if !bindJSON(c, &req) || !req.normalize(c) {
		return
	}
	if req.Sticker == "" {
		badRequest(c, "sticker is required")
		return
	}
	inst := getInstance(c)
	res, err := h.SendService.SendSticker(&send_service.StickerStruct{
		Number:       req.Number,
		Sticker:      req.Sticker,
		Delay:        req.Delay,
		MentionedJID: req.mentioned(),
		MentionAll:   req.MentionsEveryOne,
		Quoted:       req.quotedStruct(),
	}, inst)
	sent(c, inst, res, err)
}

// ---------------------------------------------------------------------------
// POST /message/sendLocation/:instance
// ---------------------------------------------------------------------------

func (h *Handler) SendLocation(c *gin.Context) {
	var req struct {
		sendOptions
		Name      string  `json:"name"`
		Address   string  `json:"address"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	}
	if !bindJSON(c, &req) || !req.normalize(c) {
		return
	}
	inst := getInstance(c)
	res, err := h.SendService.SendLocation(&send_service.LocationStruct{
		Number:       req.Number,
		Name:         req.Name,
		Address:      req.Address,
		Latitude:     req.Latitude,
		Longitude:    req.Longitude,
		Delay:        req.Delay,
		MentionedJID: req.mentioned(),
		MentionAll:   req.MentionsEveryOne,
		Quoted:       req.quotedStruct(),
	}, inst)
	sent(c, inst, res, err)
}

// ---------------------------------------------------------------------------
// POST /message/sendContact/:instance
// ---------------------------------------------------------------------------

type contactCard struct {
	FullName     string `json:"fullName"`
	Wuid         string `json:"wuid"`
	PhoneNumber  string `json:"phoneNumber"`
	Organization string `json:"organization"`
	Email        string `json:"email"`
	URL          string `json:"url"`
}

func (cc contactCard) vcard() string {
	phone := cc.PhoneNumber
	wuid := cc.Wuid
	if wuid == "" {
		wuid = onlyDigits(phone)
	}
	var b strings.Builder
	b.WriteString("BEGIN:VCARD\nVERSION:3.0\n")
	fmt.Fprintf(&b, "N:%s\nFN:%s\n", cc.FullName, cc.FullName)
	if cc.Organization != "" {
		fmt.Fprintf(&b, "ORG:%s;\n", cc.Organization)
	}
	if cc.Email != "" {
		fmt.Fprintf(&b, "EMAIL:%s\n", cc.Email)
	}
	if cc.URL != "" {
		fmt.Fprintf(&b, "URL:%s\n", cc.URL)
	}
	fmt.Fprintf(&b, "TEL;type=CELL;type=VOICE;waid=%s:%s\n", wuid, phone)
	b.WriteString("END:VCARD")
	return b.String()
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (h *Handler) SendContact(c *gin.Context) {
	var req struct {
		sendOptions
		Contact []contactCard `json:"contact"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if len(req.Contact) == 0 {
		badRequest(c, "contact is required")
		return
	}
	cli, inst, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	to, err := parseJID(req.Number)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	ctxInfo := h.contextInfo(req.sendOptions)
	var msg *waE2E.Message
	if len(req.Contact) == 1 {
		msg = &waE2E.Message{ContactMessage: &waE2E.ContactMessage{
			DisplayName: proto.String(req.Contact[0].FullName),
			Vcard:       proto.String(req.Contact[0].vcard()),
			ContextInfo: ctxInfo,
		}}
	} else {
		cards := make([]*waE2E.ContactMessage, 0, len(req.Contact))
		for _, cc := range req.Contact {
			cards = append(cards, &waE2E.ContactMessage{
				DisplayName: proto.String(cc.FullName),
				Vcard:       proto.String(cc.vcard()),
			})
		}
		msg = &waE2E.Message{ContactsArrayMessage: &waE2E.ContactsArrayMessage{
			DisplayName: proto.String(fmt.Sprintf("%d contacts", len(cards))),
			Contacts:    cards,
			ContextInfo: ctxInfo,
		}}
	}
	h.sendRaw(c, cli, inst, to, msg, req.Delay)
}

// contextInfo builds quote/mention metadata for messages sent directly.
func (h *Handler) contextInfo(o sendOptions) *waE2E.ContextInfo {
	ci := &waE2E.ContextInfo{}
	used := false
	if m := o.mentioned(); len(m) > 0 {
		for _, j := range m {
			ci.MentionedJID = append(ci.MentionedJID, strings.TrimPrefix(j, "+"))
		}
		used = true
	}
	if o.Quoted != nil && o.Quoted.Key.ID != "" {
		ci.StanzaID = proto.String(o.Quoted.Key.ID)
		if o.Quoted.Key.Participant != "" {
			ci.Participant = proto.String(o.Quoted.Key.Participant)
		} else if o.Quoted.Key.RemoteJid != "" {
			ci.Participant = proto.String(o.Quoted.Key.RemoteJid)
		}
		if len(o.Quoted.Message) > 0 {
			qm := &waE2E.Message{}
			if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(o.Quoted.Message, qm); err == nil {
				ci.QuotedMessage = qm
			}
		}
		if ci.QuotedMessage == nil {
			ci.QuotedMessage = &waE2E.Message{Conversation: proto.String("")}
		}
		used = true
	}
	if !used {
		return nil
	}
	return ci
}

// simulateTyping shows "composing"/"recording" for delay ms, as Evolution API does.
func simulateTyping(cli *whatsmeow.Client, to types.JID, delay int32, media types.ChatPresenceMedia) {
	if delay <= 0 {
		return
	}
	_ = cli.SendChatPresence(ctx(), to, types.ChatPresenceComposing, media)
	time.Sleep(time.Duration(delay) * time.Millisecond)
	_ = cli.SendChatPresence(ctx(), to, types.ChatPresencePaused, media)
}

func (h *Handler) sendRaw(c *gin.Context, cli *whatsmeow.Client, inst *instance_model.Instance, to types.JID, msg *waE2E.Message, delay int32) {
	simulateTyping(cli, to, delay, types.ChatPresenceMediaText)
	resp, err := cli.SendMessage(ctx(), to, msg)
	if err != nil {
		internalError(c, err)
		return
	}
	info := types.MessageInfo{
		MessageSource: types.MessageSource{Chat: to, IsFromMe: true},
		ID:            resp.ID,
		Timestamp:     resp.Timestamp,
		PushName:      cli.Store.PushName,
	}
	c.JSON(http.StatusCreated, messageResponse(inst, info, msg))
}

// ---------------------------------------------------------------------------
// POST /message/sendReaction/:instance
// ---------------------------------------------------------------------------

func (h *Handler) SendReaction(c *gin.Context) {
	var req struct {
		Key struct {
			RemoteJid   string `json:"remoteJid"`
			FromMe      bool   `json:"fromMe"`
			ID          string `json:"id"`
			Participant string `json:"participant"`
		} `json:"key"`
		Reaction string `json:"reaction"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.Key.ID == "" || req.Key.RemoteJid == "" {
		badRequest(c, "key.id and key.remoteJid are required")
		return
	}
	cli, inst, ok := h.clientOrFail(c)
	if !ok {
		return
	}
	chat, err := parseJID(req.Key.RemoteJid)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	sender := chat
	if req.Key.FromMe {
		sender = cli.Store.ID.ToNonAD()
	} else if req.Key.Participant != "" {
		if p, err := parseJID(req.Key.Participant); err == nil {
			sender = p
		}
	}
	msg := cli.BuildReaction(chat, sender, req.Key.ID, req.Reaction)
	h.sendRaw(c, cli, inst, chat, msg, 0)
}

// ---------------------------------------------------------------------------
// POST /message/sendPoll/:instance
// ---------------------------------------------------------------------------

func (h *Handler) SendPoll(c *gin.Context) {
	var req struct {
		sendOptions
		Name            string   `json:"name"`
		SelectableCount int      `json:"selectableCount"`
		Values          []string `json:"values"`
	}
	if !bindJSON(c, &req) || !req.normalize(c) {
		return
	}
	if req.Name == "" || len(req.Values) < 2 {
		badRequest(c, "name and at least two values are required")
		return
	}
	inst := getInstance(c)
	res, err := h.SendService.SendPoll(&send_service.PollStruct{
		Number:       req.Number,
		Question:     req.Name,
		MaxAnswer:    req.SelectableCount,
		Options:      req.Values,
		Delay:        req.Delay,
		MentionedJID: req.mentioned(),
		MentionAll:   req.MentionsEveryOne,
		Quoted:       req.quotedStruct(),
	}, inst)
	sent(c, inst, res, err)
}

// ---------------------------------------------------------------------------
// POST /message/sendList/:instance
// ---------------------------------------------------------------------------

func (h *Handler) SendList(c *gin.Context) {
	var req struct {
		sendOptions
		Title       string                 `json:"title"`
		Description string                 `json:"description"`
		ButtonText  string                 `json:"buttonText"`
		FooterText  string                 `json:"footerText"`
		Sections    []send_service.Section `json:"sections"`
	}
	if !bindJSON(c, &req) || !req.normalize(c) {
		return
	}
	if len(req.Sections) == 0 {
		badRequest(c, "sections is required")
		return
	}
	inst := getInstance(c)
	res, err := h.SendService.SendList(&send_service.ListStruct{
		Number:       req.Number,
		Title:        req.Title,
		Description:  req.Description,
		ButtonText:   req.ButtonText,
		FooterText:   req.FooterText,
		Sections:     req.Sections,
		Delay:        req.Delay,
		MentionedJID: req.mentioned(),
		MentionAll:   req.MentionsEveryOne,
		Quoted:       req.quotedStruct(),
	}, inst)
	sent(c, inst, res, err)
}

// ---------------------------------------------------------------------------
// POST /message/sendButtons/:instance
// ---------------------------------------------------------------------------

func (h *Handler) SendButtons(c *gin.Context) {
	var req struct {
		sendOptions
		Title       string                `json:"title"`
		Description string                `json:"description"`
		Footer      string                `json:"footer"`
		Thumbnail   string                `json:"thumbnail"`
		Buttons     []send_service.Button `json:"buttons"`
	}
	if !bindJSON(c, &req) || !req.normalize(c) {
		return
	}
	if len(req.Buttons) == 0 {
		badRequest(c, "buttons is required")
		return
	}
	for i := range req.Buttons {
		req.Buttons[i].Type = strings.ToLower(req.Buttons[i].Type)
	}
	inst := getInstance(c)
	data := &send_service.ButtonStruct{
		Number:       req.Number,
		Title:        req.Title,
		Description:  req.Description,
		Footer:       req.Footer,
		Buttons:      req.Buttons,
		Delay:        req.Delay,
		MentionedJID: req.mentioned(),
		MentionAll:   req.MentionsEveryOne,
		Quoted:       req.quotedStruct(),
	}
	if isURL(req.Thumbnail) {
		data.ImageUrl = req.Thumbnail
	}
	res, err := h.SendService.SendButton(data, inst)
	sent(c, inst, res, err)
}

// ---------------------------------------------------------------------------
// POST /message/sendStatus/:instance
// ---------------------------------------------------------------------------

func (h *Handler) SendStatus(c *gin.Context) {
	var req struct {
		Type            string   `json:"type"`
		Content         string   `json:"content"`
		Caption         string   `json:"caption"`
		BackgroundColor string   `json:"backgroundColor"`
		Font            int      `json:"font"`
		AllContacts     bool     `json:"allContacts"`
		StatusJidList   []string `json:"statusJidList"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.Content == "" {
		badRequest(c, "content is required")
		return
	}
	inst := getInstance(c)
	switch strings.ToLower(req.Type) {
	case "", "text":
		res, err := h.SendService.SendStatusText(&send_service.StatusTextStruct{Text: req.Content}, inst)
		sent(c, inst, res, err)
	case "image", "video", "audio":
		data := &send_service.StatusMediaStruct{Type: strings.ToLower(req.Type), Caption: req.Caption}
		if isURL(req.Content) {
			data.Url = req.Content
			res, err := h.SendService.SendStatusMediaUrl(data, inst)
			sent(c, inst, res, err)
			return
		}
		file, err := decodeMedia(req.Content)
		if err != nil {
			badRequest(c, "content must be a URL or a base64 string")
			return
		}
		res, err := h.SendService.SendStatusMediaFile(data, file, inst)
		sent(c, inst, res, err)
	default:
		badRequest(c, "type must be one of text, image, video, audio")
	}
}
