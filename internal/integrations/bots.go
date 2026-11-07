package integrations

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	instance_model "github.com/edton180/api-whatsapp-bob/internal/store"
)

// Reply é uma resposta do bot para enviar ao contato.
type Reply struct {
	Text      string
	MediaURL  string
	MediaType string // image, video, audio, document
}

func (t BotTrigger) matches(text string) bool {
	switch strings.ToLower(t.TriggerType) {
	case "none":
		return false
	case "keyword":
		a, b := strings.ToLower(strings.TrimSpace(text)), strings.ToLower(strings.TrimSpace(t.TriggerValue))
		if b == "" {
			return false
		}
		switch strings.ToLower(t.TriggerOperator) {
		case "equals":
			return a == b
		case "startswith":
			return strings.HasPrefix(a, b)
		default:
			return strings.Contains(a, b)
		}
	default:
		return true
	}
}

func (m *Manager) bot(inst *instance_model.Instance, it *Integration, in *Incoming) error {
	var trig BotTrigger
	_ = json.Unmarshal([]byte(it.Config), &trig)
	if in.FromMe && !trig.ListeningFromMe {
		return nil
	}
	if in.IsGroup && boolOr(trig.IgnoreGroups, true) {
		return nil
	}
	if strings.TrimSpace(in.Text) == "" {
		return nil
	}

	sess, _ := m.Store.GetSession(inst.Id, it.Kind, in.RemoteJid)
	if sess != nil && trig.ExpireMinutes > 0 && time.Since(sess.UpdatedAt) > time.Duration(trig.ExpireMinutes)*time.Minute {
		_ = m.Store.DeleteSession(inst.Id, it.Kind, in.RemoteJid)
		sess = nil
	}
	if trig.KeywordFinish != "" && strings.EqualFold(strings.TrimSpace(in.Text), strings.TrimSpace(trig.KeywordFinish)) {
		return m.Store.DeleteSession(inst.Id, it.Kind, in.RemoteJid)
	}
	if sess == nil && !trig.matches(in.Text) {
		return nil
	}

	var replies []Reply
	var sessionID string
	var err error
	switch it.Kind {
	case KindTypebot:
		replies, sessionID, err = m.typebot(it, in, sess)
	case KindN8N:
		replies, sessionID, err = m.n8n(inst, it, in)
	}
	if err != nil {
		// Sessão do Typebot vencida no servidor: começa de novo uma vez.
		if it.Kind == KindTypebot && sess != nil {
			_ = m.Store.DeleteSession(inst.Id, it.Kind, in.RemoteJid)
			replies, sessionID, err = m.typebot(it, in, nil)
		}
		if err != nil {
			return err
		}
	}
	if sessionID != "" {
		_ = m.Store.SaveSession(&Session{InstanceId: inst.Id, Kind: it.Kind, RemoteJid: in.RemoteJid, SessionId: sessionID, UpdatedAt: time.Now()})
	}
	return m.deliver(inst, in.RemoteJid, replies, trig.DelayMessage)
}

