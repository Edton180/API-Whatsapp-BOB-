// API WhatsApp BOB - Desenvolvido por Edton Braz.

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	"github.com/edton180/api-whatsapp-bob/internal/media"
)

// Quoted identifica a mensagem respondida.
type Quoted struct {
	MessageID   string `json:"messageId"`
	Participant string `json:"participant"`
}

// SendOptions são os campos comuns a todos os envios.
type SendOptions struct {
	ID           string   `json:"id"`
	Delay        int      `json:"delay"` // milissegundos mostrando "digitando..." antes de enviar
	Quoted       *Quoted  `json:"quoted"`
	MentionedJID []string `json:"mentionedJid"`
	MentionAll   bool     `json:"mentionAll"`

	extraNodes *[]waBinary.Node
}

// ResolveJID transforma número, JID ou id de grupo no destino do WhatsApp. Com
// CHECK_USER_EXISTS ligado, confirma o número no WhatsApp (corrige o nono dígito do Brasil).
func (s *Session) ResolveJID(ctx context.Context, number string) (types.JID, error) {
	number = strings.TrimSpace(number)
	if number == "" {
		return types.EmptyJID, errors.New("informe o número de destino")
	}
	if strings.Contains(number, "@") {
		jid, err := types.ParseJID(number)
		if err != nil {
			return types.EmptyJID, fmt.Errorf("JID inválido: %s", number)
		}
		return jid, nil
	}
	if strings.Contains(number, "-") && len(digits(number)) > 15 {
		return types.NewJID(digits(number), types.GroupServer), nil
	}
	num := digits(number)
	if len(num) > 15 && strings.HasPrefix(num, "120363") {
		return types.NewJID(num, types.GroupServer), nil
	}
	if len(num) < 8 {
		return types.EmptyJID, fmt.Errorf("número inválido: %s", number)
	}
	if !s.e.Cfg.CheckUserExists {
		return types.NewJID(num, types.DefaultUserServer), nil
	}
	if v, ok := s.jids.Load(num); ok {
		return v.(types.JID), nil
	}
	c, err := s.Client()
	if err != nil {
		return types.EmptyJID, err
	}
	res, err := c.IsOnWhatsApp(ctx, []string{"+" + num})
	if err != nil {
		return types.NewJID(num, types.DefaultUserServer), nil
	}
	for _, r := range res {
		if r.IsIn {
			s.jids.Store(num, r.JID)
			return r.JID, nil
		}
	}
	return types.EmptyJID, fmt.Errorf("o número %s não tem WhatsApp", num)
}

// Send envia a mensagem já montada e devolve {Info, Message}.
func (s *Session) Send(ctx context.Context, to types.JID, msg *waE2E.Message, opt SendOptions) (map[string]any, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	if opt.Delay > 0 && to.Server != types.BroadcastServer {
		d := time.Duration(min(opt.Delay, 25000)) * time.Millisecond
		_ = c.SendChatPresence(ctx, to, types.ChatPresenceComposing, types.ChatPresenceMediaText)
		time.Sleep(d)
		_ = c.SendChatPresence(ctx, to, types.ChatPresencePaused, types.ChatPresenceMediaText)
	}
	if err := s.decorate(ctx, c, to, msg, opt); err != nil {
		return nil, err
	}
	extra := whatsmeow.SendRequestExtra{ID: types.MessageID(opt.ID)}
	if opt.extraNodes != nil {
		extra.AdditionalNodes = opt.extraNodes
	}
	resp, err := c.SendMessage(ctx, to, msg, extra)
	if err != nil {
		s.logf("ERROR", "envio para %s falhou: %v", to.User, err)
		return nil, fmt.Errorf("o WhatsApp não aceitou a mensagem: %w", err)
	}
	s.recent.put(resp.ID, msg)
	own := types.EmptyJID
	if c.Store.ID != nil {
		own = c.Store.ID.ToNonAD()
	}
	info := map[string]any{
		"ID": resp.ID, "Chat": to.String(), "Sender": own.String(), "IsFromMe": true,
		"IsGroup": to.Server == types.GroupServer, "Timestamp": resp.Timestamp, "ServerID": resp.ServerID,
	}
	out := map[string]any{"Info": info, "Message": msg}
	s.emit("SendMessage", out)
	return out, nil
}

