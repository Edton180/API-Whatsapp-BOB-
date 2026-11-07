// API WhatsApp BOB - Desenvolvido por Edton Braz.

package api

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"

	"github.com/edton180/api-whatsapp-bob/internal/engine"
	"github.com/edton180/api-whatsapp-bob/internal/media"
)

// base são os campos comuns de envio.
type base struct {
	Number string `json:"number"`
	engine.SendOptions
}

func (s *Server) deliver(c *gin.Context, number string, opt engine.SendOptions, msg *waE2E.Message, err error) {
	if err != nil {
		failEngine(c, err)
		return
	}
	sess := s.session(c)
	to, err := sess.ResolveJID(c.Request.Context(), number)
	if err != nil {
		failEngine(c, err)
		return
	}
	res, err := sess.Send(c.Request.Context(), to, msg, opt)
	if err != nil {
		failEngine(c, err)
		return
	}
	ok(c, res)
}

func (s *Server) sendText(c *gin.Context) {
	var req struct {
		base
		Text string `json:"text"`
	}
	if !bind(c, &req) {
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		fail(c, http.StatusBadRequest, errors.New("informe o text"))
		return
	}
	s.deliver(c, req.Number, req.SendOptions, engine.Text(req.Text), nil)
}

func (s *Server) sendLink(c *gin.Context) {
	var req struct {
		base
		Text        string `json:"text"`
		URL         string `json:"url"`
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if !bind(c, &req) {
		return
	}
	if req.URL == "" {
		fail(c, http.StatusBadRequest, errors.New("informe a url"))
		return
	}
	s.deliver(c, req.Number, req.SendOptions, engine.Link(req.Text, req.URL, req.Title, req.Description), nil)
}

func (s *Server) sendMedia(c *gin.Context) {
	var req struct {
		base
		URL      string `json:"url"`
		Base64   string `json:"base64"`
		Media    string `json:"media"`
		Type     string `json:"type"`
		Caption  string `json:"caption"`
		Filename string `json:"filename"`
		Mimetype string `json:"mimetype"`
	}
	var data []byte
	var mime string
	if strings.HasPrefix(c.ContentType(), "multipart/") {
		req.Number = c.PostForm("number")
		req.Type, req.Caption, req.Filename = c.PostForm("type"), c.PostForm("caption"), c.PostForm("filename")
		req.ID = c.PostForm("id")
		req.Delay, _ = strconv.Atoi(c.PostForm("delay"))
		if q := c.PostForm("quoted.messageId"); q != "" {
			req.Quoted = &engine.Quoted{MessageID: q, Participant: c.PostForm("quoted.participant")}
		}
		fh, err := c.FormFile("file")
		if err != nil {
			fail(c, http.StatusBadRequest, errors.New("envie o arquivo no campo file"))
			return
		}
		f, err := fh.Open()
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		data, err = io.ReadAll(io.LimitReader(f, media.MaxDownload))
		f.Close()
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		if req.Filename == "" {
			req.Filename = fh.Filename
		}
		mime = media.Detect(data)
	} else {
		if !bind(c, &req) {
			return
		}
		src := req.URL
		if src == "" {
			src = req.Base64
		}
		if src == "" {
			src = req.Media
		}
		var err error
		if data, mime, err = media.Load(c.Request.Context(), src); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		if req.Mimetype != "" {
			mime = req.Mimetype
		}
	}
	msg, err := s.session(c).Media(c.Request.Context(), engine.MediaInput{
		Data: data, Type: req.Type, Mime: mime, Caption: req.Caption, Filename: req.Filename,
	})
	s.deliver(c, req.Number, req.SendOptions, msg, err)
}

func (s *Server) sendSticker(c *gin.Context) {
	var req struct {
		base
		Sticker string `json:"sticker"`
		URL     string `json:"url"`
	}
	if !bind(c, &req) {
		return
	}
	src := req.Sticker
	if src == "" {
		src = req.URL
	}
	data, _, err := media.Load(c.Request.Context(), src)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	msg, err := s.session(c).Sticker(c.Request.Context(), data)
	s.deliver(c, req.Number, req.SendOptions, msg, err)
}

func (s *Server) sendLocation(c *gin.Context) {
	var req struct {
		base
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Name      string  `json:"name"`
		Address   string  `json:"address"`
	}
	if !bind(c, &req) {
		return
	}
	s.deliver(c, req.Number, req.SendOptions, engine.Location(req.Latitude, req.Longitude, req.Name, req.Address), nil)
}

func (s *Server) sendContact(c *gin.Context) {
	var req struct {
		base
		Vcard engine.VCard `json:"vcard"`
	}
	if !bind(c, &req) {
		return
	}
	if req.Vcard.Phone == "" {
		fail(c, http.StatusBadRequest, errors.New("informe vcard.phone"))
		return
	}
	if req.Vcard.FullName == "" {
		req.Vcard.FullName = req.Vcard.Phone
	}
	s.deliver(c, req.Number, req.SendOptions, engine.Contact(req.Vcard), nil)
}

func (s *Server) sendPoll(c *gin.Context) {
	var req struct {
		base
		Question  string   `json:"question"`
		Options   []string `json:"options"`
		MaxAnswer int      `json:"maxAnswer"`
	}
	if !bind(c, &req) {
		return
	}
	msg, err := s.session(c).Poll(req.Question, req.Options, req.MaxAnswer)
	s.deliver(c, req.Number, req.SendOptions, msg, err)
}

func (s *Server) sendButton(c *gin.Context) {
	var req struct {
		base
		Title       string          `json:"title"`
		Description string          `json:"description"`
		Footer      string          `json:"footer"`
		Buttons     []engine.Button `json:"buttons"`
	}
	if !bind(c, &req) {
		return
	}
	msg, nodes, err := engine.Buttons(req.Title, req.Description, req.Footer, req.Buttons)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	sess := s.session(c)
	to, err := sess.ResolveJID(c.Request.Context(), req.Number)
	if err != nil {
		failEngine(c, err)
		return
	}
	res, err := sess.SendWithNodes(c.Request.Context(), to, msg, nodes, req.SendOptions)
	if err != nil {
		failEngine(c, err)
		return
	}
	ok(c, res)
}

func (s *Server) sendList(c *gin.Context) {
	var req struct {
		base
		Title       string               `json:"title"`
		Description string               `json:"description"`
		ButtonText  string               `json:"buttonText"`
		FooterText  string               `json:"footerText"`
		Sections    []engine.ListSection `json:"sections"`
	}
	if !bind(c, &req) {
		return
	}
	msg, err := engine.List(req.Title, req.Description, req.ButtonText, req.FooterText, req.Sections)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	s.deliver(c, req.Number, req.SendOptions, msg, nil)
}

func (s *Server) sendStatusText(c *gin.Context) {
	var req struct {
		Text            string `json:"text"`
		BackgroundColor uint32 `json:"backgroundColor"`
		Font            int32  `json:"font"`
	}
	if !bind(c, &req) {
		return
	}
	res, err := s.session(c).Send(c.Request.Context(), types.StatusBroadcastJID, engine.StatusText(req.Text, req.BackgroundColor, req.Font), engine.SendOptions{})
	if err != nil {
		failEngine(c, err)
		return
	}
	ok(c, res)
}

func (s *Server) sendStatusMedia(c *gin.Context) {
	var req struct {
		URL     string `json:"url"`
		Type    string `json:"type"`
		Caption string `json:"caption"`
	}
	if !bind(c, &req) {
		return
	}
	data, mime, err := media.Load(c.Request.Context(), req.URL)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	sess := s.session(c)
	msg, err := sess.Media(c.Request.Context(), engine.MediaInput{Data: data, Mime: mime, Type: req.Type, Caption: req.Caption})
	if err != nil {
		failEngine(c, err)
		return
	}
	res, err := sess.Send(c.Request.Context(), types.StatusBroadcastJID, msg, engine.SendOptions{})
	if err != nil {
		failEngine(c, err)
		return
	}
	ok(c, res)
}
