// API WhatsApp BOB - Desenvolvido por Edton Braz.

package api

import (
	"encoding/csv"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.mau.fi/whatsmeow/proto/waE2E"

	"github.com/edton180/api-whatsapp-bob/internal/engine"
	"github.com/edton180/api-whatsapp-bob/internal/media"
)

// Membros de grupos: extrair, copiar para outro grupo e enviar mensagem individual a cada membro.

// groupMembers extrai os membros. Com format=csv devolve a planilha (número, nome, administrador).
func (s *Server) groupMembers(c *gin.Context) {
	var req struct {
		GroupJid string `json:"groupJid"`
		Format   string `json:"format"`
	}
	if !bind(c, &req) {
		return
	}
	res, err := s.session(c).GroupMembers(c.Request.Context(), req.GroupJid)
	if err != nil {
		failEngine(c, err)
		return
	}
	if strings.EqualFold(req.Format, "csv") || strings.EqualFold(c.Query("format"), "csv") {
		c.Header("Content-Type", "text/csv; charset=utf-8")
		c.Header("Content-Disposition", `attachment; filename="membros.csv"`)
		w := csv.NewWriter(c.Writer)
		_ = w.Write([]string{"numero", "jid", "nome", "administrador"})
		for _, m := range res["members"].([]engine.GroupMember) {
			admin := "nao"
			if m.IsAdmin || m.IsSuperAdmin {
				admin = "sim"
			}
			_ = w.Write([]string{m.Number, m.JID, m.Name, admin})
		}
		w.Flush()
		return
	}
	ok(c, res)
}

// groupMembersCopy adiciona os membros de um grupo em outro (tarefa em segundo plano).
func (s *Server) groupMembersCopy(c *gin.Context) {
	var req struct {
		FromGroupJid string `json:"fromGroupJid"`
		ToGroupJid   string `json:"toGroupJid"`
		SkipAdmins   bool   `json:"skipAdmins"`
		Batch        int    `json:"batch"`
		Delay        int    `json:"delay"`
		Limit        int    `json:"limit"`
	}
	if !bind(c, &req) {
		return
	}
	job, err := s.session(c).GroupCopyMembers(c.Request.Context(), engine.GroupCopyOptions{
		From: req.FromGroupJid, To: req.ToGroupJid, SkipAdmins: req.SkipAdmins, Batch: req.Batch, Delay: req.Delay, Limit: req.Limit,
	})
	if err != nil {
		failEngine(c, err)
		return
	}
	ok(c, job)
}

// groupMembersSend envia a mensagem no privado de cada membro (tarefa em segundo plano).
func (s *Server) groupMembersSend(c *gin.Context) {
	var req struct {
		GroupJid   string     `json:"groupJid"`
		GroupJids  stringList `json:"groupJids"`
		Text       string     `json:"text"`
		URL        string     `json:"url"`
		Base64     string     `json:"base64"`
		Type       string     `json:"type"`
		Filename   string     `json:"filename"`
		SkipAdmins bool       `json:"skipAdmins"`
		Delay      int        `json:"delay"`
		Limit      int        `json:"limit"`
	}
	if !bind(c, &req) {
		return
	}
	groups := append([]string{}, req.GroupJids...)
	if req.GroupJid != "" {
		groups = append(groups, req.GroupJid)
	}
	sess := s.session(c)
	var msg *waE2E.Message
	src := req.URL
	if src == "" {
		src = req.Base64
	}
	switch {
	case src != "":
		data, mime, err := media.Load(c.Request.Context(), src)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		// A mídia sobe uma vez só; cada membro recebe a mesma mensagem (o text vira legenda)
		if msg, err = sess.Media(c.Request.Context(), engine.MediaInput{Data: data, Type: req.Type, Mime: mime, Caption: req.Text, Filename: req.Filename}); err != nil {
			failEngine(c, err)
			return
		}
	case strings.TrimSpace(req.Text) != "":
		msg = engine.Text(req.Text)
	default:
		fail(c, http.StatusBadRequest, errors.New("informe o text ou a mídia (url ou base64)"))
		return
	}
	job, err := sess.GroupSendToMembers(c.Request.Context(), engine.GroupSendOptions{Groups: groups, SkipAdmins: req.SkipAdmins, Delay: req.Delay, Limit: req.Limit}, msg)
	if err != nil {
		failEngine(c, err)
		return
	}
	ok(c, job)
}

func (s *Server) groupJobs(c *gin.Context) {
	ok(c, gin.H{"jobs": s.session(c).GroupJobList()})
}

func (s *Server) groupJob(c *gin.Context) {
	job, found := s.session(c).GroupJobGet(c.Param("id"))
	if !found {
		fail(c, http.StatusNotFound, errors.New("tarefa não encontrada"))
		return
	}
	ok(c, job)
}

func (s *Server) groupJobCancel(c *gin.Context) {
	job, found := s.session(c).GroupJobCancel(c.Param("id"))
	if !found {
		fail(c, http.StatusNotFound, errors.New("tarefa não encontrada"))
		return
	}
	ok(c, job)
}
