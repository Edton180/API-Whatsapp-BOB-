// API WhatsApp BOB - Desenvolvido por Edton Braz.

package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.mau.fi/whatsmeow/types"

	"github.com/edton180/api-whatsapp-bob/internal/media"

	"github.com/edton180/api-whatsapp-bob/internal/engine"
)

// registerTools monta as ferramentas de extração (/tools/*) e as tarefas em massa (/bulk/*).
func (s *Server) registerTools(r *gin.Engine, inst gin.HandlerFunc) {
	tg := r.Group("/tools", inst)
	tg.POST("/group/members", s.toolsGroupMembers)
	tg.POST("/group/active", s.toolsGroupActive)
	tg.GET("/chat/contacts", s.toolsChatContacts)
	tg.POST("/number/filter", s.toolsNumberFilter)
	tg.POST("/invite/check", s.toolsInviteCheck)
	tg.POST("/group/add", s.toolsGroupAdd)

	bg := r.Group("/bulk", inst)
	bg.POST("/group/add", s.bulkGroupAdd)
	bg.POST("/send", s.bulkSend)
	bg.POST("/group/create", s.bulkGroupCreate)
	bg.GET("/list", s.bulkList)
	bg.GET("/:id", s.bulkGet)
	bg.POST("/:id/:action", s.bulkControl)
}

// POST /tools/group/members — extrai os participantes dos grupos (sem repetir quem está em mais
// de um). Sem groups, lê todos os grupos da instância.
func (s *Server) toolsGroupMembers(c *gin.Context) {
	var req struct {
		Groups        stringList `json:"groups"`
		OnlyAdmins    bool       `json:"onlyAdmins"`
		ExcludeAdmins bool       `json:"excludeAdmins"`
	}
	if !bindOptional(c, &req) {
		return
	}
	res, err := s.session(c).GroupMembers(c.Request.Context(), req.Groups,
		engine.MembersOptions{OnlyAdmins: req.OnlyAdmins, ExcludeAdmins: req.ExcludeAdmins})
	reply(c, res, err)
}

// POST /tools/group/active — quem escreveu no grupo nos últimos dias.
func (s *Server) toolsGroupActive(c *gin.Context) {
	var req struct {
		GroupJid string `json:"groupJid"`
		Days     int    `json:"days"`
	}
	if !bind(c, &req) {
		return
	}
	res, err := s.session(c).ActiveMembers(c.Request.Context(), req.GroupJid, req.Days)
	reply(c, gin.H{"members": res}, err)
}

// GET /tools/chat/contacts — pessoas com quem a instância tem conversa.
func (s *Server) toolsChatContacts(c *gin.Context) {
	res, err := s.session(c).ChatContacts()
	reply(c, gin.H{"contacts": res}, err)
}

// POST /tools/number/filter — separa os números que têm WhatsApp dos que não têm.
func (s *Server) toolsNumberFilter(c *gin.Context) {
	var req struct {
		Numbers stringList `json:"numbers"`
	}
	if !bind(c, &req) {
		return
	}
	res, err := s.session(c).FilterNumbers(c.Request.Context(), req.Numbers)
	reply(c, res, err)
}

// POST /tools/invite/check — confere links de convite sem entrar nos grupos.
func (s *Server) toolsInviteCheck(c *gin.Context) {
	var req struct {
		Links stringList `json:"links"`
	}
	if !bind(c, &req) {
		return
	}
	res, err := s.session(c).CheckInvites(c.Request.Context(), req.Links)
	reply(c, gin.H{"links": res}, err)
}

// POST /tools/group/add — adiciona uma pessoa ao grupo e diz o que aconteceu com ela.
func (s *Server) toolsGroupAdd(c *gin.Context) {
	var req struct {
		GroupJid       string `json:"groupJid"`
		Number         string `json:"number"`
		InviteFallback bool   `json:"inviteFallback"`
		InviteText     string `json:"inviteText"`
	}
	if !bind(c, &req) {
		return
	}
	res, err := s.session(c).AddParticipant(c.Request.Context(), req.GroupJid, req.Number,
		engine.AddOptions{InviteFallback: req.InviteFallback, InviteText: req.InviteText})
	reply(c, res, err)
}

// ---------------------------------------------------------------- tarefas em massa

// POST /bulk/group/add — adiciona uma lista de números a um grupo, um por vez.
func (s *Server) bulkGroupAdd(c *gin.Context) {
	var req struct {
		GroupJid       string     `json:"groupJid"`
		Numbers        stringList `json:"numbers"`
		IntervalSecs   int        `json:"intervalSeconds"`
		InviteFallback bool       `json:"inviteFallback"`
		InviteText     string     `json:"inviteText"`
		Label          string     `json:"label"`
	}
	if !bind(c, &req) {
		return
	}
	if strings.TrimSpace(req.GroupJid) == "" {
		fail(c, http.StatusBadRequest, errors.New("informe o groupJid"))
		return
	}
	sess := s.session(c)
	opt := engine.AddOptions{InviteFallback: req.InviteFallback, InviteText: req.InviteText}
	job, err := s.Engine.Bulk().Start(current(c).Id, engine.BulkGroupAdd, req.Label, req.Numbers, req.IntervalSecs,
		func(ctx context.Context, target string) (string, error) {
			res, err := sess.AddParticipant(ctx, req.GroupJid, target, opt)
			if err != nil {
				return "", err
			}
			if res.Status == "added" || res.Status == "member" || res.Status == "invited" {
				return res.Status, nil
			}
			return "", engine.BulkFail(res.Detail)
		})
	reply(c, job, err)
}

