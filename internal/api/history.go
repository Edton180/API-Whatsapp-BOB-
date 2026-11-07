// API WhatsApp BOB - Desenvolvido por Edton Braz.

package api

// Rotas do histórico de mensagens e conversas (nativas e no padrão v2).

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/edton180/api-whatsapp-bob/internal/engine"
	"github.com/edton180/api-whatsapp-bob/internal/store"
)

func (s *Server) registerHistory(r *gin.Engine, inst gin.HandlerFunc, named gin.HandlerFunc) {
	r.GET("/chat/list", inst, s.chatList)
	r.GET("/chat/messages", inst, s.chatMessages)
	r.POST("/chat/history", inst, s.chatHistory)
	r.GET("/message/get", inst, s.messageGet)
	r.GET("/message/poll", inst, s.messagePoll)

	r.POST("/chat/findMessages/:instance", named, s.v2FindMessages)
	r.POST("/chat/findChats/:instance", named, s.v2FindChats)
	r.POST("/chat/findStatusMessage/:instance", named, s.v2FindStatusMessage)
}

// historyOn responde 400 quando STORE_MESSAGES=false.
func (s *Server) historyOn(c *gin.Context, v2 bool) bool {
	if s.Engine.Cfg.StoreMessages {
		return true
	}
	if v2 {
		v2Fail(c, http.StatusBadRequest, engine.ErrHistoryOff.Error())
	} else {
		fail(c, http.StatusBadRequest, engine.ErrHistoryOff)
	}
	return false
}

// messageJSON monta a mensagem do histórico no formato {key, pushName, message, ...}.
func messageJSON(m *store.Message) gin.H {
	var body map[string]any
	if msg := engine.DecodeMessage(m.Raw); msg != nil {
		b, _ := json.Marshal(msg)
		_ = json.Unmarshal(b, &body)
	}
	key := gin.H{"id": m.ID, "remoteJid": m.Chat, "fromMe": m.FromMe}
	if strings.HasSuffix(m.Chat, "@g.us") && m.Sender != "" {
		key["participant"] = m.Sender
	}
	return gin.H{
		"id":               m.ID,
		"key":              key,
		"pushName":         m.PushName,
		"sender":           m.Sender,
		"messageType":      m.Type,
		"message":          body,
		"text":             m.Text,
		"messageTimestamp": m.Timestamp,
		"status":           m.Status,
		"edited":           m.Edited,
		"deleted":          m.Deleted,
		"instanceId":       m.InstanceID,
	}
}

func chatJSON(ch *store.Chat, last *store.Message) gin.H {
	out := gin.H{
		"id":            ch.JID,
		"remoteJid":     ch.JID,
		"name":          ch.Name,
		"pushName":      ch.Name,
		"unreadCount":   ch.Unread,
		"archived":      ch.Archived,
		"pinned":        ch.Pinned,
		"isGroup":       strings.HasSuffix(ch.JID, "@g.us"),
		"lastTimestamp": ch.LastTimestamp,
		"updatedAt":     time.Unix(ch.LastTimestamp, 0).UTC().Format(time.RFC3339),
		"instanceId":    ch.InstanceID,
	}
	if last != nil {
		out["lastMessage"] = messageJSON(last)
	} else if ch.LastMessageID != "" {
		out["lastMessage"] = gin.H{
			"key":              gin.H{"id": ch.LastMessageID, "remoteJid": ch.JID, "fromMe": ch.LastFromMe},
			"text":             ch.LastText,
			"messageTimestamp": ch.LastTimestamp,
		}
	}
	return out
}

func queryInt(c *gin.Context, name string, def int) int {
	if n, err := strconv.Atoi(c.Query(name)); err == nil {
		return n
	}
	return def
}

// ---------------------------------------------------------------- nativas

// chatList: GET /chat/list?limit=100&offset=0
func (s *Server) chatList(c *gin.Context) {
	if !s.historyOn(c, false) {
		return
	}
	limit, offset := queryInt(c, "limit", 100), queryInt(c, "offset", 0)
	list, total, err := s.Repo.Chats(current(c).Id, limit, offset)
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	out := make([]gin.H, 0, len(list))
	for i := range list {
		out = append(out, chatJSON(&list[i], nil))
	}
	ok(c, gin.H{"total": total, "limit": limit, "offset": offset, "chats": out})
}

// chatMessages: GET /chat/messages?chat=5511...&limit=50&offset=0&before=<unix>&fromMe=true
func (s *Server) chatMessages(c *gin.Context) {
	if !s.historyOn(c, false) {
		return
	}
	q := store.MessageQuery{
		Chat:   s.session(c).ChatJID(c.Request.Context(), c.Query("chat")),
		Limit:  queryInt(c, "limit", 50),
		Offset: queryInt(c, "offset", 0),
		Before: int64(queryInt(c, "before", 0)),
		After:  int64(queryInt(c, "after", 0)),
	}
	if v := c.Query("fromMe"); v != "" {
		b := v == "true" || v == "1"
		q.FromMe = &b
	}
	list, total, err := s.Repo.Messages(current(c).Id, q)
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	out := make([]gin.H, 0, len(list))
	for i := range list {
		out = append(out, messageJSON(&list[i]))
	}
	ok(c, gin.H{"total": total, "limit": q.Limit, "offset": q.Offset, "messages": out})
}

