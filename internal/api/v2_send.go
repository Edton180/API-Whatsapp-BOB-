// API WhatsApp BOB - Desenvolvido por Edton Braz.

package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	"github.com/edton180/api-whatsapp-bob/internal/engine"
	"github.com/edton180/api-whatsapp-bob/internal/media"
)

// v2Options são os campos comuns de envio no padrão v2.
type v2Options struct {
	Number           string   `json:"number"`
	Delay            int      `json:"delay"`
	LinkPreview      *bool    `json:"linkPreview"`
	MentionsEveryOne bool     `json:"mentionsEveryOne"`
	Mentioned        []string `json:"mentioned"`
	Quoted           *struct {
		Key struct {
			ID          string `json:"id"`
			RemoteJid   string `json:"remoteJid"`
			FromMe      bool   `json:"fromMe"`
			Participant string `json:"participant"`
		} `json:"key"`
	} `json:"quoted"`
}

func (o v2Options) send() engine.SendOptions {
	opt := engine.SendOptions{Delay: o.Delay, MentionAll: o.MentionsEveryOne}
	for _, m := range o.Mentioned {
		if d := strings.TrimPrefix(strings.SplitN(m, "@", 2)[0], "+"); d != "" {
			opt.MentionedJID = append(opt.MentionedJID, d+"@s.whatsapp.net")
		}
	}
	if o.Quoted != nil && o.Quoted.Key.ID != "" {
		p := o.Quoted.Key.Participant
		if p == "" && !o.Quoted.Key.FromMe {
			p = o.Quoted.Key.RemoteJid
		}
		opt.Quoted = &engine.Quoted{MessageID: o.Quoted.Key.ID, Participant: p}
	}
	return opt
}

// v2Deliver resolve o destino, envia e responde no padrão v2.
func (s *Server) v2Deliver(c *gin.Context, o v2Options, msg *waE2E.Message, err error) {
	if err != nil {
		v2Engine(c, err)
		return
	}
	sess := s.session(c)
	to, err := sess.ResolveJID(c.Request.Context(), o.Number)
	if err != nil {
		v2Engine(c, err)
		return
	}
	res, err := sess.Send(c.Request.Context(), to, msg, o.send())
	if err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusCreated, v2Message(current(c), res))
}

func (s *Server) v2SendText(c *gin.Context) {
	var req struct {
		v2Options
		Text string `json:"text"`
	}
	if !v2Bind(c, &req) {
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		v2Fail(c, http.StatusBadRequest, "text is required")
		return
	}
	s.v2Deliver(c, req.v2Options, engine.Text(req.Text), nil)
}

