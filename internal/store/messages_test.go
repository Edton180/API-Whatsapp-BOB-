// API WhatsApp BOB - Desenvolvido por Edton Braz.

package store

import "testing"

func TestMessageHistory(t *testing.T) {
	db, err := Open("", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r := &Repo{DB: db}
	const inst, chat = "i1", "5511999998888@s.whatsapp.net"
	for i, id := range []string{"A", "B", "C"} {
		m := &Message{InstanceID: inst, ID: id, Chat: chat, FromMe: id != "B", Text: id, Status: "SERVER_ACK", Timestamp: int64(100 + i)}
		if err := r.SaveMessage(m); err != nil {
			t.Fatal(err)
		}
		if err := r.TouchChat(inst, chat, "Cliente", m, !m.FromMe); err != nil {
			t.Fatal(err)
		}
	}

	// status só avança
	_ = r.UpdateStatus(inst, []string{"A"}, "READ")
	_ = r.UpdateStatus(inst, []string{"A"}, "DELIVERY_ACK")
	if m, _ := r.MessageByID(inst, "A"); m.Status != "READ" {
		t.Fatalf("status voltou: %s", m.Status)
	}
	// regravar a mensagem não rebaixa o status
	_ = r.SaveMessage(&Message{InstanceID: inst, ID: "A", Chat: chat, FromMe: true, Status: "SERVER_ACK", Timestamp: 100})
	if m, _ := r.MessageByID(inst, "A"); m.Status != "READ" {
		t.Fatalf("status rebaixado ao regravar: %s", m.Status)
	}

	_ = r.EditMessage(inst, "C", "editada", nil)
	_ = r.MarkDeleted(inst, "B")
	list, total, err := r.Messages(inst, MessageQuery{Chat: chat, Limit: 2})
	if err != nil || total != 3 || len(list) != 2 || list[0].ID != "C" || list[0].Text != "editada" || !list[0].Edited {
		t.Fatalf("lista: %d %v %v", total, list, err)
	}
	if !list[1].Deleted {
		t.Fatal("B deveria estar apagada")
	}
	fromMe := false
	if _, n, _ := r.Messages(inst, MessageQuery{Chat: chat, FromMe: &fromMe}); n != 1 {
		t.Fatalf("filtro fromMe: %d", n)
	}
	if old, _ := r.OldestMessage(inst, chat); old.ID != "A" {
		t.Fatalf("mais antiga: %s", old.ID)
	}

	ch, err := r.ChatByJID(inst, chat)
	if err != nil || ch.LastMessageID != "C" || ch.Unread != 1 || ch.Name != "Cliente" {
		t.Fatalf("conversa: %+v %v", ch, err)
	}
	chats, n, _ := r.Chats(inst, 10, 0)
	if n != 1 || len(chats) != 1 {
		t.Fatalf("conversas: %d", n)
	}

	_ = r.SavePollVote(&PollVote{InstanceID: inst, PollID: "P", Voter: "x", Options: `["Sim"]`})
	_ = r.SavePollVote(&PollVote{InstanceID: inst, PollID: "P", Voter: "x", Options: `["Não"]`})
	votes, _ := r.PollVotes(inst, "P")
	if len(votes) != 1 || votes[0].Selected()[0] != "Não" {
		t.Fatalf("voto deveria ser substituído: %v", votes)
	}

	// apagar a instância leva o histórico junto
	_ = r.Delete(inst)
	if _, n, _ := r.Messages(inst, MessageQuery{}); n != 0 {
		t.Fatal("histórico deveria ter sido apagado")
	}
}
