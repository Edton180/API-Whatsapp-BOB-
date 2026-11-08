// API WhatsApp BOB - Desenvolvido por Edton Braz.

package api

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// registerExtra monta as rotas de grupos, comunidades, canais, chamadas e perfil que
// completam a API nativa.
func (s *Server) registerExtra(r *gin.Engine, inst gin.HandlerFunc) {
	gg := r.Group("/group", inst)
	gg.POST("/settings", s.groupSettings)
	gg.POST("/ephemeral", s.ephemeral)
	gg.POST("/inviteinfo", s.groupInviteInfo)
	gg.POST("/requests", s.groupRequests)
	gg.POST("/requests/update", s.groupRequestsUpdate)
	gg.POST("/photo/remove", s.groupPhotoRemove)

	r.POST("/chat/ephemeral", inst, s.ephemeral)

	cm := r.Group("/community", inst)
	cm.POST("/create", s.communityCreate)
	cm.POST("/link", s.communityLink(true))
	cm.POST("/unlink", s.communityLink(false))
	cm.POST("/groups", s.communityGroups)

	ug := r.Group("/user", inst)
	ug.POST("/business", s.userBusiness)
	ug.POST("/privacy", s.userPrivacySet)
	ug.DELETE("/profilePicture", s.profilePictureRemove)

	r.POST("/instance/presence", inst, s.globalPresence)
	r.POST("/call/reject", inst, s.callReject)

	ng := r.Group("/newsletter", inst)
	ng.POST("/create", s.newsletterCreate)
	ng.POST("/info", s.newsletterInfo)
	ng.POST("/follow", s.newsletterFollow(true))
	ng.POST("/unfollow", s.newsletterFollow(false))
	ng.POST("/mute", s.newsletterMute(true))
	ng.POST("/unmute", s.newsletterMute(false))
	ng.POST("/messages", s.newsletterMessages)
}

// ---------------------------------------------------------------- grupos

func (s *Server) groupSettings(c *gin.Context) {
	var req groupRequest
	if !bind(c, &req) {
		return
	}
	reply(c, nil, s.session(c).GroupSetting(c.Request.Context(), req.GroupJid, req.Action))
}

func (s *Server) ephemeral(c *gin.Context) {
	var req struct {
		GroupJid   string `json:"groupJid"`
		Number     string `json:"number"`
		Expiration int64  `json:"expiration"`
	}
	if !bind(c, &req) {
		return
	}
	target := req.Number
	if req.GroupJid != "" {
		target = req.GroupJid
		if !strings.Contains(target, "@") {
			target += "@g.us"
		}
	}
	reply(c, gin.H{"expiration": req.Expiration}, s.session(c).Ephemeral(c.Request.Context(), target, req.Expiration))
}

func (s *Server) groupInviteInfo(c *gin.Context) {
	var req groupRequest
	if !bind(c, &req) {
		return
	}
	res, err := s.session(c).GroupInviteInfo(c.Request.Context(), req.Code)
	reply(c, res, err)
}

func (s *Server) groupRequests(c *gin.Context) {
	var req groupRequest
	if !bind(c, &req) {
		return
	}
	res, err := s.session(c).GroupRequests(c.Request.Context(), req.GroupJid)
	reply(c, res, err)
}

func (s *Server) groupRequestsUpdate(c *gin.Context) {
	var req groupRequest
	if !bind(c, &req) {
		return
	}
	res, err := s.session(c).GroupRequestsUpdate(c.Request.Context(), req.GroupJid, req.Action, req.Participants)
	reply(c, res, err)
}

func (s *Server) groupPhotoRemove(c *gin.Context) {
	var req groupRequest
	if !bind(c, &req) {
		return
	}
	reply(c, nil, s.session(c).RemoveProfilePicture(c.Request.Context(), req.GroupJid))
}

// ---------------------------------------------------------------- comunidades

func (s *Server) communityCreate(c *gin.Context) {
	var req groupRequest
	if !bind(c, &req) {
		return
	}
	res, err := s.session(c).CommunityCreate(c.Request.Context(), req.Name, req.Description)
	reply(c, res, err)
}