func (s *Server) v2Media(c *gin.Context, o v2Options, typ, src, caption, fileName, mime string) {
	if strings.TrimSpace(src) == "" {
		v2Fail(c, http.StatusBadRequest, "media is required (URL or base64)")
		return
	}
	data, detected, err := media.Load(c.Request.Context(), src)
	if err != nil {
		v2Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if mime == "" {
		mime = detected
	}
	msg, err := s.session(c).Media(c.Request.Context(), engine.MediaInput{
		Data: data, Type: typ, Mime: mime, Caption: caption, Filename: fileName,
	})
	s.v2Deliver(c, o, msg, err)
}

func (s *Server) v2SendMedia(c *gin.Context) {
	var req struct {
		v2Options
		Mediatype string `json:"mediatype"`
		Mimetype  string `json:"mimetype"`
		Caption   string `json:"caption"`
		Media     string `json:"media"`
		FileName  string `json:"fileName"`
	}
	if !v2Bind(c, &req) {
		return
	}
	typ := strings.ToLower(req.Mediatype)
	switch typ {
	case "image", "video", "document", "audio", "ptv":
	default:
		v2Fail(c, http.StatusBadRequest, "mediatype must be image, video, document or audio")
		return
	}
	if typ == "audio" {
		typ = "audio-file"
	}
	s.v2Media(c, req.v2Options, typ, req.Media, req.Caption, req.FileName, req.Mimetype)
}

func (s *Server) v2SendPtv(c *gin.Context) {
	var req struct {
		v2Options
		Video string `json:"video"`
	}
	if !v2Bind(c, &req) {
		return
	}
	s.v2Media(c, req.v2Options, "ptv", req.Video, "", "", "")
}

func (s *Server) v2SendAudio(c *gin.Context) {
	var req struct {
		v2Options
		Audio string `json:"audio"`
	}
	if !v2Bind(c, &req) {
		return
	}
	s.v2Media(c, req.v2Options, "ptt", req.Audio, "", "", "")
}

func (s *Server) v2SendSticker(c *gin.Context) {
	var req struct {
		v2Options
		Sticker string `json:"sticker"`
	}
	if !v2Bind(c, &req) {
		return
	}
	data, _, err := media.Load(c.Request.Context(), req.Sticker)
	if err != nil {
		v2Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	msg, err := s.session(c).Sticker(c.Request.Context(), data)
	s.v2Deliver(c, req.v2Options, msg, err)
}

func (s *Server) v2SendLocation(c *gin.Context) {
	var req struct {
		v2Options
		Name      string  `json:"name"`
		Address   string  `json:"address"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	}
	if !v2Bind(c, &req) {
		return
	}
	s.v2Deliver(c, req.v2Options, engine.Location(req.Latitude, req.Longitude, req.Name, req.Address), nil)
}

func (s *Server) v2SendContact(c *gin.Context) {
	var req struct {
		v2Options
		Contact []struct {
			FullName     string `json:"fullName"`
			Wuid         string `json:"wuid"`
			PhoneNumber  string `json:"phoneNumber"`
			Organization string `json:"organization"`
			Email        string `json:"email"`
			URL          string `json:"url"`
		} `json:"contact"`
	}
	if !v2Bind(c, &req) {
		return
	}
	if len(req.Contact) == 0 {
		v2Fail(c, http.StatusBadRequest, "contact is required")
		return
	}
	cards := make([]*waE2E.ContactMessage, 0, len(req.Contact))
	for _, ct := range req.Contact {
		phone := ct.PhoneNumber
		if phone == "" {
			phone = ct.Wuid
		}
		name := ct.FullName
		if name == "" {
			name = phone
		}
		cards = append(cards, engine.Contact(engine.VCard{
			FullName: name, Phone: phone, Organization: ct.Organization, Email: ct.Email, URL: ct.URL,
		}).ContactMessage)
	}
	msg := &waE2E.Message{ContactMessage: cards[0]}
	if len(cards) > 1 {
		msg = &waE2E.Message{ContactsArrayMessage: &waE2E.ContactsArrayMessage{
			DisplayName: proto.String(fmt.Sprintf("%d contatos", len(cards))),
			Contacts:    cards,
		}}
	}
	s.v2Deliver(c, req.v2Options, msg, nil)
}

func (s *Server) v2SendReaction(c *gin.Context) {
	var req struct {
		Key struct {
			RemoteJid   string `json:"remoteJid"`
			FromMe      bool   `json:"fromMe"`
			ID          string `json:"id"`
			Participant string `json:"participant"`
		} `json:"key"`
		Reaction string `json:"reaction"`
	}
	if !v2Bind(c, &req) {
		return
	}
	if req.Key.ID == "" || req.Key.RemoteJid == "" {
		v2Fail(c, http.StatusBadRequest, "key.id and key.remoteJid are required")
		return
	}
	res, err := s.session(c).React(c.Request.Context(), req.Key.RemoteJid, req.Key.ID, req.Reaction, req.Key.FromMe, req.Key.Participant)
	if err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusCreated, v2Message(current(c), res))
}

func (s *Server) v2SendPoll(c *gin.Context) {
	var req struct {
		v2Options
		Name            string   `json:"name"`
		SelectableCount int      `json:"selectableCount"`
		Values          []string `json:"values"`
	}
	if !v2Bind(c, &req) {
		return
	}
	msg, err := s.session(c).Poll(req.Name, req.Values, req.SelectableCount)
	s.v2Deliver(c, req.v2Options, msg, err)
}

func (s *Server) v2SendList(c *gin.Context) {
	var req struct {
		v2Options
		Title       string               `json:"title"`
		Description string               `json:"description"`
		ButtonText  string               `json:"buttonText"`
		FooterText  string               `json:"footerText"`
		Sections    []engine.ListSection `json:"sections"`
	}
	if !v2Bind(c, &req) {
		return
	}
	msg, err := engine.List(req.Title, req.Description, req.ButtonText, req.FooterText, req.Sections)
	s.v2Deliver(c, req.v2Options, msg, err)
}

func (s *Server) v2SendButtons(c *gin.Context) {
	var req struct {
		v2Options
		Title       string          `json:"title"`
		Description string          `json:"description"`
		Footer      string          `json:"footer"`
		Buttons     []engine.Button `json:"buttons"`
	}
	if !v2Bind(c, &req) {
		return
	}
	msg, nodes, err := engine.Buttons(req.Title, req.Description, req.Footer, req.Buttons)
	if err != nil {
		v2Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	sess := s.session(c)
	to, err := sess.ResolveJID(c.Request.Context(), req.Number)
	if err != nil {
		v2Engine(c, err)
		return
	}
	res, err := sess.SendWithNodes(c.Request.Context(), to, msg, nodes, req.send())
	if err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusCreated, v2Message(current(c), res))
}

// v2SendStatus publica um status (texto, imagem, vídeo ou áudio).
func (s *Server) v2SendStatus(c *gin.Context) {
	var req struct {
		Type            string `json:"type"`
		Content         string `json:"content"`
		Caption         string `json:"caption"`
		BackgroundColor string `json:"backgroundColor"`
		Font            int32  `json:"font"`
	}
	if !v2Bind(c, &req) {
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		v2Fail(c, http.StatusBadRequest, "content is required")
		return
	}
	sess := s.session(c)
	var msg *waE2E.Message
	var err error
	switch strings.ToLower(req.Type) {
	case "", "text":
		msg = engine.StatusText(req.Content, parseColor(req.BackgroundColor), req.Font)
	case "image", "video", "audio":
		data, mime, lerr := media.Load(c.Request.Context(), req.Content)
		if lerr != nil {
			v2Fail(c, http.StatusBadRequest, lerr.Error())
			return
		}
		typ := strings.ToLower(req.Type)
		if typ == "audio" {
			typ = "ptt"
		}
		msg, err = sess.Media(c.Request.Context(), engine.MediaInput{Data: data, Mime: mime, Type: typ, Caption: req.Caption})
	default:
		v2Fail(c, http.StatusBadRequest, "type must be text, image, video or audio")
		return
	}
	if err != nil {
		v2Engine(c, err)
		return
	}
	res, err := sess.Send(c.Request.Context(), types.StatusBroadcastJID, msg, engine.SendOptions{})
	if err != nil {
		v2Engine(c, err)
		return
	}
	c.JSON(http.StatusCreated, v2Message(current(c), res))
}

// parseColor aceita "#RRGGBB" ou "#AARRGGBB" e devolve ARGB (0 = cor padrão).
func parseColor(v string) uint32 {
	v = strings.TrimPrefix(strings.TrimSpace(v), "#")
	var n uint32
	if _, err := fmt.Sscanf(v, "%x", &n); err != nil {
		return 0
	}
	if len(v) == 6 {
		n |= 0xFF000000
	}
	return n
}
