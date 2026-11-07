// Package evolutionapi exposes the Evolution API v2 REST contract
// (https://doc.evolution-api.com/v2) on top of the Evolution Go engine.
//
// Every route follows the Evolution API shape `/<module>/<action>/:instance`,
// where `:instance` is the instance *name*. Requests are authenticated with the
// `apikey` header, which may be either the GLOBAL_API_KEY or the token of the
// instance named in the URL. Handlers translate the Evolution API payloads to
// the engine's own services, so both APIs drive the same WhatsApp sessions.
package evolutionapi

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"

	call_service "github.com/evolution-foundation/evolution-go/pkg/call/service"
	chat_service "github.com/evolution-foundation/evolution-go/pkg/chat/service"
	"github.com/evolution-foundation/evolution-go/pkg/config"
	group_service "github.com/evolution-foundation/evolution-go/pkg/group/service"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	instance_repository "github.com/evolution-foundation/evolution-go/pkg/instance/repository"
	instance_service "github.com/evolution-foundation/evolution-go/pkg/instance/service"
	label_service "github.com/evolution-foundation/evolution-go/pkg/label/service"
	message_service "github.com/evolution-foundation/evolution-go/pkg/message/service"
	send_service "github.com/evolution-foundation/evolution-go/pkg/sendMessage/service"
	user_service "github.com/evolution-foundation/evolution-go/pkg/user/service"
	"github.com/evolution-foundation/evolution-go/pkg/utils"
	whatsmeow_service "github.com/evolution-foundation/evolution-go/pkg/whatsmeow/service"
)

const instanceKey = "instance"

// Deps groups the engine services the compatibility layer is built on.
type Deps struct {
	Config             *config.Config
	ClientPointer      map[string]*whatsmeow.Client
	InstanceRepository instance_repository.InstanceRepository
	InstanceService    instance_service.InstanceService
	WhatsmeowService   whatsmeow_service.WhatsmeowService
	SendService        send_service.SendService
	MessageService     message_service.MessageService
	UserService        user_service.UserService
	ChatService        chat_service.ChatService
	GroupService       group_service.GroupService
	LabelService       label_service.LabelService
	CallService        call_service.CallService
}

// Handler implements the Evolution API v2 routes.
type Handler struct {
	Deps
}

func New(d Deps) *Handler {
	return &Handler{Deps: d}
}

// ---------------------------------------------------------------------------
// Auth
// ---------------------------------------------------------------------------

func apiKey(c *gin.Context) string {
	if k := c.GetHeader("apikey"); k != "" {
		return k
	}
	return strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
}

// requireGlobal accepts only the GLOBAL_API_KEY.
func (h *Handler) requireGlobal(c *gin.Context) {
	if k := apiKey(c); k == "" || k != h.Config.GlobalApiKey {
		unauthorized(c)
		return
	}
	c.Next()
}

// requireInstance resolves `:instance` by name and accepts the global key or
// that instance's own token.
func (h *Handler) requireInstance(c *gin.Context) {
	key := apiKey(c)
	if key == "" {
		unauthorized(c)
		return
	}
	name := c.Param("instance")
	inst, err := h.InstanceRepository.GetInstanceByName(name)
	if err != nil || inst == nil {
		// Allow addressing the instance by id as well.
		inst, err = h.InstanceRepository.GetInstanceByID(name)
	}
	if err != nil || inst == nil {
		if key != h.Config.GlobalApiKey {
			unauthorized(c)
			return
		}
		notFound(c, fmt.Sprintf("The \"%s\" instance does not exist", name))
		return
	}
	if key != h.Config.GlobalApiKey && key != inst.Token {
		unauthorized(c)
		return
	}
	c.Set(instanceKey, inst)
	c.Next()
}

func getInstance(c *gin.Context) *instance_model.Instance {
	return c.MustGet(instanceKey).(*instance_model.Instance)
}

// ---------------------------------------------------------------------------
// Responses (same envelope as Evolution API v2 errors)
// ---------------------------------------------------------------------------

func errorResponse(c *gin.Context, status int, msg ...string) {
	c.AbortWithStatusJSON(status, gin.H{
		"status": status,
		"error":  http.StatusText(status),
		"response": gin.H{
			"message": msg,
		},
	})
}

func unauthorized(c *gin.Context)           { errorResponse(c, http.StatusUnauthorized, "Unauthorized") }
func notFound(c *gin.Context, msg string)   { errorResponse(c, http.StatusNotFound, msg) }
func badRequest(c *gin.Context, msg string) { errorResponse(c, http.StatusBadRequest, msg) }
func internalError(c *gin.Context, err error) {
	errorResponse(c, http.StatusInternalServerError, err.Error())
}

func bindJSON(c *gin.Context, v interface{}) bool {
	if err := c.ShouldBindJSON(v); err != nil {
		badRequest(c, err.Error())
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// client returns a connected whatsmeow client for the instance, starting the
// session if it is not running yet.
func (h *Handler) client(inst *instance_model.Instance) (*whatsmeow.Client, error) {
	cli := h.ClientPointer[inst.Id]
	if cli == nil {
		if err := h.WhatsmeowService.StartInstance(inst.Id); err != nil {
			return nil, err
		}
		for i := 0; i < 20 && h.ClientPointer[inst.Id] == nil; i++ {
			time.Sleep(250 * time.Millisecond)
		}
		cli = h.ClientPointer[inst.Id]
	}
	if cli == nil || !cli.IsConnected() {
		return nil, errors.New("instance is not connected")
	}
	if !cli.IsLoggedIn() {
		return nil, errors.New("instance is not logged in, read the QR code first")
	}
	return cli, nil
}

func (h *Handler) clientOrFail(c *gin.Context) (*whatsmeow.Client, *instance_model.Instance, bool) {
	inst := getInstance(c)
	cli, err := h.client(inst)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return nil, nil, false
	}
	return cli, inst, true
}

// normalizeNumber turns a phone number or JID into the canonical JID string
// used by the engine services.
func normalizeNumber(number string) (string, error) {
	return utils.CreateJID(strings.TrimSpace(number))
}

func parseJID(number string) (types.JID, error) {
	jid, ok := utils.ParseJID(strings.TrimSpace(number))
	if !ok {
		return types.EmptyJID, fmt.Errorf("invalid number or jid: %s", number)
	}
	return utils.CanonicalJID(jid), nil
}

// decodeMedia accepts a base64 string, optionally as a data URI.
func decodeMedia(media string) ([]byte, error) {
	if i := strings.Index(media, "base64,"); i >= 0 && strings.HasPrefix(media, "data:") {
		media = media[i+len("base64,"):]
	}
	media = strings.TrimSpace(media)
	if b, err := base64.StdEncoding.DecodeString(media); err == nil {
		return b, nil
	}
	return base64.RawStdEncoding.DecodeString(media)
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func ctx() context.Context {
	return context.Background()
}

const maxDownload = 64 << 20 // 64 MB

var httpClient = &http.Client{Timeout: 60 * time.Second}

func downloadURL(u string) ([]byte, error) {
	resp, err := httpClient.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("download failed: %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxDownload))
}
