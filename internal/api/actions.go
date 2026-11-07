// API WhatsApp BOB - Desenvolvido por Edton Braz.

package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// stringList aceita "x" ou ["x", "y"].
type stringList []string

func (l *stringList) UnmarshalJSON(b []byte) error {
	var one string
	if json.Unmarshal(b, &one) == nil {
		if one != "" {
			*l = []string{one}
		}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return errors.New("use um texto ou uma lista de textos")
	}
	*l = many
	return nil
}

func reply(c *gin.Context, data any, err error) {
	if err != nil {
		failEngine(c, err)
		return
	}
	if data == nil {
		data = gin.H{"ok": true}
	}
	ok(c, data)
}

// ---------------------------------------------------------------- mensagens

func (s *Server) react(c *gin.Context) {
	var req struct {
		Number      string `json:"number"`
		ID          string `json:"id"`
		Reaction    string `json:"reaction"`
		FromMe      bool   `json:"fromMe"`
		Participant string `json:"participant"`
	}
	if !bind(c, &req) {
		return
	}
	res, err := s.session(c).React(c.Request.Context(), req.Number, req.ID, req.Reaction, req.FromMe, req.Participant)
	reply(c, res, err)
}

func (s *Server) markRead(c *gin.Context) {
	var req struct {
		Number string     `json:"number"`
		ID     stringList `json:"id"`
	}
	if !bind(c, &req) {
		return
	}
	reply(c, nil, s.session(c).MarkRead(c.Request.Context(), req.Number, req.ID))
}

func (s *Server) revoke(c *gin.Context) {
	var req struct {
		Chat      string `json:"chat"`
		MessageID string `json:"messageId"`
	}
	if !bind(c, &req) {
		return
	}
	res, err := s.session(c).Revoke(c.Request.Context(), req.Chat, req.MessageID)
	reply(c, res, err)
}

func (s *Server) edit(c *gin.Context) {
	var req struct {
		Chat      string `json:"chat"`
		MessageID string `json:"messageId"`
		Message   string `json:"message"`
	}
	if !bind(c, &req) {
		return
	}
	res, err := s.session(c).Edit(c.Request.Context(), req.Chat, req.MessageID, req.Message)
	reply(c, res, err)
}

func (s *Server) presence(c *gin.Context) {
	var req struct {
		Number  string `json:"number"`
		State   string `json:"state"`
		IsAudio bool   `json:"isAudio"`
		Delay   int    `json:"delay"`
	}
	if !bind(c, &req) {
		return
	}
	reply(c, nil, s.session(c).Presence(c.Request.Context(), req.Number, req.State, req.IsAudio, req.Delay))
}

func (s *Server) downloadMedia(c *gin.Context) {
	var req struct {
		Message json.RawMessage `json:"message"`
		ID      string          `json:"id"`
	}
	if !bind(c, &req) {
		return
	}
	data, mime, err := s.session(c).DownloadMedia(c.Request.Context(), req.Message, req.ID)
	if err != nil {
		failEngine(c, err)
		return
	}
	ok(c, gin.H{"base64": base64.StdEncoding.EncodeToString(data), "mimetype": mime, "size": len(data)})
}

// ---------------------------------------------------------------- usuários

func (s *Server) userCheck(c *gin.Context) {
	var req struct {
		Number stringList `json:"number"`
	}
	if !bind(c, &req) {
		return
	}
	users, err := s.session(c).Check(c.Request.Context(), req.Number)
	reply(c, gin.H{"Users": users}, err)
}

func (s *Server) userInfo(c *gin.Context) {
	var req struct {
		Number stringList `json:"number"`
	}
	if !bind(c, &req) {
		return
	}
	users, err := s.session(c).UserInfo(c.Request.Context(), req.Number)
	reply(c, gin.H{"Users": users}, err)
}

func (s *Server) userAvatar(c *gin.Context) {
	var req struct {
		Number  string `json:"number"`
		Preview bool   `json:"preview"`
	}
	if !bind(c, &req) {
		return
	}
	res, err := s.session(c).Avatar(c.Request.Context(), req.Number, req.Preview)
	reply(c, res, err)
}

func (s *Server) userContacts(c *gin.Context) {
	res, err := s.session(c).Contacts(c.Request.Context())
	reply(c, res, err)
}

func (s *Server) userBlock(block bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Number string `json:"number"`
		}
		if !bind(c, &req) {
			return
		}
		res, err := s.session(c).Block(c.Request.Context(), req.Number, block)
		reply(c, res, err)
	}
}

func (s *Server) userBlocklist(c *gin.Context) {
	res, err := s.session(c).Blocklist(c.Request.Context())
	reply(c, res, err)
}

func (s *Server) userPrivacy(c *gin.Context) {
	res, err := s.session(c).Privacy(c.Request.Context())
	reply(c, res, err)
}

func (s *Server) profileName(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if !bind(c, &req) {
		return
	}
	reply(c, nil, s.session(c).ProfileName(c.Request.Context(), req.Name))
}

func (s *Server) profileStatus(c *gin.Context) {
	var req struct {
		Status string `json:"status"`
	}
	if !bind(c, &req) {
		return
	}
	reply(c, nil, s.session(c).ProfileStatus(c.Request.Context(), req.Status))
}

