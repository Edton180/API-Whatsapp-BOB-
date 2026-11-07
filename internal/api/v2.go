// API WhatsApp BOB - Desenvolvido por Edton Braz.

package api

// Rotas no padrão da Evolution API v2: /<módulo>/<ação>/:instance, com o NOME da instância
// na URL. Servem para quem já tem um sistema (n8n, CRM, chatbot) feito para esse padrão
// poder trocar para a BOB só mudando o endereço. Tudo roda sobre o motor próprio da BOB.

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mau.fi/whatsmeow/proto/waE2E"

	"github.com/edton180/api-whatsapp-bob/internal/engine"
	"github.com/edton180/api-whatsapp-bob/internal/store"
)

func (s *Server) registerV2(r *gin.Engine) {
	r.GET("/instance/fetchInstances", s.v2FetchInstances)

	n := r.Group("", s.requireNamed())

	i := n.Group("/instance")
	i.GET("/connect/:instance", s.v2Connect)
	i.GET("/connectionState/:instance", s.v2ConnectionState)
	i.POST("/restart/:instance", s.v2Restart)
	i.PUT("/restart/:instance", s.v2Restart)
	i.POST("/setPresence/:instance", s.v2SetPresence)
	i.DELETE("/logout/:instance", s.v2Logout)

	n.POST("/settings/set/:instance", s.v2SettingsSet)
	n.GET("/settings/find/:instance", s.v2SettingsFind)
	n.POST("/webhook/set/:instance", s.v2WebhookSet)
	n.GET("/webhook/find/:instance", s.v2WebhookFind)
	n.POST("/proxy/set/:instance", s.v2ProxySet)
	n.GET("/proxy/find/:instance", s.v2ProxyFind)

	m := n.Group("/message")
	m.POST("/sendText/:instance", s.v2SendText)
	m.POST("/sendMedia/:instance", s.v2SendMedia)
	m.POST("/sendPtv/:instance", s.v2SendPtv)
	m.POST("/sendWhatsAppAudio/:instance", s.v2SendAudio)
	m.POST("/sendSticker/:instance", s.v2SendSticker)
	m.POST("/sendLocation/:instance", s.v2SendLocation)
	m.POST("/sendContact/:instance", s.v2SendContact)
	m.POST("/sendReaction/:instance", s.v2SendReaction)
	m.POST("/sendPoll/:instance", s.v2SendPoll)
	m.POST("/sendList/:instance", s.v2SendList)
	m.POST("/sendButtons/:instance", s.v2SendButtons)
	m.POST("/sendStatus/:instance", s.v2SendStatus)

	ch := n.Group("/chat")
	ch.POST("/whatsappNumbers/:instance", s.v2WhatsappNumbers)
	ch.POST("/markMessageAsRead/:instance", s.v2MarkRead)
	ch.POST("/markChatUnread/:instance", s.v2MarkChatUnread)
	ch.POST("/archiveChat/:instance", s.v2ArchiveChat)
	ch.DELETE("/deleteMessageForEveryone/:instance", s.v2DeleteForEveryone)
	ch.POST("/updateMessage/:instance", s.v2UpdateMessage)
	ch.POST("/sendPresence/:instance", s.v2SendPresence)
	ch.POST("/updateBlockStatus/:instance", s.v2UpdateBlockStatus)
	ch.POST("/fetchProfilePictureUrl/:instance", s.v2ProfilePictureURL)
	ch.POST("/fetchProfile/:instance", s.v2FetchProfile)
	ch.POST("/fetchBusinessProfile/:instance", s.v2BusinessProfile)
	ch.POST("/findContacts/:instance", s.v2FindContacts)
	ch.POST("/getBase64FromMediaMessage/:instance", s.v2Base64FromMedia)
	ch.POST("/updateProfileName/:instance", s.v2UpdateProfileName)
	ch.POST("/updateProfileStatus/:instance", s.v2UpdateProfileStatus)
	ch.POST("/updateProfilePicture/:instance", s.v2UpdateProfilePicture)
	ch.DELETE("/removeProfilePicture/:instance", s.v2RemoveProfilePicture)
	ch.GET("/fetchPrivacySettings/:instance", s.v2FetchPrivacy)
	ch.POST("/updatePrivacySettings/:instance", s.v2UpdatePrivacy)

	g := n.Group("/group")
	g.POST("/create/:instance", s.v2GroupCreate)
	g.POST("/updateGroupSubject/:instance", s.v2GroupSubject)
	g.POST("/updateGroupDescription/:instance", s.v2GroupDescription)
	g.POST("/updateGroupPicture/:instance", s.v2GroupPicture)
	g.GET("/findGroupInfos/:instance", s.v2GroupInfo)
	g.GET("/fetchAllGroups/:instance", s.v2GroupAll)
	g.GET("/participants/:instance", s.v2GroupParticipants)
	g.GET("/inviteCode/:instance", s.v2GroupInviteCode)
	g.POST("/revokeInviteCode/:instance", s.v2GroupRevokeInvite)
	g.PUT("/revokeInviteCode/:instance", s.v2GroupRevokeInvite)
	g.GET("/inviteInfo/:instance", s.v2GroupInviteInfo)
	g.GET("/acceptInviteCode/:instance", s.v2GroupAcceptInvite)
	g.POST("/sendInvite/:instance", s.v2GroupSendInvite)
	g.POST("/updateParticipant/:instance", s.v2GroupUpdateParticipant)
	g.POST("/updateSetting/:instance", s.v2GroupUpdateSetting)
	g.POST("/toggleEphemeral/:instance", s.v2GroupEphemeral)
	g.DELETE("/leaveGroup/:instance", s.v2GroupLeave)

	n.GET("/label/findLabels/:instance", s.v2FindLabels)
	n.POST("/label/handleLabel/:instance", s.v2HandleLabel)

	n.POST("/call/offer/:instance", s.v2CallOffer)
}