// POST /bulk/send — manda a mesma mensagem para vários números, um por vez.
func (s *Server) bulkSend(c *gin.Context) {
	var req struct {
		Numbers      stringList `json:"numbers"`
		Text         string     `json:"text"`
		URL          string     `json:"url"`
		Base64       string     `json:"base64"`
		Type         string     `json:"type"`
		Filename     string     `json:"filename"`
		Mimetype     string     `json:"mimetype"`
		IntervalSecs int        `json:"intervalSeconds"`
		Label        string     `json:"label"`
	}
	if !bind(c, &req) {
		return
	}
	src := req.URL
	if src == "" {
		src = req.Base64
	}
	if strings.TrimSpace(req.Text) == "" && src == "" {
		fail(c, http.StatusBadRequest, errors.New("informe o text ou o url do arquivo"))
		return
	}
	// O arquivo é baixado uma vez só, antes de começar a tarefa.
	var data []byte
	var mime string
	if src != "" {
		var err error
		if data, mime, err = media.Load(c.Request.Context(), src); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		if req.Mimetype != "" {
			mime = req.Mimetype
		}
	}
	sess := s.session(c)
	job, err := s.Engine.Bulk().Start(current(c).Id, engine.BulkSend, req.Label, req.Numbers, req.IntervalSecs,
		func(ctx context.Context, target string) (string, error) {
			to, err := sess.ResolveJID(ctx, target)
			if err != nil {
				return "", engine.BulkFail(err.Error())
			}
			text := engine.Personalize(req.Text, target)
			msg := engine.Text(text)
			if len(data) > 0 {
				if msg, err = sess.Media(ctx, engine.MediaInput{Data: data, Type: req.Type, Mime: mime,
					Caption: text, Filename: req.Filename}); err != nil {
					return "", err
				}
			}
			res, err := sess.Send(ctx, to, msg, engine.SendOptions{})
			if err != nil {
				return "", err
			}
			info, _ := res["Info"].(map[string]any)
			id, _ := info["ID"].(string)
			return id, nil
		})
	reply(c, job, err)
}

// POST /bulk/group/create — cria vários grupos com os mesmos participantes.
func (s *Server) bulkGroupCreate(c *gin.Context) {
	var req struct {
		Names        stringList `json:"names"`
		Participants stringList `json:"participants"`
		Description  string     `json:"description"`
		IntervalSecs int        `json:"intervalSeconds"`
		Label        string     `json:"label"`
	}
	if !bind(c, &req) {
		return
	}
	sess := s.session(c)
	job, err := s.Engine.Bulk().Start(current(c).Id, engine.BulkGroupCreate, req.Label, req.Names, req.IntervalSecs,
		func(ctx context.Context, name string) (string, error) {
			res, err := sess.GroupCreate(ctx, name, req.Participants)
			if err != nil {
				return "", err
			}
			g, _ := res.(*types.GroupInfo)
			if g == nil {
				return "", nil
			}
			if req.Description != "" {
				_ = sess.GroupDescription(ctx, g.JID.String(), req.Description)
			}
			return g.JID.String(), nil
		})
	reply(c, job, err)
}

// GET /bulk/list — tarefas da instância.
func (s *Server) bulkList(c *gin.Context) {
	ok(c, gin.H{"jobs": s.Engine.Bulk().List(current(c).Id)})
}

// GET /bulk/:id — uma tarefa com o relatório item a item.
func (s *Server) bulkGet(c *gin.Context) {
	job, err := s.Engine.Bulk().Get(current(c).Id, c.Param("id"))
	if errors.Is(err, engine.ErrBulkNotFound) {
		fail(c, http.StatusNotFound, err)
		return
	}
	reply(c, job, err)
}

// POST /bulk/:id/pause | resume | cancel
func (s *Server) bulkControl(c *gin.Context) {
	job, err := s.Engine.Bulk().Control(current(c).Id, c.Param("id"), c.Param("action"))
	if errors.Is(err, engine.ErrBulkNotFound) {
		fail(c, http.StatusNotFound, err)
		return
	}
	reply(c, job, err)
}

// bindOptional aceita corpo vazio (todos os campos ficam no valor padrão).
func bindOptional(c *gin.Context, v any) bool {
	if c.Request.Body == nil || c.Request.ContentLength == 0 {
		return true
	}
	if err := c.ShouldBindJSON(v); err != nil {
		fail(c, http.StatusBadRequest, errors.New("JSON inválido: "+err.Error()))
		return false
	}
	return true
}