func (s *Server) profilePicture(c *gin.Context) {
	var req struct {
		Image string `json:"image"`
	}
	if !bind(c, &req) {
		return
	}
	id, err := s.session(c).ProfilePicture(c.Request.Context(), req.Image, "")
	reply(c, gin.H{"pictureId": id}, err)
}

// ---------------------------------------------------------------- conversas e etiquetas

func (s *Server) chatAction(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Chat     string `json:"chat"`
			Duration int    `json:"duration"` // horas (mute)
		}
		if !bind(c, &req) {
			return
		}
		reply(c, nil, s.session(c).Chat(c.Request.Context(), action, req.Chat, req.Duration))
	}
}

func (s *Server) labelList(c *gin.Context) {
	list, err := s.Repo.Labels(current(c).Id)
	reply(c, list, err)
}

func (s *Server) labelChat(add bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Jid     string `json:"jid"`
			LabelID string `json:"labelId"`
		}
		if !bind(c, &req) {
			return
		}
		reply(c, nil, s.session(c).Label(c.Request.Context(), req.Jid, req.LabelID, add))
	}
}

func (s *Server) labelMessage(add bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Jid       string `json:"jid"`
			LabelID   string `json:"labelId"`
			MessageID string `json:"messageId"`
		}
		if !bind(c, &req) {
			return
		}
		reply(c, nil, s.session(c).LabelMessage(c.Request.Context(), req.Jid, req.LabelID, req.MessageID, add))
	}
}

func (s *Server) labelEdit(c *gin.Context) {
	var req struct {
		LabelID string `json:"labelId"`
		Name    string `json:"name"`
		Color   int32  `json:"color"`
		Deleted bool   `json:"deleted"`
	}
	if !bind(c, &req) {
		return
	}
	reply(c, nil, s.session(c).LabelEdit(c.Request.Context(), req.LabelID, req.Name, req.Color, req.Deleted))
}

// ---------------------------------------------------------------- grupos

type groupRequest struct {
	GroupJid     string     `json:"groupJid"`
	GroupName    string     `json:"groupName"`
	Name         string     `json:"name"`
	Description  string     `json:"description"`
	Image        string     `json:"image"`
	Action       string     `json:"action"`
	Participants stringList `json:"participants"`
	Reset        bool       `json:"reset"`
	Code         string     `json:"code"`
}

func (s *Server) groupList(c *gin.Context) {
	res, err := s.session(c).Groups(c.Request.Context())
	reply(c, res, err)
}

func (s *Server) groupInfo(c *gin.Context) {
	var req groupRequest
	if !bind(c, &req) {
		return
	}
	res, err := s.session(c).GroupInfo(c.Request.Context(), req.GroupJid)
	reply(c, res, err)
}

func (s *Server) groupCreate(c *gin.Context) {
	var req groupRequest
	if !bind(c, &req) {
		return
	}
	name := req.GroupName
	if name == "" {
		name = req.Name
	}
	res, err := s.session(c).GroupCreate(c.Request.Context(), name, req.Participants)
	reply(c, res, err)
}

func (s *Server) groupParticipant(c *gin.Context) {
	var req groupRequest
	if !bind(c, &req) {
		return
	}
	res, err := s.session(c).GroupParticipants(c.Request.Context(), req.GroupJid, req.Action, req.Participants)
	reply(c, res, err)
}

func (s *Server) groupName(c *gin.Context) {
	var req groupRequest
	if !bind(c, &req) {
		return
	}
	reply(c, nil, s.session(c).GroupName(c.Request.Context(), req.GroupJid, req.Name))
}

func (s *Server) groupDescription(c *gin.Context) {
	var req groupRequest
	if !bind(c, &req) {
		return
	}
	reply(c, nil, s.session(c).GroupDescription(c.Request.Context(), req.GroupJid, req.Description))
}

func (s *Server) groupPhoto(c *gin.Context) {
	var req groupRequest
	if !bind(c, &req) {
		return
	}
	if req.GroupJid == "" {
		fail(c, http.StatusBadRequest, errors.New("informe o groupJid"))
		return
	}
	id, err := s.session(c).ProfilePicture(c.Request.Context(), req.Image, req.GroupJid)
	reply(c, gin.H{"pictureId": id}, err)
}

func (s *Server) groupInvite(c *gin.Context) {
	var req groupRequest
	if !bind(c, &req) {
		return
	}
	link, err := s.session(c).GroupInvite(c.Request.Context(), req.GroupJid, req.Reset)
	reply(c, gin.H{"link": link}, err)
}

func (s *Server) groupJoin(c *gin.Context) {
	var req groupRequest
	if !bind(c, &req) {
		return
	}
	jid, err := s.session(c).GroupJoin(c.Request.Context(), req.Code)
	reply(c, gin.H{"jid": jid}, err)
}

func (s *Server) groupLeave(c *gin.Context) {
	var req groupRequest
	if !bind(c, &req) {
		return
	}
	reply(c, nil, s.session(c).GroupLeave(c.Request.Context(), req.GroupJid))
}

func (s *Server) newsletterList(c *gin.Context) {
	res, err := s.session(c).Newsletters(c.Request.Context())
	reply(c, res, err)
}