// messageGet: GET /message/get?id=<id da mensagem>
func (s *Server) messageGet(c *gin.Context) {
	if !s.historyOn(c, false) {
		return
	}
	m, err := s.Repo.MessageByID(current(c).Id, c.Query("id"))
	if err != nil {
		fail(c, http.StatusNotFound, err)
		return
	}
	ok(c, messageJSON(m))
}

// messagePoll: GET /message/poll?id=<id da enquete> devolve os votos somados.
func (s *Server) messagePoll(c *gin.Context) {
	res, err := s.session(c).PollResults(c.Query("id"))
	if errors.Is(err, store.ErrMessageNotFound) {
		fail(c, http.StatusNotFound, errors.New("enquete não encontrada no histórico"))
		return
	}
	reply(c, res, err)
}

// chatHistory: POST /chat/history {"chat": "5511...", "count": 50} pede ao celular mensagens
// mais antigas da conversa; elas chegam pelo evento HistorySync e entram no histórico.
func (s *Server) chatHistory(c *gin.Context) {
	var req struct {
		Chat  string `json:"chat"`
		Count int    `json:"count"`
	}
	if !bind(c, &req) {
		return
	}
	res, err := s.session(c).RequestHistory(c.Request.Context(), req.Chat, req.Count)
	reply(c, res, err)
}

// ---------------------------------------------------------------- padrão v2

type v2Where struct {
	Where struct {
		ID        string `json:"id"`
		RemoteJid string `json:"remoteJid"`
		Key       struct {
			ID        string `json:"id"`
			RemoteJid string `json:"remoteJid"`
			FromMe    *bool  `json:"fromMe"`
		} `json:"key"`
	} `json:"where"`
	Page   int `json:"page"`
	Offset int `json:"offset"` // no padrão v2, "offset" é a quantidade por página
	Limit  int `json:"limit"`
}

func (w *v2Where) paging() (page, size int) {
	page, size = w.Page, w.Offset
	if size <= 0 {
		size = w.Limit
	}
	if size <= 0 || size > 500 {
		size = 50
	}
	if page <= 0 {
		page = 1
	}
	return page, size
}

func (s *Server) v2FindMessages(c *gin.Context) {
	if !s.historyOn(c, true) {
		return
	}
	var req v2Where
	_ = c.ShouldBindJSON(&req)
	page, size := req.paging()
	chat := req.Where.Key.RemoteJid
	if chat == "" {
		chat = req.Where.RemoteJid
	}
	id := req.Where.Key.ID
	if id == "" {
		id = req.Where.ID
	}
	q := store.MessageQuery{
		Chat: s.session(c).ChatJID(c.Request.Context(), chat), ID: id, FromMe: req.Where.Key.FromMe,
		Limit: size, Offset: (page - 1) * size,
	}
	list, total, err := s.Repo.Messages(current(c).Id, q)
	if err != nil {
		v2Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	records := make([]gin.H, 0, len(list))
	for i := range list {
		records = append(records, messageJSON(&list[i]))
	}
	pages := (int(total) + size - 1) / size
	c.JSON(http.StatusOK, gin.H{"messages": gin.H{"total": total, "pages": pages, "currentPage": page, "records": records}})
}

func (s *Server) v2FindChats(c *gin.Context) {
	if !s.historyOn(c, true) {
		return
	}
	var req v2Where
	_ = c.ShouldBindJSON(&req)
	inst := current(c).Id
	if jid := req.Where.RemoteJid; jid != "" {
		ch, err := s.Repo.ChatByJID(inst, s.session(c).ChatJID(c.Request.Context(), jid))
		if err != nil {
			c.JSON(http.StatusOK, []gin.H{})
			return
		}
		last, _ := s.Repo.MessageByID(inst, ch.LastMessageID)
		c.JSON(http.StatusOK, []gin.H{chatJSON(ch, last)})
		return
	}
	page, size := req.paging()
	if req.Offset <= 0 && req.Limit <= 0 {
		size = 500
	}
	list, _, err := s.Repo.Chats(inst, size, (page-1)*size)
	if err != nil {
		v2Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]gin.H, 0, len(list))
	for i := range list {
		last, _ := s.Repo.MessageByID(inst, list[i].LastMessageID)
		out = append(out, chatJSON(&list[i], last))
	}
	c.JSON(http.StatusOK, out)
}

func (s *Server) v2FindStatusMessage(c *gin.Context) {
	if !s.historyOn(c, true) {
		return
	}
	var req v2Where
	_ = c.ShouldBindJSON(&req)
	page, size := req.paging()
	id := req.Where.ID
	if id == "" {
		id = req.Where.Key.ID
	}
	chat := req.Where.RemoteJid
	if chat == "" {
		chat = req.Where.Key.RemoteJid
	}
	q := store.MessageQuery{Chat: s.session(c).ChatJID(c.Request.Context(), chat), ID: id, Limit: size, Offset: (page - 1) * size}
	list, _, err := s.Repo.Messages(current(c).Id, q)
	if err != nil {
		v2Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]gin.H, 0, len(list))
	for _, m := range list {
		row := gin.H{"keyId": m.ID, "remoteJid": m.Chat, "fromMe": m.FromMe, "status": m.Status, "instanceId": m.InstanceID}
		if strings.HasSuffix(m.Chat, "@g.us") {
			row["participant"] = m.Sender
		}
		out = append(out, row)
	}
	c.JSON(http.StatusOK, out)
}