// decorate aplica resposta (quoted) e menções no conteúdo da mensagem.
func (s *Session) decorate(ctx context.Context, c *whatsmeow.Client, to types.JID, msg *waE2E.Message, opt SendOptions) error {
	if opt.Quoted == nil && len(opt.MentionedJID) == 0 && !opt.MentionAll {
		return nil
	}
	// texto simples vira texto estendido para aceitar contexto
	if msg.Conversation != nil {
		msg.ExtendedTextMessage = &waE2E.ExtendedTextMessage{Text: msg.Conversation}
		msg.Conversation = nil
	}
	ci := contextInfo(msg)
	if ci == nil {
		return nil
	}
	if q := opt.Quoted; q != nil && q.MessageID != "" {
		ci.StanzaID = proto.String(q.MessageID)
		participant := q.Participant
		if participant == "" {
			participant = to.ToNonAD().String()
		} else if !strings.Contains(participant, "@") {
			participant = digits(participant) + "@" + types.DefaultUserServer
		}
		ci.Participant = proto.String(participant)
		if quoted := s.recent.get(q.MessageID); quoted != nil {
			ci.QuotedMessage = quoted
		} else {
			ci.QuotedMessage = &waE2E.Message{Conversation: proto.String("")}
		}
	}
	mentions := []string{}
	for _, m := range opt.MentionedJID {
		if !strings.Contains(m, "@") {
			m = digits(m) + "@" + types.DefaultUserServer
		}
		mentions = append(mentions, m)
	}
	if opt.MentionAll && to.Server == types.GroupServer {
		if g, err := c.GetGroupInfo(ctx, to); err == nil {
			for _, p := range g.Participants {
				mentions = append(mentions, p.JID.String())
			}
		}
	}
	if len(mentions) > 0 {
		ci.MentionedJID = mentions
	}
	return nil
}

// contextInfo devolve o ContextInfo do tipo de mensagem presente (criando se preciso).
func contextInfo(m *waE2E.Message) *waE2E.ContextInfo {
	ensure := func(p **waE2E.ContextInfo) *waE2E.ContextInfo {
		if *p == nil {
			*p = &waE2E.ContextInfo{}
		}
		return *p
	}
	switch {
	case m.ExtendedTextMessage != nil:
		return ensure(&m.ExtendedTextMessage.ContextInfo)
	case m.ImageMessage != nil:
		return ensure(&m.ImageMessage.ContextInfo)
	case m.VideoMessage != nil:
		return ensure(&m.VideoMessage.ContextInfo)
	case m.AudioMessage != nil:
		return ensure(&m.AudioMessage.ContextInfo)
	case m.DocumentMessage != nil:
		return ensure(&m.DocumentMessage.ContextInfo)
	case m.StickerMessage != nil:
		return ensure(&m.StickerMessage.ContextInfo)
	case m.LocationMessage != nil:
		return ensure(&m.LocationMessage.ContextInfo)
	case m.ContactMessage != nil:
		return ensure(&m.ContactMessage.ContextInfo)
	case m.ListMessage != nil:
		return ensure(&m.ListMessage.ContextInfo)
	case m.ViewOnceMessage.GetMessage().GetInteractiveMessage() != nil:
		return ensure(&m.ViewOnceMessage.Message.InteractiveMessage.ContextInfo)
	}
	return nil
}

// ---------------------------------------------------------------- montagem das mensagens

// Text monta uma mensagem de texto.
func Text(text string) *waE2E.Message {
	return &waE2E.Message{Conversation: proto.String(text)}
}

// Link monta um texto com pré-visualização de link.
func Link(text, link, title, description string) *waE2E.Message {
	if !strings.Contains(text, link) {
		text = strings.TrimSpace(text + "\n" + link)
	}
	return &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
		Text: proto.String(text), MatchedText: proto.String(link),
		Title: proto.String(title), Description: proto.String(description),
	}}
}

// MediaInput descreve um arquivo a enviar.
type MediaInput struct {
	Data     []byte
	Type     string // image, video, audio, ptt, document
	Mime     string
	Caption  string
	Filename string
}