func (m *Manager) deliver(inst *instance_model.Instance, to string, replies []Reply, delay int) error {
	for i, r := range replies {
		if i > 0 && delay > 0 && !m.Sync {
			time.Sleep(time.Duration(delay) * time.Millisecond)
		}
		var err error
		if r.MediaURL != "" {
			err = m.sendMedia(inst, to, r.MediaURL, r.MediaType, r.Text)
		} else if strings.TrimSpace(r.Text) != "" {
			err = m.sendText(inst, to, r.Text)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// ---------- Typebot ----------

type typebotResponse struct {
	SessionId string `json:"sessionId"`
	Messages  []struct {
		Type    string          `json:"type"`
		Content json.RawMessage `json:"content"`
	} `json:"messages"`
	Input *struct {
		Type    string `json:"type"`
		Options *struct {
			Labels *struct {
				Placeholder string `json:"placeholder"`
			} `json:"labels"`
		} `json:"options"`
		Items []struct {
			Content string `json:"content"`
		} `json:"items"`
	} `json:"input"`
}

func (m *Manager) typebot(it *Integration, in *Incoming, sess *Session) ([]Reply, string, error) {
	cfg, err := decode[TypebotConfig](it)
	if err != nil {
		return nil, "", err
	}
	base := strings.TrimRight(cfg.Url, "/")
	headers := map[string]string{}
	if cfg.ApiToken != "" {
		headers["Authorization"] = "Bearer " + cfg.ApiToken
	}
	msg := map[string]any{"type": "text", "text": in.Text}
	var res typebotResponse
	if sess == nil {
		body := map[string]any{
			"message": msg,
			"prefilledVariables": map[string]any{
				"remoteJid": in.RemoteJid, "pushName": in.PushName, "number": in.Number,
			},
		}
		err = m.call("POST", fmt.Sprintf("%s/api/v1/typebots/%s/startChat", base, cfg.Typebot), headers, body, &res)
	} else {
		err = m.call("POST", fmt.Sprintf("%s/api/v1/sessions/%s/continueChat", base, sess.SessionId), headers, map[string]any{"message": msg}, &res)
		if res.SessionId == "" {
			res.SessionId = sess.SessionId
		}
	}
	if err != nil {
		return nil, "", err
	}
	var out []Reply
	for _, msg := range res.Messages {
		switch msg.Type {
		case "text":
			var c struct {
				RichText []any  `json:"richText"`
				Markdown string `json:"markdown"`
			}
			_ = json.Unmarshal(msg.Content, &c)
			text := c.Markdown
			if text == "" {
				text = richText(c.RichText)
			}
			out = append(out, Reply{Text: text})
		case "image", "video", "audio", "embed":
			var c struct {
				Url string `json:"url"`
			}
			_ = json.Unmarshal(msg.Content, &c)
			t := msg.Type
			if t == "embed" {
				out = append(out, Reply{Text: c.Url})
				continue
			}
			out = append(out, Reply{MediaURL: c.Url, MediaType: t})
		}
	}
	// Botões e listas do Typebot viram opções numeradas (funciona em qualquer WhatsApp).
	if res.Input != nil && len(res.Input.Items) > 0 {
		var b strings.Builder
		for i, item := range res.Input.Items {
			fmt.Fprintf(&b, "%d. %s\n", i+1, item.Content)
		}
		out = append(out, Reply{Text: strings.TrimSpace(b.String())})
	}
	return out, res.SessionId, nil
}

// richText converte o formato Slate do Typebot em texto com a formatação do WhatsApp.
func richText(nodes []any) string {
	var lines []string
	for _, n := range nodes {
		lines = append(lines, richNode(n))
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func richNode(n any) string {
	m, ok := n.(map[string]any)
	if !ok {
		return ""
	}
	if t, ok := m["text"].(string); ok {
		if t == "" {
			return ""
		}
		if b, _ := m["bold"].(bool); b {
			t = "*" + t + "*"
		}
		if i, _ := m["italic"].(bool); i {
			t = "_" + t + "_"
		}
		if s, _ := m["strikethrough"].(bool); s {
			t = "~" + t + "~"
		}
		return t
	}
	var b strings.Builder
	children, _ := m["children"].([]any)
	for _, c := range children {
		b.WriteString(richNode(c))
	}
	if url, _ := m["url"].(string); url != "" && m["type"] == "a" {
		return b.String() + " (" + url + ")"
	}
	return b.String()
}

// ---------- N8N ----------

func (m *Manager) n8n(inst *instance_model.Instance, it *Integration, in *Incoming) ([]Reply, string, error) {
	cfg, err := decode[N8NConfig](it)
	if err != nil {
		return nil, "", err
	}
	headers := map[string]string{}
	if cfg.BasicUser != "" {
		headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(cfg.BasicUser+":"+cfg.BasicPass))
	}
	// Formato de corpo já usado pelos fluxos de WhatsApp no N8N, para os fluxos existentes funcionarem.
	body := map[string]any{
		"chatInput":    in.Text,
		"sessionId":    in.RemoteJid,
		"remoteJid":    in.RemoteJid,
		"number":       in.Number,
		"pushName":     in.PushName,
		"fromMe":       in.FromMe,
		"messageId":    in.ID,
		"mediaType":    in.MediaType,
		"instanceName": inst.Name,
		"instanceId":   inst.Id,
		"apiKey":       inst.Token,
	}
	var raw json.RawMessage
	if err := m.call("POST", cfg.WebhookUrl, headers, body, &raw); err != nil {
		return nil, "", err
	}
	return n8nReplies(raw), in.RemoteJid, nil
}

// n8nReplies aceita {output}, {text}, {reply}, {answer}, {message} ou uma lista deles; uma linha em branco dupla (três quebras) separa mensagens.
func n8nReplies(raw json.RawMessage) []Reply {
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) != nil {
		list = []json.RawMessage{raw}
	}
	var out []Reply
	for _, item := range list {
		var s string
		if json.Unmarshal(item, &s) == nil {
			out = append(out, splitText(s)...)
			continue
		}
		var o map[string]any
		if json.Unmarshal(item, &o) != nil {
			continue
		}
		for _, k := range []string{"output", "text", "reply", "answer", "message"} {
			if v, ok := o[k].(string); ok && v != "" {
				out = append(out, splitText(v)...)
				break
			}
		}
		if u, ok := o["mediaUrl"].(string); ok && u != "" {
			t, _ := o["mediaType"].(string)
			if t == "" {
				t = "image"
			}
			c, _ := o["caption"].(string)
			out = append(out, Reply{MediaURL: u, MediaType: t, Text: c})
		}
	}
	return out
}

func splitText(s string) []Reply {
	var out []Reply
	for _, p := range strings.Split(s, "\n\n\n") {
		if strings.TrimSpace(p) != "" {
			out = append(out, Reply{Text: strings.TrimSpace(p)})
		}
	}
	return out
}
