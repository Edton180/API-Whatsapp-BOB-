// API WhatsApp BOB - Desenvolvido por Edton Braz.

// Package api expõe as rotas HTTP da BOB.
package api

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/edton180/api-whatsapp-bob/internal/calls"
	"github.com/edton180/api-whatsapp-bob/internal/engine"
	"github.com/edton180/api-whatsapp-bob/internal/integrations"
	"github.com/edton180/api-whatsapp-bob/internal/panel"
	"github.com/edton180/api-whatsapp-bob/internal/store"
)

// Server reúne as dependências das rotas.
type Server struct {
	Engine       *engine.Engine
	Repo         *store.Repo
	Integrations *integrations.Manager
	Calls        *calls.Manager // nil quando o servidor não liga chamadas
}

// Router monta todas as rotas.
func (s *Server) Router() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), cors())
	r.MaxMultipartMemory = 64 << 20

	// públicas
	r.GET("/", s.home)
	r.GET("/server/ok", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.GET("/donate/pix", func(c *gin.Context) { ok(c, PixInfo()) })
	r.GET("/license/status", s.license)
	r.GET("/license/register", s.license)
	r.GET("/license/activate", s.license)
	r.GET("/openapi.yaml", func(c *gin.Context) { c.Data(http.StatusOK, "application/yaml; charset=utf-8", panel.OpenAPI()) })
	r.GET("/openapi.json", func(c *gin.Context) { c.Data(http.StatusOK, "application/json; charset=utf-8", panel.OpenAPIJSON()) })
	r.GET("/swagger", func(c *gin.Context) { c.Redirect(http.StatusFound, "/swagger/index.html") })
	r.GET("/swagger/*any", func(c *gin.Context) { c.Data(http.StatusOK, "text/html; charset=utf-8", panel.SwaggerPage()) })
	if s.Engine.Cfg.PanelEnabled {
		panel.Register(r)
	}

	global := s.requireGlobal()
	inst := s.requireInstance()

	r.GET("/server/info", global, s.serverInfo)
	r.GET("/ws", s.websocket)

	// instâncias (chave global)
	g := r.Group("/instance")
	g.POST("/create", global, s.v2CreateOrNative(s.instanceCreate))
	g.GET("/all", global, s.instanceAll)
	g.GET("/info/:id", global, s.instanceInfo)
	g.DELETE("/delete/:id", global, s.instanceDelete)
	g.POST("/forcereconnect/:id", global, s.instanceForceReconnect)
	g.POST("/proxy/:id", global, s.instanceProxySet)
	g.DELETE("/proxy/:id", global, s.instanceProxyDelete)
	g.GET("/logs/:id", global, s.instanceLogs)
	g.GET("/:id/advanced-settings", s.requireAny(), s.settingsGet)
	g.PUT("/:id/advanced-settings", s.requireAny(), s.settingsPut)
	g.POST("/:id/webhook", s.requireAny(), s.webhookSet)

	// conexão (token da instância)
	g.POST("/connect", inst, s.connect)
	g.GET("/status", inst, s.status)
	g.GET("/qr", inst, s.qr)
	g.POST("/pair", inst, s.pair)
	g.POST("/disconnect", inst, s.disconnect)
	g.POST("/reconnect", inst, s.reconnect)
	g.DELETE("/logout", inst, s.logout)

	// envio
	sg := r.Group("/send", inst)
	sg.POST("/text", s.sendText)
	sg.POST("/link", s.sendLink)
	sg.POST("/media", s.sendMedia)
	sg.POST("/sticker", s.sendSticker)
	sg.POST("/location", s.sendLocation)
	sg.POST("/contact", s.sendContact)
	sg.POST("/poll", s.sendPoll)
	sg.POST("/button", s.sendButton)
	sg.POST("/list", s.sendList)
	sg.POST("/status/text", s.sendStatusText)
	sg.POST("/status/media", s.sendStatusMedia)

	// mensagens
	mg := r.Group("/message", inst)
	mg.POST("/react", s.react)
	mg.POST("/markread", s.markRead)
	mg.POST("/delete", s.revoke)
	mg.POST("/edit", s.edit)
	mg.POST("/presence", s.presence)
	mg.POST("/downloadmedia", s.downloadMedia)

	// usuários
	ug := r.Group("/user", inst)
	ug.POST("/check", s.userCheck)
	ug.POST("/info", s.userInfo)
	ug.POST("/avatar", s.userAvatar)
	ug.GET("/contacts", s.userContacts)
	ug.POST("/block", s.userBlock(true))
	ug.POST("/unblock", s.userBlock(false))
	ug.GET("/blocklist", s.userBlocklist)
	ug.GET("/privacy", s.userPrivacy)
	ug.POST("/profileName", s.profileName)
	ug.POST("/profileStatus", s.profileStatus)
	ug.POST("/profilePicture", s.profilePicture)

	// conversas
	cg := r.Group("/chat", inst)
	for _, a := range []string{"archive", "unarchive", "pin", "unpin", "mute", "unmute", "read", "unread"} {
		cg.POST("/"+a, s.chatAction(a))
	}

	// etiquetas
	r.GET("/label/list", inst, s.labelList)
	r.POST("/label/chat", inst, s.labelChat(true))
	r.POST("/unlabel/chat", inst, s.labelChat(false))
	r.POST("/label/message", inst, s.labelMessage(true))
	r.POST("/unlabel/message", inst, s.labelMessage(false))
	r.POST("/label/edit", inst, s.labelEdit)

	// grupos
	gg := r.Group("/group", inst)
	gg.GET("/myall", s.groupList)
	gg.GET("/list", s.groupList)
	gg.POST("/info", s.groupInfo)
	gg.POST("/create", s.groupCreate)
	gg.POST("/participant", s.groupParticipant)
	gg.POST("/name", s.groupName)
	gg.POST("/description", s.groupDescription)
	gg.POST("/photo", s.groupPhoto)
	gg.POST("/invitelink", s.groupInvite)
	gg.POST("/join", s.groupJoin)
	gg.POST("/leave", s.groupLeave)

	r.GET("/newsletter/list", inst, s.newsletterList)

	// grupos, comunidades, canais, chamadas e perfil (complementos)
	s.registerExtra(r, inst)
	s.registerCalls(r, inst)
	s.registerHistory(r, inst, s.requireNamed())

	// rotas no padrão da Evolution API v2 (/message/sendText/:instance...)
	s.registerV2(r)

	// integrações (Chatwoot, Typebot, N8N)
	if s.Integrations != nil {
		s.Integrations.Register(r, inst)
	}

	r.NoRoute(func(c *gin.Context) { fail(c, http.StatusNotFound, errors.New("rota não encontrada: veja /swagger")) })
	return r
}

