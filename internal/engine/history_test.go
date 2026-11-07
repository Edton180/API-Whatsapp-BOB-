// API WhatsApp BOB - Desenvolvido por Edton Braz.

package engine

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	"github.com/edton180/api-whatsapp-bob/internal/config"
	bobevents "github.com/edton180/api-whatsapp-bob/internal/events"
	bobstore "github.com/edton180/api-whatsapp-bob/internal/store"
)

func historySession(t *testing.T) (*Session, *bobstore.Repo) {
	t.Helper()
	dir := t.TempDir()
	db, err := bobstore.Open("", dir)
	if err != nil {
		t.Fatal(err)
	}
	repo := &bobstore.Repo{DB: db}
	e, err := New(&config.Config{DataDir: dir, StoreMessages: true}, repo, bobevents.NewDispatcher())
	if err != nil {
		t.Fatal(err)
	}
	inst := &bobstore.Instance{Name: "h"}
	if err := repo.Create(inst); err != nil {
		t.Fatal(err)
	}
	return newSession(e, inst), repo
}

func TestRecordHistory(t *testing.T) {
	s, repo := historySession(t)
	chat, _ := types.ParseJID("5511999998888@s.whatsapp.net")
	own, _ := types.ParseJID("5511000000000@s.whatsapp.net")
	in := types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, Sender: chat}, ID: "IN1", PushName: "Cliente", Timestamp: time.Unix(1000, 0)}
	s.record(in, &waE2E.Message{Conversation: proto.String("oi")}, "DELIVERY_ACK", false)
	out := types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, Sender: own, IsFromMe: true}, ID: "OUT1", Timestamp: time.Unix(1001, 0)}
	s.record(out, &waE2E.Message{ImageMessage: &waE2E.ImageMessage{Caption: proto.String("foto")}}, "SERVER_ACK", false)

	m, err := repo.MessageByID(s.ID, "OUT1")
	if err != nil || m.Type != "imageMessage" || m.Text != "foto" || !m.FromMe {
		t.Fatalf("mensagem enviada: %+v %v", m, err)
	}
	if got := s.lookup("OUT1"); got.GetImageMessage().GetCaption() != "foto" {
		t.Fatal("lookup deveria achar a mensagem no histórico")
	}

	// edição e apagar para todos mudam a original
	edit := &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
		Type: waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(), Key: &waCommon.MessageKey{ID: proto.String("IN1")},
		EditedMessage: &waE2E.Message{Conversation: proto.String("olá")},
	}}
	s.record(types.MessageInfo{MessageSource: in.MessageSource, ID: "EDIT1"}, edit, "", false)
	revoke := &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{Type: waE2E.ProtocolMessage_REVOKE.Enum(), Key: &waCommon.MessageKey{ID: proto.String("OUT1")}}}
	s.record(types.MessageInfo{MessageSource: out.MessageSource, ID: "REV1"}, revoke, "", false)
	if m, _ := repo.MessageByID(s.ID, "IN1"); m.Text != "olá" || !m.Edited {
		t.Fatalf("edição: %+v", m)
	}
	if m, _ := repo.MessageByID(s.ID, "OUT1"); !m.Deleted {
		t.Fatal("deveria estar apagada")
	}
	if _, err := repo.MessageByID(s.ID, "EDIT1"); err == nil {
		t.Fatal("a edição não é uma mensagem nova")
	}

	// recibo de leitura
	s.onReceipt(&events.Receipt{MessageSource: types.MessageSource{Chat: chat}, MessageIDs: []types.MessageID{"OUT1"}, Type: types.ReceiptTypeRead})
	if m, _ := repo.MessageByID(s.ID, "OUT1"); m.Status != "READ" {
		t.Fatalf("status: %s", m.Status)
	}

	// conversa: última mensagem, nome e não lidas
	ch, err := repo.ChatByJID(s.ID, chat.String())
	if err != nil || ch.LastMessageID != "OUT1" || ch.Name != "Cliente" || ch.Unread != 1 {
		t.Fatalf("conversa: %+v %v", ch, err)
	}
	if last := s.lastMessage(chat); last == nil || last.ID != "OUT1" || !last.FromMe {
		t.Fatalf("última: %+v", last)
	}
	s.chatChanged(chat, "read")
	s.chatChanged(chat, "archive")
	if ch, _ := repo.ChatByJID(s.ID, chat.String()); ch.Unread != 0 || !ch.Archived {
		t.Fatalf("ações na conversa: %+v", ch)
	}

	// reação não vira mensagem
	s.record(types.MessageInfo{MessageSource: in.MessageSource, ID: "R1"}, &waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{Text: proto.String("👍")}}, "", false)
	if _, err := repo.MessageByID(s.ID, "R1"); err == nil {
		t.Fatal("reação não deveria ser guardada como mensagem")
	}
}

func TestPollResults(t *testing.T) {
	s, repo := historySession(t)
	chat, _ := types.ParseJID("5511999998888@s.whatsapp.net")
	poll := &waE2E.Message{PollCreationMessageV3: &waE2E.PollCreationMessage{
		Name:    proto.String("Pizza?"),
		Options: []*waE2E.PollCreationMessage_Option{{OptionName: proto.String("Sim")}, {OptionName: proto.String("Não")}},
	}}
	s.record(types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, IsFromMe: true}, ID: "POLL1"}, poll, "SERVER_ACK", false)
	if names := PollOptions(poll); len(names) != 2 || names[1] != "Não" {
		t.Fatalf("opções: %v", names)
	}
	if h := whatsmeow.HashPollOptions([]string{"Sim"}); len(h[0]) != 32 {
		t.Fatal("hash de opção deve ter 32 bytes")
	}
	_ = repo.SavePollVote(&bobstore.PollVote{InstanceID: s.ID, PollID: "POLL1", Voter: "a", Options: `["Sim"]`})
	_ = repo.SavePollVote(&bobstore.PollVote{InstanceID: s.ID, PollID: "POLL1", Voter: "b", Options: `["Sim","Não"]`})
	res, err := s.PollResults("POLL1")
	if err != nil {
		t.Fatal(err)
	}
	opts := res["options"].([]map[string]any)
	if res["question"] != "Pizza?" || opts[0]["votes"] != 2 || opts[1]["votes"] != 1 {
		t.Fatalf("resultado: %v", res)
	}
}

func TestMessageKindAndText(t *testing.T) {
	cases := []struct {
		m          *waE2E.Message
		kind, text string
	}{
		{&waE2E.Message{Conversation: proto.String("oi")}, "conversation", "oi"},
		{&waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String("link")}, MessageContextInfo: &waE2E.MessageContextInfo{}}, "extendedTextMessage", "link"},
		{&waE2E.Message{EphemeralMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{VideoMessage: &waE2E.VideoMessage{Caption: proto.String("v")}}}}, "videoMessage", "v"},
		{&waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{FileName: proto.String("nota.pdf")}}, "documentMessage", "nota.pdf"},
	}
	for _, c := range cases {
		if k := MessageKind(c.m); k != c.kind {
			t.Errorf("tipo: %s, esperado %s", k, c.kind)
		}
		if x := MessageText(c.m); x != c.text {
			t.Errorf("texto: %s, esperado %s", x, c.text)
		}
	}
}
