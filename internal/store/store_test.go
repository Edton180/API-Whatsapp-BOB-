// API WhatsApp BOB - Desenvolvido por Edton Braz.

package store

import (
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Uma instalação 1.x tem a tabela "instances" com colunas a mais. A 2.0 precisa ler essas
// instâncias sem perder nada (é assim que os números continuam conectados depois de atualizar).
func TestReadsInstancesFromVersion1(t *testing.T) {
	dir := t.TempDir()
	old, err := gorm.Open(sqlite.Open("file:"+filepath.Join(dir, "bob.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := old.Exec(`CREATE TABLE instances (
		id uuid PRIMARY KEY, name text, token text UNIQUE, webhook text, rabbitmq_enable text,
		web_socket_enable text, nats_enable text, jid text, qrcode text, connected numeric, expiration integer,
		disconnect_reason text, events text, os_name text, proxy text, client_name text, created_at datetime,
		always_online numeric DEFAULT false, reject_call numeric DEFAULT false, msg_reject_call text DEFAULT '',
		read_messages numeric DEFAULT false, ignore_groups numeric DEFAULT false, ignore_status numeric DEFAULT false)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := old.Exec(`INSERT INTO instances (id, name, token, webhook, rabbitmq_enable, jid, events, reject_call, msg_reject_call, created_at)
		VALUES ('7d6f6a3e-1111-4c3b-9a55-000000000001', 'omniflow-1', 'TOKENANTIGO', 'https://crm/hook', 'enabled',
		'5511999998888:12@s.whatsapp.net', 'MESSAGE,CONNECTION', true, 'Sem ligações', CURRENT_TIMESTAMP)`).Error; err != nil {
		t.Fatal(err)
	}
	if d, _ := old.DB(); d != nil {
		d.Close()
	}

	db, err := Open("", dir)
	if err != nil {
		t.Fatal(err)
	}
	repo := &Repo{DB: db}
	inst, err := repo.ByToken("TOKENANTIGO")
	if err != nil {
		t.Fatal(err)
	}
	if inst.Name != "omniflow-1" || inst.Jid != "5511999998888:12@s.whatsapp.net" || inst.Webhook != "https://crm/hook" ||
		inst.Events != "MESSAGE,CONNECTION" || !inst.RejectCall || inst.MsgRejectCall != "Sem ligações" {
		t.Fatalf("instância antiga lida errado: %+v", inst)
	}
	if err := repo.Update(inst.Id, map[string]any{"connected": true}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(&Instance{Name: "nova"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(&Instance{Name: "nova"}); err == nil {
		t.Fatal("nome repetido deve falhar")
	}
	if got, _ := repo.ByIDOrName("omniflow-1"); got == nil || got.Id != inst.Id {
		t.Fatal("ByIDOrName")
	}
}
