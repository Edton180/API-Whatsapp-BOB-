// API WhatsApp BOB - Desenvolvido por Edton Braz.

package store

import (
	"testing"
	"time"
)

func newCallRepo(t *testing.T) *Repo {
	t.Helper()
	db, err := Open("", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &Repo{DB: db}
}

func TestCallLifecycle(t *testing.T) {
	r := newCallRepo(t)
	t0 := time.Unix(1_800_000_000, 0)
	c, err := r.CreateCall(&Call{InstanceID: "i1", CallID: "AA", RemoteJid: "5511999990000@s.whatsapp.net", StartedAt: t0})
	if err != nil || c.Status != CallRinging {
		t.Fatalf("criar: %v %+v", err, c)
	}
	// Evento repetido não duplica nem muda a chamada.
	if again, err := r.CreateCall(&Call{InstanceID: "i1", CallID: "AA", RemoteJid: "outro"}); err != nil || again.ID != c.ID || again.RemoteJid != c.RemoteJid {
		t.Fatalf("idempotência: %v %+v", err, again)
	}
	if c, _ = r.MarkCallAnswered("i1", "AA", t0.Add(5*time.Second)); c.Status != CallConnected {
		t.Fatalf("atender: %+v", c)
	}
	// Atender de novo (outro atendente) não mexe na hora em que foi atendida.
	if c2, _ := r.MarkCallAnswered("i1", "AA", t0.Add(9*time.Second)); !c2.AnsweredAt.Equal(*c.AnsweredAt) {
		t.Fatalf("atendida duas vezes: %v", c2.AnsweredAt)
	}
	c, err = r.FinishCall("i1", "AA", CallEnded, "hangup", t0.Add(65*time.Second))
	if err != nil || c.Status != CallEnded || c.DurationSec != 60 {
		t.Fatalf("encerrar: %v %+v", err, c)
	}
	if c, _ = r.FinishCall("i1", "AA", CallRejected, "x", t0.Add(90*time.Second)); c.Status != CallEnded || c.DurationSec != 60 {
		t.Fatalf("chamada encerrada mudou: %+v", c)
	}
	if _, err := r.CallByID("i2", "AA"); err != ErrCallNotFound {
		t.Fatalf("outra instância enxergou a chamada: %v", err)
	}
}

func TestCallMissedAndElsewhere(t *testing.T) {
	r := newCallRepo(t)
	now := time.Now()
	for _, id := range []string{"M", "E", "O"} {
		if _, err := r.CreateCall(&Call{InstanceID: "i1", CallID: id, FromMe: id == "O"}); err != nil {
			t.Fatal(err)
		}
	}
	if c, _ := r.FinishCall("i1", "M", CallEnded, "timeout", now); c.Status != CallMissed {
		t.Fatalf("não atendida deveria ser perdida: %+v", c)
	}
	if c, _ := r.FinishCall("i1", "E", CallEnded, ReasonElsewhere, now); c.Status != CallEnded {
		t.Fatalf("atendida no celular não é perdida: %+v", c)
	}
	if c, _ := r.FinishCall("i1", "O", CallEnded, "timeout", now); c.Status != CallEnded {
		t.Fatalf("ligação feita sem resposta fica encerrada: %+v", c)
	}
	missed, total, err := r.Calls("i1", CallQuery{Status: CallMissed})
	if err != nil || total != 1 || missed[0].CallID != "M" {
		t.Fatalf("filtro: %v %d %+v", err, total, missed)
	}
}

func TestCloseOpenCalls(t *testing.T) {
	r := newCallRepo(t)
	_, _ = r.CreateCall(&Call{InstanceID: "i1", CallID: "R"})
	_, _ = r.CreateCall(&Call{InstanceID: "i1", CallID: "C"})
	_, _ = r.MarkCallAnswered("i1", "C", time.Now().Add(-time.Minute))
	closed, err := r.CloseOpenCalls(time.Now())
	if err != nil || len(closed) != 2 {
		t.Fatalf("%v %+v", err, closed)
	}
	for _, c := range closed {
		if !c.Final() || c.Reason != "server_restart" {
			t.Fatalf("%+v", c)
		}
	}
}
