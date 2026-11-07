package integrations

import (
	"fmt"
	"net/url"
	"strings"
	"sync"

	instance_model "github.com/edton180/api-whatsapp-bob/internal/store"
)

// Conversas abertas já encontradas: instância|jid -> id da conversa no Chatwoot.
var cwConversations sync.Map

type cwClient struct {
	m   *Manager
	cfg ChatwootConfig
}

func (c cwClient) url(path string) string {
	return fmt.Sprintf("%s/api/v1/accounts/%s%s", strings.TrimRight(c.cfg.Url, "/"), c.cfg.AccountId, path)
}

func (c cwClient) do(method, path string, body, out any) error {
	return c.m.call(method, c.url(path), map[string]string{"api_access_token": c.cfg.Token}, body, out)
}

// conversation devolve a conversa aberta do contato na caixa da BOB, criando contato e conversa se preciso.
func (c cwClient) conversation(inst *instance_model.Instance, in *Incoming) (int, error) {
	key := inst.Id + "|" + in.RemoteJid
	if v, ok := cwConversations.Load(key); ok {
		return v.(int), nil
	}
	phone := "+" + in.Number
	identifier := in.RemoteJid

	type contactInbox struct {
		SourceId string `json:"source_id"`
		Inbox    struct {
			Id int `json:"id"`
		} `json:"inbox"`
	}
	type contact struct {
		Id           int            `json:"id"`
		PhoneNumber  string         `json:"phone_number"`
		Identifier   string         `json:"identifier"`
		ContactInbox []contactInbox `json:"contact_inboxes"`
	}
	var search struct {
		Payload []contact `json:"payload"`
	}
	noPhone := in.IsGroup || strings.HasSuffix(in.RemoteJid, "@lid") // grupo ou número oculto (LID)
	q := in.Number
	if noPhone {
		q = identifier
	}
	if err := c.do("GET", "/contacts/search?q="+url.QueryEscape(q), nil, &search); err != nil {
		return 0, err
	}
	var found *contact
	for i := range search.Payload {
		p := search.Payload[i]
		if p.Identifier == identifier || (!noPhone && strings.TrimPrefix(p.PhoneNumber, "+") == in.Number) {
			found = &p
			break
		}
	}
	sourceID := ""
	if found == nil {
		name := in.PushName
		if name == "" || in.FromMe {
			name = in.Number
		}
		body := map[string]any{"inbox_id": c.cfg.InboxId, "name": name, "identifier": identifier}
		if !noPhone {
			body["phone_number"] = phone
		}
		var created struct {
			Payload struct {
				Contact      contact      `json:"contact"`
				ContactInbox contactInbox `json:"contact_inbox"`
			} `json:"payload"`
		}
		if err := c.do("POST", "/contacts", body, &created); err != nil {
			return 0, err
		}
		found = &created.Payload.Contact
		sourceID = created.Payload.ContactInbox.SourceId
	} else {
		for _, ci := range found.ContactInbox {
			if ci.Inbox.Id == c.cfg.InboxId {
				sourceID = ci.SourceId
			}
		}
	}

	var convs struct {
		Payload []struct {
			Id      int    `json:"id"`
			InboxId int    `json:"inbox_id"`
			Status  string `json:"status"`
		} `json:"payload"`
	}
	if err := c.do("GET", fmt.Sprintf("/contacts/%d/conversations", found.Id), nil, &convs); err != nil {
		return 0, err
	}
	for _, cv := range convs.Payload {
		if cv.InboxId == c.cfg.InboxId && cv.Status != "resolved" {
			cwConversations.Store(key, cv.Id)
			return cv.Id, nil
		}
	}
	if sourceID == "" {
		var ci struct {
			SourceId string `json:"source_id"`
		}
		if err := c.do("POST", fmt.Sprintf("/contacts/%d/contact_inboxes", found.Id), map[string]any{"inbox_id": c.cfg.InboxId}, &ci); err != nil {
			return 0, err
		}
		sourceID = ci.SourceId
	}
	var conv struct {
		Id int `json:"id"`
	}
	if err := c.do("POST", "/conversations", map[string]any{"contact_id": found.Id, "inbox_id": c.cfg.InboxId, "source_id": sourceID, "status": "open"}, &conv); err != nil {
		return 0, err
	}
	cwConversations.Store(key, conv.Id)
	return conv.Id, nil
}