// Media envia o arquivo para os servidores do WhatsApp e monta a mensagem.
func (s *Session) Media(ctx context.Context, in MediaInput) (*waE2E.Message, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	if len(in.Data) == 0 {
		return nil, errors.New("arquivo vazio")
	}
	if in.Mime == "" || in.Mime == "application/octet-stream" {
		in.Mime = media.Detect(in.Data)
	}
	typ := strings.ToLower(in.Type)
	if typ == "" {
		switch {
		case strings.HasPrefix(in.Mime, "image/"):
			typ = "image"
		case strings.HasPrefix(in.Mime, "video/"):
			typ = "video"
		case strings.HasPrefix(in.Mime, "audio/"):
			typ = "audio"
		default:
			typ = "document"
		}
	}
	switch typ {
	case "image":
		up, err := c.Upload(ctx, in.Data, whatsmeow.MediaImage)
		if err != nil {
			return nil, fmt.Errorf("upload da imagem: %w", err)
		}
		return &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
			URL: &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey, FileEncSHA256: up.FileEncSHA256,
			FileSHA256: up.FileSHA256, FileLength: proto.Uint64(up.FileLength), Mimetype: proto.String(in.Mime),
			Caption: optional(in.Caption), JPEGThumbnail: media.Thumbnail(in.Data),
		}}, nil
	case "video":
		up, err := c.Upload(ctx, in.Data, whatsmeow.MediaVideo)
		if err != nil {
			return nil, fmt.Errorf("upload do vídeo: %w", err)
		}
		return &waE2E.Message{VideoMessage: &waE2E.VideoMessage{
			URL: &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey, FileEncSHA256: up.FileEncSHA256,
			FileSHA256: up.FileSHA256, FileLength: proto.Uint64(up.FileLength), Mimetype: proto.String(in.Mime),
			Caption: optional(in.Caption),
		}}, nil
	case "audio", "ptt", "voice":
		data, mime, seconds, ptt := in.Data, in.Mime, uint32(0), true
		if conv, secs, err := media.VoiceNote(ctx, in.Data); err == nil {
			data, mime, seconds = conv, "audio/ogg; codecs=opus", secs
		} else if !strings.Contains(in.Mime, "ogg") {
			ptt = false // sem ffmpeg: vai como arquivo de áudio comum
		} else {
			mime = "audio/ogg; codecs=opus"
		}
		up, err := c.Upload(ctx, data, whatsmeow.MediaAudio)
		if err != nil {
			return nil, fmt.Errorf("upload do áudio: %w", err)
		}
		return &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
			URL: &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey, FileEncSHA256: up.FileEncSHA256,
			FileSHA256: up.FileSHA256, FileLength: proto.Uint64(up.FileLength), Mimetype: proto.String(mime),
			PTT: proto.Bool(ptt), Seconds: proto.Uint32(seconds),
		}}, nil
	default:
		name := in.Filename
		if name == "" {
			name = "arquivo" + media.Extension(in.Mime)
		}
		up, err := c.Upload(ctx, in.Data, whatsmeow.MediaDocument)
		if err != nil {
			return nil, fmt.Errorf("upload do documento: %w", err)
		}
		return &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
			URL: &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey, FileEncSHA256: up.FileEncSHA256,
			FileSHA256: up.FileSHA256, FileLength: proto.Uint64(up.FileLength), Mimetype: proto.String(in.Mime),
			FileName: proto.String(name), Title: proto.String(name), Caption: optional(in.Caption),
		}}, nil
	}
}

// Sticker converte e envia a figurinha para os servidores do WhatsApp.
func (s *Session) Sticker(ctx context.Context, data []byte) (*waE2E.Message, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	webp, animated, err := media.Sticker(data)
	if err != nil {
		return nil, err
	}
	up, err := c.Upload(ctx, webp, whatsmeow.MediaImage)
	if err != nil {
		return nil, fmt.Errorf("upload da figurinha: %w", err)
	}
	return &waE2E.Message{StickerMessage: &waE2E.StickerMessage{
		URL: &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey, FileEncSHA256: up.FileEncSHA256,
		FileSHA256: up.FileSHA256, FileLength: proto.Uint64(up.FileLength), Mimetype: proto.String("image/webp"),
		Width: proto.Uint32(512), Height: proto.Uint32(512), IsAnimated: proto.Bool(animated),
	}}, nil
}

