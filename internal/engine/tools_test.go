// API WhatsApp BOB - Desenvolvido por Edton Braz.

package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func user(n string) types.JID  { return types.NewJID(n, types.DefaultUserServer) }
func group(n string) types.JID { return types.NewJID(n, types.GroupServer) }

func TestMergeMembersJuntaGruposSemRepetir(t *testing.T) {
	g1 := &types.GroupInfo{
		JID:          group("120363001"),
		GroupName:    types.GroupName{Name: "Clientes"},
		Participants: []types.GroupParticipant{{JID: user("5511900000001")}, {JID: user("5511900000002"), IsAdmin: true}, {JID: user("5511999999999")}},
	}
	g2 := &types.GroupInfo{
		JID:       group("120363002"),
		GroupName: types.GroupName{Name: "Promoções"},
		Participants: []types.GroupParticipant{
			{JID: user("5511900000002")},
			{JID: types.NewJID("77777", "lid")}, // número oculto: não dá telefone
		},
	}

	res := MergeMembers([]*types.GroupInfo{g1, g2}, "5511999999999", nil, MembersOptions{})

	if res.Total != 2 {
		t.Fatalf("esperava 2 membros (sem o próprio número e sem repetir), veio %d: %+v", res.Total, res.Members)
	}
	if res.Hidden != 1 {
		t.Fatalf("esperava 1 participante oculto, veio %d", res.Hidden)
	}
	var dois Member
	for _, m := range res.Members {
		if m.Phone == "5511900000002" {
			dois = m
		}
	}
	if len(dois.Groups) != 2 || !dois.Admin {
		t.Fatalf("quem está nos dois grupos deve listar os dois e manter o admin: %+v", dois)
	}
	if len(res.Groups) != 2 || res.Groups[0].Name != "Clientes" || res.Groups[0].Total != 3 {
		t.Fatalf("resumo dos grupos: %+v", res.Groups)
	}
}

func TestMergeMembersFiltraAdmins(t *testing.T) {
	g := &types.GroupInfo{
		JID:          group("120363003"),
		Participants: []types.GroupParticipant{{JID: user("551190000001"), IsAdmin: true}, {JID: user("551190000002")}},
	}
	if res := MergeMembers([]*types.GroupInfo{g}, "", nil, MembersOptions{OnlyAdmins: true}); res.Total != 1 || !res.Members[0].Admin {
		t.Fatalf("onlyAdmins: %+v", res.Members)
	}
	if res := MergeMembers([]*types.GroupInfo{g}, "", nil, MembersOptions{ExcludeAdmins: true}); res.Total != 1 || res.Members[0].Admin {
		t.Fatalf("excludeAdmins: %+v", res.Members)
	}
}

// bulkTest cria um gerenciador sem espera entre os itens.
func bulkTest() *Bulk {
	b := newBulk()
	b.sleep = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }
	return b
}

func waitStatus(t *testing.T, b *Bulk, inst, id, want string) *BulkJob {
	t.Helper()
	for i := 0; i < 200; i++ {
		j, err := b.Get(inst, id)
		if err != nil {
			t.Fatal(err)
		}
		if j.Status == want {
			return j
		}
		time.Sleep(10 * time.Millisecond)
	}
	j, _ := b.Get(inst, id)
	t.Fatalf("a tarefa não chegou em %s: %+v", want, j)
	return nil
}

func TestBulkContaFeitosEFalhas(t *testing.T) {
	b := bulkTest()
	job, err := b.Start("inst-1", BulkGroupAdd, "teste", []string{"1", "2", "3", "3"}, 0, func(_ context.Context, target string) (string, error) {
		if target == "2" {
			return "", BulkFail("a pessoa não aceita ser adicionada")
		}
		return "added", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if job.Total != 3 {
		t.Fatalf("o número repetido deveria sair da lista: %d", job.Total)
	}
	done := waitStatus(t, b, "inst-1", job.ID, "completed")
	if done.Done != 2 || done.Failed != 1 || done.Pending != 0 {
		t.Fatalf("contagem: %+v", done)
	}
	full, _ := b.Get("inst-1", job.ID)
	if full.Items[1].Status != "failed" || full.Items[1].Detail == "" {
		t.Fatalf("o item que falhou deve explicar o motivo: %+v", full.Items[1])
	}
}

func TestBulkPausaQuandoOWhatsAppLimita(t *testing.T) {
	b := bulkTest()
	job, err := b.Start("inst-2", BulkSend, "", []string{"1", "2"}, 0, func(_ context.Context, string2 string) (string, error) {
		return "", errors.New("rate-overlimit: espere")
	})
	if err != nil {
		t.Fatal(err)
	}
	p := waitStatus(t, b, "inst-2", job.ID, "paused")
	if p.Pending != 2 || p.Failed != 0 {
		t.Fatalf("nada pode ser gasto da lista quando o WhatsApp limita: %+v", p)
	}
	if _, err := b.Control("inst-2", job.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	c := waitStatus(t, b, "inst-2", job.ID, "cancelled")
	if c.Pending != 2 {
		t.Fatalf("cancelada: %+v", c)
	}
}

func TestBulkUmaTarefaPorTipo(t *testing.T) {
	b := bulkTest()
	step := func(ctx context.Context, _ string) (string, error) { <-ctx.Done(); return "", ctx.Err() }
	if _, err := b.Start("inst-3", BulkGroupAdd, "", []string{"1"}, 0, step); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Start("inst-3", BulkGroupAdd, "", []string{"2"}, 0, step); err == nil {
		t.Fatal("duas tarefas do mesmo tipo na mesma instância deveriam ser recusadas")
	}
	if _, err := b.Start("outra", BulkGroupAdd, "", []string{"2"}, 0, step); err != nil {
		t.Fatalf("outra instância pode ter a sua: %v", err)
	}
	b.CancelInstance("inst-3")
	b.CancelInstance("outra")
}

func TestBulkSoAceitaTiposConhecidos(t *testing.T) {
	b := bulkTest()
	if _, err := b.Start("i", "qualquer", "", []string{"1"}, 0, nil); err == nil {
		t.Fatal("tipo inválido")
	}
	if _, err := b.Start("i", BulkSend, "", nil, 0, nil); err == nil {
		t.Fatal("lista vazia")
	}
}

// Cancelar enquanto um item roda e ele volta com erro de limite: a tarefa termina cancelada, não pausada
// nem concluída.
func TestBulkCancelarDuranteLimiteContinuaCancelada(t *testing.T) {
	b := bulkTest()
	started := make(chan struct{})
	job, err := b.Start("inst-4", BulkSend, "", []string{"1", "2"}, 0, func(ctx context.Context, _ string) (string, error) {
		close(started)
		<-ctx.Done()
		return "", errors.New("rate-overlimit")
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if _, err := b.Control("inst-4", job.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	var full *BulkJob
	for i := 0; i < 200; i++ {
		if full, _ = b.Get("inst-4", job.ID); full.FinishedAt != 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if full.Status != "cancelled" || full.FinishedAt == 0 || full.Pending != 2 {
		t.Fatalf("cancelada: %+v", full)
	}
	for _, it := range full.Items {
		if it.Status != "cancelled" {
			t.Fatalf("itens que faltavam devem ficar cancelled: %+v", full.Items)
		}
	}
}
