package evolutionapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	label_service "github.com/evolution-foundation/evolution-go/pkg/label/service"
)

// GET /label/findLabels/:instance
func (h *Handler) FindLabels(c *gin.Context) {
	labels, err := h.LabelService.GetLabels(getInstance(c))
	if err != nil {
		internalError(c, err)
		return
	}
	out := make([]gin.H, 0, len(labels))
	for _, l := range labels {
		out = append(out, gin.H{
			"id":           l.LabelID,
			"name":         l.LabelName,
			"color":        l.LabelColor,
			"predefinedId": l.PredefinedId,
		})
	}
	c.JSON(http.StatusOK, out)
}

// POST /label/handleLabel/:instance
func (h *Handler) HandleLabel(c *gin.Context) {
	var req struct {
		Number  string `json:"number"`
		LabelID string `json:"labelId"`
		Action  string `json:"action"`
	}
	if !bindJSON(c, &req) {
		return
	}
	jid, err := parseJID(req.Number)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	if req.LabelID == "" {
		badRequest(c, "labelId is required")
		return
	}
	data := &label_service.ChatLabelStruct{JID: jid.String(), LabelID: req.LabelID}
	inst := getInstance(c)
	switch req.Action {
	case "add":
		err = h.LabelService.ChatLabel(data, inst)
	case "remove":
		err = h.LabelService.ChatUnlabel(data, inst)
	default:
		badRequest(c, "action must be add or remove")
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"numberJid": jid.String(), "labelId": req.LabelID, "add": req.Action == "add", "remove": req.Action == "remove"})
}

// POST /call/offer/:instance
//
// WhatsApp's multi-device protocol used by the engine cannot place calls, so
// this route reports it clearly instead of pretending it worked.
func (h *Handler) OfferCall(c *gin.Context) {
	errorResponse(c, http.StatusNotImplemented, "placing calls is not supported by the WhatsApp Web protocol; use /call/reject or the CALL webhook event")
}
