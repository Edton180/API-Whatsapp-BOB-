// API WhatsApp BOB - Desenvolvido por Edton Braz.

package engine

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// O webhook "Message" precisa manter o formato que os sistemas já leem: Info.ID, Info.Chat,
// Info.IsFromMe, Info.PushName e Message.conversation / extendedTextMessage.text.
func TestMessageJSONFormat(t *testing.T) {
	chat, _ := types.ParseJID("5511999998888@s.whatsapp.net")
	ev := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chat, Sender: chat, IsFromMe: false},
			ID:            "3EB0ABC", PushName: "Cliente", Timestamp: time.Unix(1700000000, 0),
		},
		Message: &waE2E.Message{Conversation: proto.String("oi")},
	}
	b, _ := json.Marshal(ev)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	info := m["Info"].(map[string]any)
	msg := m["Message"].(map[string]any)
	if info["ID"] != "3EB0ABC" || info["Chat"] != "5511999998888@s.whatsapp.net" || info["PushName"] != "Cliente" || info["IsFromMe"] != false {
		t.Fatalf("Info fora do formato: %v", info)
	}
	if msg["conversation"] != "oi" {
		t.Fatalf("Message fora do formato: %v", msg)
	}
	// e o caminho inverso (usado em /message/downloadmedia)
	var back waE2E.Message
	raw, _ := json.Marshal(msg)
	if err := json.Unmarshal(raw, &back); err != nil || back.GetConversation() != "oi" {
		t.Fatal("o JSON da mensagem deve voltar a ser mensagem")
	}
}

func TestReceiptJSONFormat(t *testing.T) {
	chat, _ := types.ParseJID("5511999998888@s.whatsapp.net")
	b, _ := json.Marshal(&events.Receipt{MessageSource: types.MessageSource{Chat: chat}, MessageIDs: []string{"A", "B"}, Type: types.ReceiptTypeRead})
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m["Type"] != "read" || len(m["MessageIDs"].([]any)) != 2 || m["Chat"] == nil {
		t.Fatalf("Receipt fora do formato: %v", m)
	}
}

func TestBuilders(t *testing.T) {
	c := Contact(VCard{FullName: "Suporte", Phone: "+55 (11) 98888-7777", Organization: "Loja"})
	if v := c.GetContactMessage().GetVcard(); !strings.Contains(v, "waid=5511988887777:+5511988887777") || !strings.Contains(v, "ORG:Loja") {
		t.Fatalf("vcard: %s", v)
	}
	msg, nodes, err := Buttons("Pedido", "Escolha", "Loja", []Button{
		{Type: "reply", DisplayText: "Sim"}, {Type: "url", DisplayText: "Site", URL: "https://x.com"},
		{Type: "copy", DisplayText: "Copiar", CopyCode: "ABC"}, {Type: "pix", Name: "Loja", Key: "a@b.com", KeyType: "email"},
	})
	if err != nil || nodes == nil || (*nodes)[0].Tag != "biz" {
		t.Fatal("botões precisam do nó biz")
	}
	flow := msg.GetViewOnceMessage().GetMessage().GetInteractiveMessage().GetNativeFlowMessage().GetButtons()
	names := []string{}
	for _, b := range flow {
		names = append(names, b.GetName())
	}
	if strings.Join(names, ",") != "quick_reply,cta_url,cta_copy,payment_info" {
		t.Fatalf("botões: %v", names)
	}
	if !strings.Contains(flow[3].GetButtonParamsJSON(), `"key_type":"EMAIL"`) {
		t.Fatal("pix sem tipo de chave")
	}
	if _, _, err := Buttons("", "", "", nil); err == nil {
		t.Fatal("sem botões deve falhar")
	}
	l, err := List("Cardápio", "Escolha", "", "", []ListSection{{Title: "Lanches", Rows: []ListRow{{Title: "X"}}}})
	if err != nil || l.GetListMessage().GetButtonText() != "Ver opções" || l.GetListMessage().GetSections()[0].GetRows()[0].GetRowID() == "" {
		t.Fatal("lista")
	}
	loc := Location(-23.5, -46.6, "Loja", "")
	if loc.GetLocationMessage().GetDegreesLatitude() != -23.5 || loc.GetLocationMessage().Address != nil {
		t.Fatal("localização")
	}
}

func TestContextInfo(t *testing.T) {
	m := Text("oi")
	m.ExtendedTextMessage = &waE2E.ExtendedTextMessage{Text: m.Conversation}
	m.Conversation = nil
	ci := contextInfo(m)
	ci.StanzaID = proto.String("X")
	if m.GetExtendedTextMessage().GetContextInfo().GetStanzaID() != "X" {
		t.Fatal("contextInfo deve apontar para a mensagem")
	}
	img := &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}
	if contextInfo(img) == nil || img.ImageMessage.ContextInfo == nil {
		t.Fatal("contextInfo da imagem")
	}
}

func TestMessageCacheAndRing(t *testing.T) {
	c := newMessageCache(2)
	c.put("a", Text("1"))
	c.put("b", Text("2"))
	c.put("c", Text("3"))
	if c.get("a") != nil || c.get("c").GetConversation() != "3" {
		t.Fatal("cache deve guardar só as últimas")
	}
	r := newRing(2)
	for i := 0; i < 5; i++ {
		r.add(LogEntry{Message: string(rune('a' + i))})
	}
	if l := r.list(); len(l) != 2 || l[1].Message != "e" {
		t.Fatal("registro deve guardar só os últimos")
	}
}

func TestUnwrapAndDigits(t *testing.T) {
	inner := &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}
	m := &waE2E.Message{EphemeralMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{ViewOnceMessage: &waE2E.FutureProofMessage{Message: inner}}}}
	if unwrap(m) != inner {
		t.Fatal("unwrap")
	}
	if digits("+55 (11) 9.8888-7777") != "5511988887777" {
		t.Fatal("digits")
	}
	if eventName(&events.PushName{}) != "PushName" {
		t.Fatal("eventName")
	}
}