// requireNamed aceita a chave global ou o token da instância citada em :instance
// (nome ou id).
func (s *Server) requireNamed() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := apiKey(c)
		if key == "" {
			v2Fail(c, http.StatusUnauthorized, "Unauthorized")
			return
		}
		ref := c.Param("instance")
		inst, err := s.Repo.ByIDOrName(ref)
		global := s.isGlobal(key)
		if err != nil || inst == nil {
			if !global {
				v2Fail(c, http.StatusUnauthorized, "Unauthorized")
				return
			}
			v2Fail(c, http.StatusNotFound, "The \""+ref+"\" instance does not exist")
			return
		}
		if !global && subtle.ConstantTimeCompare([]byte(key), []byte(inst.Token)) != 1 {
			v2Fail(c, http.StatusUnauthorized, "Unauthorized")
			return
		}
		c.Set("instance", inst)
		c.Next()
	}
}

// v2Fail responde no formato de erro do padrão v2.
func v2Fail(c *gin.Context, code int, msg ...string) {
	c.AbortWithStatusJSON(code, gin.H{
		"status":   code,
		"error":    http.StatusText(code),
		"response": gin.H{"message": msg},
	})
}

// v2Engine traduz erros do motor.
func v2Engine(c *gin.Context, err error) {
	switch {
	case errors.Is(err, engine.ErrNotLogged), errors.Is(err, engine.ErrNotConnected):
		v2Fail(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, store.ErrNotFound):
		v2Fail(c, http.StatusNotFound, err.Error())
	default:
		v2Fail(c, http.StatusBadRequest, err.Error())
	}
}

func v2Bind(c *gin.Context, v any) bool {
	if err := c.ShouldBindJSON(v); err != nil {
		v2Fail(c, http.StatusBadRequest, "JSON inválido: "+err.Error())
		return false
	}
	return true
}

// v2Message monta a resposta de envio: {key, message, messageType, messageTimestamp, status}.
func v2Message(inst *store.Instance, res map[string]any) gin.H {
	info, _ := res["Info"].(map[string]any)
	msg, _ := res["Message"].(*waE2E.Message)
	var body map[string]any
	if msg != nil {
		b, _ := json.Marshal(msg)
		_ = json.Unmarshal(b, &body)
	}
	ts := time.Now()
	if t, ok := info["Timestamp"].(time.Time); ok && !t.IsZero() {
		ts = t
	}
	return gin.H{
		"key": gin.H{
			"remoteJid": info["Chat"],
			"fromMe":    true,
			"id":        info["ID"],
		},
		"status":           "PENDING",
		"message":          body,
		"messageType":      messageType(body),
		"messageTimestamp": ts.Unix(),
		"instanceId":       inst.Id,
		"source":           "api",
	}
}

func messageType(m map[string]any) string {
	for k := range m {
		if k != "messageContextInfo" {
			return k
		}
	}
	return "unknown"
}

func groupParam(c *gin.Context) (string, bool) {
	g := strings.TrimSpace(c.Query("groupJid"))
	if g == "" {
		v2Fail(c, http.StatusBadRequest, "informe ?groupJid= na URL")
		return "", false
	}
	return g, true
}