// Location monta uma localização.
func Location(lat, lng float64, name, address string) *waE2E.Message {
	return &waE2E.Message{LocationMessage: &waE2E.LocationMessage{
		DegreesLatitude: proto.Float64(lat), DegreesLongitude: proto.Float64(lng),
		Name: optional(name), Address: optional(address),
	}}
}

// VCard são os dados de um contato compartilhado.
type VCard struct {
	FullName     string `json:"fullName"`
	Phone        string `json:"phone"`
	Organization string `json:"organization"`
	Email        string `json:"email"`
	URL          string `json:"url"`
}

// Contact monta um cartão de contato.
func Contact(v VCard) *waE2E.Message {
	num := digits(v.Phone)
	var b strings.Builder
	b.WriteString("BEGIN:VCARD\nVERSION:3.0\n")
	b.WriteString("FN:" + v.FullName + "\n")
	if v.Organization != "" {
		b.WriteString("ORG:" + v.Organization + ";\n")
	}
	if v.Email != "" {
		b.WriteString("EMAIL:" + v.Email + "\n")
	}
	if v.URL != "" {
		b.WriteString("URL:" + v.URL + "\n")
	}
	b.WriteString(fmt.Sprintf("TEL;type=CELL;type=VOICE;waid=%s:+%s\n", num, num))
	b.WriteString("END:VCARD")
	return &waE2E.Message{ContactMessage: &waE2E.ContactMessage{DisplayName: proto.String(v.FullName), Vcard: proto.String(b.String())}}
}

// Poll monta uma enquete.
func (s *Session) Poll(question string, options []string, max int) (*waE2E.Message, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	if len(options) < 2 {
		return nil, errors.New("a enquete precisa de pelo menos 2 opções")
	}
	if len(options) > 12 {
		options = options[:12]
	}
	if max < 0 || max > len(options) {
		max = 1
	}
	return c.BuildPollCreation(question, options, max), nil
}

// Button é um botão de mensagem interativa.
type Button struct {
	Type        string `json:"type"` // reply, url, call, copy, pix
	DisplayText string `json:"displayText"`
	ID          string `json:"id"`
	URL         string `json:"url"`
	PhoneNumber string `json:"phoneNumber"`
	CopyCode    string `json:"copyCode"`
	Currency    string `json:"currency"`
	Name        string `json:"name"`
	KeyType     string `json:"keyType"`
	Key         string `json:"key"`
}

