// API WhatsApp BOB - Desenvolvido por Edton Braz.

package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func pn(n string) types.JID  { return types.NewJID(n, types.DefaultUserServer) }
func lid(n string) types.JID { return types.NewJID(n, types.HiddenUserServer) }

func TestPickMembers(t *testing.T) {
	self, selfLID := pn("5511900000000"), lid("900")
	src := []types.GroupParticipant{
		{JID: lid("900"), LID: lid("900"), PhoneNumber: pn("5511900000000"), IsAdmin: true}, // o próprio número
		{JID: pn("5511911111111"), PhoneNumber: pn("5511911111111")},
		{JID: lid("222"), LID: lid("222"), PhoneNumber: pn("5511922222222"), IsAdmin: true},
		{JID: lid("333"), LID: lid("333")},                           // número oculto
		{JID: pn("5511911111111"), PhoneNumber: pn("5511911111111")}, // repetido (outro grupo)
		{JID: pn("5511944444444"), PhoneNumber: pn("5511944444444")},
	}
	dst := []types.GroupParticipant{{JID: lid("x"), LID: lid("x"), PhoneNumber: pn("5511944444444")}, {JID: lid("333"), LID: lid("333")}}

	got := pickMembers(src, nil, self, selfLID, false, 0)
	if len(got) != 4 {
		t.Fatalf("sem exclusões: esperava 4, veio %d", len(got))
	}
	got = pickMembers(src, dst, self, selfLID, false, 0)
	if len(got) != 2 || got[0].JID.User != "5511911111111" || got[1].JID.User != "222" {
		t.Fatalf("quem já está no destino não entra: %+v", got)
	}
	got = pickMembers(src, dst, self, selfLID, true, 0)
	if len(got) != 1 || got[0].JID.User != "5511911111111" {
		t.Fatalf("skipAdmins: %+v", got)
	}
	if got = pickMembers(src, nil, self, selfLID, false, 2); len(got) != 2 {
		t.Fatalf("limit: %d", len(got))
	}
}

func TestClampDelay(t *testing.T) {
	if clampDelay(0, groupSendDefault, groupSendMinDelay) != groupSendDefault {
		t.Fatal("sem valor usa o padrão")
	}
	if clampDelay(1, groupSendDefault, groupSendMinDelay) != groupSendMinDelay {
		t.Fatal("abaixo do mínimo sobe para o mínimo")
	}
	if clampDelay(20, groupAddDefaultDelay, groupAddMinDelay) != 20*time.Second {
		t.Fatal("valor válido é mantido")
	}
	if clampDelay(99999, groupAddDefaultDelay, groupAddMinDelay) != 10*time.Minute {
		t.Fatal("teto de 10 minutos")
	}
}

func newTestJob(n int) *groupJob {
	j := &groupJob{GroupJob: GroupJob{ID: newJobID(), Status: "running", StartedAt: time.Now(), Total: n}}
	for i := 0; i < n; i++ {
		j.Items = append(j.Items, GroupJobItem{Status: "pending"})
	}
	return j
}

func TestRunGroupJobBatchesAndResults(t *testing.T) {
	j := newTestJob(5)
	var batches [][2]int
	runGroupJob(context.Background(), j, 2, time.Millisecond, func(_ context.Context, a, b int) ([]GroupJobItem, error) {
		batches = append(batches, [2]int{a, b})
		out := make([]GroupJobItem, b-a)
		for i := range out {
			out[i].Status = "ok"
			if a+i == 3 {
				out[i] = GroupJobItem{Status: "failed", Error: addErrorText(403)}
			}
			if a+i == 4 {
				out[i] = GroupJobItem{Status: "skipped", Error: addErrorText(409)}
			}
		}
		return out, nil
	})
	if len(batches) != 3 || batches[2] != [2]int{4, 5} {
		t.Fatalf("lotes: %v", batches)
	}
	s := j.snapshot(true)
	if s.Status != "done" || s.Processed != 5 || s.OK != 3 || s.Failed != 1 || s.Skipped != 1 || s.FinishedAt == nil {
		t.Fatalf("resultado: %+v", s)
	}
	if s.Items[3].Error == "" || s.Items[4].Status != "skipped" {
		t.Fatalf("itens: %+v", s.Items)
	}
}

func TestRunGroupJobStopsOnErrorAndCancel(t *testing.T) {
	j := newTestJob(4)
	runGroupJob(context.Background(), j, 1, time.Millisecond, func(_ context.Context, a, _ int) ([]GroupJobItem, error) {
		if a == 1 {
			return nil, errors.New("desconectado")
		}
		return []GroupJobItem{{Status: "ok"}}, nil
	})
	if s := j.snapshot(true); s.Status != "failed" || s.Error != "desconectado" || s.Processed != 1 || s.Items[2].Status != "pending" {
		t.Fatalf("erro deve parar a tarefa: %+v", s)
	}

	j = newTestJob(50)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runGroupJob(ctx, j, 1, 20*time.Millisecond, func(_ context.Context, _, _ int) ([]GroupJobItem, error) {
			return []GroupJobItem{{Status: "ok"}}, nil
		})
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cancelar deve interromper a espera")
	}
	if s := j.snapshot(false); s.Status != "cancelled" || s.Processed == 0 || s.Processed >= 50 {
		t.Fatalf("cancelada: %+v", s)
	}
}

func TestGroupJobsAreScopedToInstance(t *testing.T) {
	j := newTestJob(1)
	j.InstanceID = "inst-a"
	groupJobs.Store(j.ID, j)
	defer groupJobs.Delete(j.ID)
	a, b := &Session{ID: "inst-a"}, &Session{ID: "inst-b"}
	if _, ok := a.GroupJobGet(j.ID); !ok {
		t.Fatal("a instância dona vê a tarefa")
	}
	if _, ok := b.GroupJobGet(j.ID); ok {
		t.Fatal("outra instância não pode ver a tarefa")
	}
	if _, ok := b.GroupJobCancel(j.ID); ok {
		t.Fatal("outra instância não pode cancelar")
	}
	if got, ok := a.GroupJobCancel(j.ID); !ok || got.Status != "cancelled" {
		t.Fatal("a dona cancela")
	}
	if len(a.GroupJobList()) != 1 || len(b.GroupJobList()) != 0 {
		t.Fatal("lista por instância")
	}
}