// ---------------------------------------------------------------- respostas

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"data": data, "message": "success"})
}

func fail(c *gin.Context, code int, err error) {
	msg := "erro"
	if err != nil {
		msg = err.Error()
	}
	c.AbortWithStatusJSON(code, gin.H{"error": msg, "message": msg})
}

// failEngine traduz erros do motor para o código HTTP certo.
func failEngine(c *gin.Context, err error) {
	switch {
	case errors.Is(err, engine.ErrNotLogged), errors.Is(err, engine.ErrNotConnected):
		fail(c, http.StatusConflict, err)
	case errors.Is(err, store.ErrNotFound):
		fail(c, http.StatusNotFound, err)
	default:
		fail(c, http.StatusBadRequest, err)
	}
}

func bind(c *gin.Context, v any) bool {
	if err := c.ShouldBindJSON(v); err != nil {
		fail(c, http.StatusBadRequest, errors.New("JSON inválido: "+err.Error()))
		return false
	}
	return true
}

// ---------------------------------------------------------------- autenticação

func apiKey(c *gin.Context) string {
	if k := c.GetHeader("apikey"); k != "" {
		return k
	}
	if a := c.GetHeader("Authorization"); strings.HasPrefix(a, "Bearer ") {
		return strings.TrimPrefix(a, "Bearer ")
	}
	return ""
}

func (s *Server) isGlobal(key string) bool {
	g := s.Engine.Cfg.APIKey
	return g != "" && key != "" && subtle.ConstantTimeCompare([]byte(key), []byte(g)) == 1
}

func (s *Server) requireGlobal() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !s.isGlobal(apiKey(c)) {
			fail(c, http.StatusUnauthorized, errors.New("chave global inválida (cabeçalho apikey)"))
			return
		}
		c.Next()
	}
}

// requireInstance aceita o token da instância, ou a chave global junto com o cabeçalho
// "instance" (id ou nome) ou ?instance=.
func (s *Server) requireInstance() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := apiKey(c)
		var inst *store.Instance
		var err error
		if s.isGlobal(key) {
			ref := c.GetHeader("instance")
			if ref == "" {
				ref = c.Query("instance")
			}
			if ref == "" {
				fail(c, http.StatusBadRequest, errors.New("com a chave global, informe a instância no cabeçalho \"instance\""))
				return
			}
			inst, err = s.Repo.ByIDOrName(ref)
		} else {
			inst, err = s.Repo.ByToken(key)
		}
		if err != nil || inst == nil {
			fail(c, http.StatusUnauthorized, errors.New("token da instância inválido (cabeçalho apikey)"))
			return
		}
		c.Set("instance", inst)
		c.Next()
	}
}

// requireAny aceita a chave global ou o token da própria instância da rota (:id).
func (s *Server) requireAny() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := apiKey(c)
		inst, err := s.Repo.ByIDOrName(c.Param("id"))
		if err != nil {
			fail(c, http.StatusNotFound, store.ErrNotFound)
			return
		}
		if !s.isGlobal(key) && subtle.ConstantTimeCompare([]byte(key), []byte(inst.Token)) != 1 {
			fail(c, http.StatusUnauthorized, errors.New("chave inválida"))
			return
		}
		c.Set("instance", inst)
		c.Next()
	}
}

func current(c *gin.Context) *store.Instance {
	v, _ := c.Get("instance")
	inst, _ := v.(*store.Instance)
	return inst
}

func (s *Server) session(c *gin.Context) *engine.Session {
	return s.Engine.Session(current(c))
}

func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Headers", "Content-Type, apikey, instance, Authorization")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