// Buttons monta uma mensagem com botões (resposta, link, ligar, copiar e Pix).
func Buttons(title, description, footer string, buttons []Button) (*waE2E.Message, *[]waBinary.Node, error) {
	if len(buttons) == 0 {
		return nil, nil, errors.New("informe ao menos um botão")
	}
	flow := []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{}
	for i, b := range buttons {
		var name string
		params := map[string]any{}
		switch strings.ToLower(b.Type) {
		case "url":
			name, params = "cta_url", map[string]any{"display_text": b.DisplayText, "url": b.URL, "merchant_url": b.URL}
		case "call":
			name, params = "cta_call", map[string]any{"display_text": b.DisplayText, "phone_number": b.PhoneNumber}
		case "copy":
			name, params = "cta_copy", map[string]any{"display_text": b.DisplayText, "copy_code": b.CopyCode, "id": fmt.Sprintf("copy_%d", i)}
		case "pix":
			cur := b.Currency
			if cur == "" {
				cur = "BRL"
			}
			name = "payment_info"
			params = map[string]any{
				"currency": cur, "total_amount": map[string]any{"value": 0, "offset": 100},
				"reference_id": fmt.Sprintf("BOB%d", time.Now().UnixNano()%1e10), "type": "physical-goods", "order": map[string]any{"status": "pending", "subtotal": map[string]any{"value": 0, "offset": 100}, "order_type": "ORDER", "items": []any{}},
				"payment_settings": []any{map[string]any{"type": "pix_static_code", "pix_static_code": map[string]any{
					"merchant_name": b.Name, "key": b.Key, "key_type": strings.ToUpper(b.KeyType),
				}}},
				"share_payment_status": false,
			}
		default:
			id := b.ID
			if id == "" {
				id = fmt.Sprintf("btn_%d", i)
			}
			name, params = "quick_reply", map[string]any{"display_text": b.DisplayText, "id": id}
		}
		js, _ := json.Marshal(params)
		flow = append(flow, &waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{Name: proto.String(name), ButtonParamsJSON: proto.String(string(js))})
	}
	im := &waE2E.InteractiveMessage{
		Body:   &waE2E.InteractiveMessage_Body{Text: proto.String(description)},
		Footer: &waE2E.InteractiveMessage_Footer{Text: proto.String(footer)},
		InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
			Buttons: flow, MessageVersion: proto.Int32(1),
		}},
	}
	if title != "" {
		im.Header = &waE2E.InteractiveMessage_Header{Title: proto.String(title), HasMediaAttachment: proto.Bool(false)}
	}
	msg := &waE2E.Message{ViewOnceMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{
		MessageContextInfo: &waE2E.MessageContextInfo{DeviceListMetadata: &waE2E.DeviceListMetadata{}, DeviceListMetadataVersion: proto.Int32(2)},
		InteractiveMessage: im,
	}}}
	nodes := []waBinary.Node{{
		Tag: "biz",
		Content: []waBinary.Node{{
			Tag:   "interactive",
			Attrs: waBinary.Attrs{"type": "native_flow", "v": "1"},
			Content: []waBinary.Node{{
				Tag:   "native_flow",
				Attrs: waBinary.Attrs{"v": "9", "name": "mixed"},
			}},
		}},
	}}
	return msg, &nodes, nil
}

// ListSection é uma seção da lista de opções.
type ListSection struct {
	Title string    `json:"title"`
	Rows  []ListRow `json:"rows"`
}

// ListRow é uma opção da lista.
type ListRow struct {
	RowID       string `json:"rowId"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// List monta uma lista de opções.
func List(title, description, buttonText, footer string, sections []ListSection) (*waE2E.Message, error) {
	if len(sections) == 0 {
		return nil, errors.New("informe ao menos uma seção com opções")
	}
	if buttonText == "" {
		buttonText = "Ver opções"
	}
	ss := []*waE2E.ListMessage_Section{}
	for si, sec := range sections {
		rows := []*waE2E.ListMessage_Row{}
		for ri, r := range sec.Rows {
			id := r.RowID
			if id == "" {
				id = fmt.Sprintf("row_%d_%d", si, ri)
			}
			rows = append(rows, &waE2E.ListMessage_Row{RowID: proto.String(id), Title: proto.String(r.Title), Description: optional(r.Description)})
		}
		ss = append(ss, &waE2E.ListMessage_Section{Title: proto.String(sec.Title), Rows: rows})
	}
	return &waE2E.Message{ListMessage: &waE2E.ListMessage{
		Title: proto.String(title), Description: proto.String(description), ButtonText: proto.String(buttonText),
		FooterText: optional(footer), ListType: waE2E.ListMessage_SINGLE_SELECT.Enum(), Sections: ss,
	}}, nil
}

// SendWithNodes envia mensagens que precisam de nós extras (botões).
func (s *Session) SendWithNodes(ctx context.Context, to types.JID, msg *waE2E.Message, nodes *[]waBinary.Node, opt SendOptions) (map[string]any, error) {
	opt.extraNodes = nodes
	return s.Send(ctx, to, msg, opt)
}

// StatusText publica um status de texto.
func StatusText(text string, background uint32, font int32) *waE2E.Message {
	if background == 0 {
		background = 0xFF1DB954
	}
	return &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
		Text: proto.String(text), BackgroundArgb: proto.Uint32(background), TextArgb: proto.Uint32(0xFFFFFFFF),
		Font: waE2E.ExtendedTextMessage_FontType(font).Enum(),
	}}
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return proto.String(s)
}