func (s *Server) communityLink(link bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			CommunityJid string `json:"communityJid"`
			GroupJid     string `json:"groupJid"`
		}
		if !bind(c, &req) {
			return
		}
		reply(c, nil, s.session(c).CommunityLink(c.Request.Context(), req.CommunityJid, req.GroupJid, link))
	}
}

func (s *Server) communityGroups(c *gin.Context) {
	var req struct {
		CommunityJid string `json:"communityJid"`
	}
	if !bind(c, &req) {
		return
	}
	res, err := s.session(c).CommunityGroups(c.Request.Context(), req.CommunityJid)
	reply(c, res, err)
}

// ---------------------------------------------------------------- perfil

func (s *Server) userBusiness(c *gin.Context) {
	var req struct {
		Number string `json:"number"`
	}
	if !bind(c, &req) {
		return
	}
	res, err := s.session(c).BusinessProfile(c.Request.Context(), req.Number)
	if err == nil && res == nil {
		reply(c, gin.H{"isBusiness": false}, nil)
		return
	}
	reply(c, res, err)
}

func (s *Server) userPrivacySet(c *gin.Context) {
	var req map[string]string
	if !bind(c, &req) {
		return
	}
	res, err := s.session(c).SetPrivacy(c.Request.Context(), req)
	reply(c, res, err)
}

func (s *Server) profilePictureRemove(c *gin.Context) {
	reply(c, nil, s.session(c).RemoveProfilePicture(c.Request.Context(), ""))
}

func (s *Server) globalPresence(c *gin.Context) {
	var req struct {
		Presence string `json:"presence"`
	}
	if !bind(c, &req) {
		return
	}
	reply(c, gin.H{"presence": req.Presence}, s.session(c).SetGlobalPresence(c.Request.Context(), req.Presence))
}

func (s *Server) callReject(c *gin.Context) {
	var req struct {
		From        string `json:"from"`
		CallCreator string `json:"callCreator"`
		CallID      string `json:"callId"`
	}
	if !bind(c, &req) {
		return
	}
	// Com as chamadas ligadas, a chamada viva é recusada pelo motor de chamadas.
	if s.Calls != nil && req.CallID != "" {
		if err := s.Calls.Reject(current(c).Id, req.CallID); err == nil {
			ok(c, gin.H{"ok": true})
			return
		}
	}
	from := req.From
	if from == "" {
		from = req.CallCreator
	}
	reply(c, nil, s.session(c).RejectCall(c.Request.Context(), from, req.CallID))
}

// ---------------------------------------------------------------- canais

type newsletterRequest struct {
	ID          string `json:"newsletterId"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Picture     string `json:"picture"`
	Link        string `json:"link"`
	Count       int    `json:"count"`
}

func (s *Server) newsletterCreate(c *gin.Context) {
	var req newsletterRequest
	if !bind(c, &req) {
		return
	}
	res, err := s.session(c).NewsletterCreate(c.Request.Context(), req.Name, req.Description, req.Picture)
	reply(c, res, err)
}

func (s *Server) newsletterInfo(c *gin.Context) {
	var req newsletterRequest
	if !bind(c, &req) {
		return
	}
	ref := req.ID
	if ref == "" {
		ref = req.Link
	}
	res, err := s.session(c).NewsletterInfo(c.Request.Context(), ref)
	reply(c, res, err)
}

func (s *Server) newsletterFollow(follow bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req newsletterRequest
		if !bind(c, &req) {
			return
		}
		reply(c, nil, s.session(c).NewsletterFollow(c.Request.Context(), req.ID, follow))
	}
}

func (s *Server) newsletterMute(mute bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req newsletterRequest
		if !bind(c, &req) {
			return
		}
		reply(c, nil, s.session(c).NewsletterMute(c.Request.Context(), req.ID, mute))
	}
}

func (s *Server) newsletterMessages(c *gin.Context) {
	var req newsletterRequest
	if !bind(c, &req) {
		return
	}
	res, err := s.session(c).NewsletterMessages(c.Request.Context(), req.ID, req.Count)
	reply(c, res, err)
}