// chatwootInbound copia a mensagem do WhatsApp para o Chatwoot.
func (m *Manager) chatwootInbound(inst *instance_model.Instance, it *Integration, in *Incoming) error {
	cfg, err := decode[ChatwootConfig](it)
	if err != nil {
		return err
	}
	if in.IsGroup && boolOr(cfg.IgnoreGroups, true) {
		return nil
	}
	if in.FromMe && !boolOr(cfg.ImportFromMe, true) {
		return nil
	}
	content := in.Text
	if in.MediaType != "" {
		label := map[string]string{"image": "imagem", "video": "vídeo", "audio": "áudio", "document": "documento", "sticker": "figurinha"}[in.MediaType]
		if label == "" {
			label = in.MediaType
		}
		content = strings.TrimSpace("[" + label + "] " + content)
	}
	if content == "" {
		return nil
	}
	c := cwClient{m: m, cfg: cfg}
	convID, err := c.conversation(inst, in)
	if err != nil {
		return err
	}
	typ := "incoming"
	if in.FromMe {
		typ = "outgoing"
	}
	body := map[string]any{"content": content, "message_type": typ, "private": false, "source_id": "WAID:" + in.ID}
	err = c.do("POST", fmt.Sprintf("/conversations/%d/messages", convID), body, nil)
	if err != nil && strings.Contains(err.Error(), "HTTP 404") {
		// A conversa foi apagada no Chatwoot: procura ou cria outra.
		cwConversations.Delete(inst.Id + "|" + in.RemoteJid)
		if convID, err = c.conversation(inst, in); err == nil {
			err = c.do("POST", fmt.Sprintf("/conversations/%d/messages", convID), body, nil)
		}
	}
	return err
}

// ChatwootEvent é o corpo que o Chatwoot envia ao webhook da caixa de entrada.
type ChatwootEvent struct {
	Event       string `json:"event"`
	MessageType string `json:"message_type"`
	Private     bool   `json:"private"`
	Content     string `json:"content"`
	SourceId    string `json:"source_id"`
	Sender      struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"sender"`
	Inbox struct {
		Id int `json:"id"`
	} `json:"inbox"`
	Conversation struct {
		Id     int    `json:"id"`
		Status string `json:"status"`
		Meta   struct {
			Sender struct {
				PhoneNumber string `json:"phone_number"`
				Identifier  string `json:"identifier"`
			} `json:"sender"`
		} `json:"meta"`
	} `json:"conversation"`
	Attachments []struct {
		DataUrl  string `json:"data_url"`
		FileType string `json:"file_type"`
	} `json:"attachments"`
}

// ChatwootOutbound envia ao WhatsApp a resposta do atendente no Chatwoot.
func (m *Manager) ChatwootOutbound(inst *instance_model.Instance, it *Integration, ev ChatwootEvent) error {
	cfg, err := decode[ChatwootConfig](it)
	if err != nil {
		return err
	}
	if ev.Event == "conversation_status_changed" && ev.Conversation.Status == "resolved" {
		cwConversations.Range(func(k, v any) bool {
			if v.(int) == ev.Conversation.Id {
				cwConversations.Delete(k)
			}
			return true
		})
		return nil
	}
	if ev.Event != "message_created" || ev.MessageType != "outgoing" || ev.Private {
		return nil
	}
	if strings.HasPrefix(ev.SourceId, "WAID:") || (ev.Inbox.Id != 0 && cfg.InboxId != 0 && ev.Inbox.Id != cfg.InboxId) {
		return nil // veio do próprio WhatsApp ou de outra caixa
	}
	to := ev.Conversation.Meta.Sender.Identifier
	if !strings.Contains(to, "@") {
		to = strings.TrimPrefix(ev.Conversation.Meta.Sender.PhoneNumber, "+")
	}
	if to == "" {
		return fmt.Errorf("conversa %d sem telefone", ev.Conversation.Id)
	}
	text := ev.Content
	if cfg.SignMsg && ev.Sender.Name != "" && text != "" {
		text = "*" + ev.Sender.Name + ":*\n" + text
	}
	if len(ev.Attachments) == 0 {
		return m.sendText(inst, to, text)
	}
	for i, a := range ev.Attachments {
		caption := ""
		if i == 0 {
			caption = text
		}
		typ := a.FileType
		if typ == "file" || typ == "" {
			typ = "document"
		}
		if err := m.sendMedia(inst, to, a.DataUrl, typ, caption); err != nil {
			return err
		}
	}
	return nil
}
