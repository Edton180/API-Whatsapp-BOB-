// API WhatsApp BOB - Desenvolvido por Edton Braz.

package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/edton180/api-whatsapp-bob/internal/pix"
	"github.com/edton180/api-whatsapp-bob/internal/store"
)

func chargeInstance(t *testing.T, h http.Handler, name string) string {
	t.Helper()
	r := call(t, h, "POST", "/instance/create", key, map[string]any{"name": name})
	if r.Code != 200 {
		t.Fatalf("create: %s", r.Raw)
	}
	return data(r)["token"].(string)
}

func TestChargeWithInstancePix(t *testing.T) {
	h := newServer(t)
	token := chargeInstance(t, h, "loja")
	no := false

	// sem chave Pix e sem link: não há como cobrar
	if r := call(t, h, "POST", "/charge/create", token, map[string]any{"number": "5511999998888", "amount": 10, "send": no}); r.Code != 400 || !strings.Contains(r.Raw, "chave Pix") {
		t.Fatalf("sem chave Pix deve pedir a configuração: %d %s", r.Code, r.Raw)
	}
	if r := call(t, h, "PUT", "/charge/settings", token, map[string]any{"pixKey": "123", "pixKeyType": "cpf", "pixName": "Loja"}); r.Code != 400 {
		t.Fatalf("chave inválida deve ser recusada: %s", r.Raw)
	}
	r := call(t, h, "PUT", "/charge/settings", token, map[string]any{"pixKey": "contato@loja.com", "pixKeyType": "email", "pixName": "Loja do Zé", "pixCity": "Manaus"})
	if r.Code != 200 || data(r)["pixKey"] != "contato@loja.com" {
		t.Fatalf("settings: %s", r.Raw)
	}
	if r := call(t, h, "GET", "/charge/settings", token, nil); data(r)["pixName"] != "Loja do Zé" {
		t.Fatalf("settings salvos: %s", r.Raw)
	}

	for _, bad := range []map[string]any{
		{"number": "5511999998888", "send": no},
		{"number": "5511999998888", "amount": 0, "send": no},
		{"amount": 10, "send": no},
		{"number": "5511999998888", "amount": 10, "dueDate": "20/11/2026", "send": no},
		{"number": "5511999998888", "amount": 10, "pixCode": "isto não é pix", "send": no},
		{"number": "5511999998888", "amount": 10, "paymentUrl": "javascript:alert(1)", "send": no},
	} {
		if r := call(t, h, "POST", "/charge/create", token, bad); r.Code != 400 {
			t.Fatalf("pedido inválido aceito: %v -> %d %s", bad, r.Code, r.Raw)
		}
	}

	r = call(t, h, "POST", "/charge/create", token, map[string]any{
		"number": "5511999998888", "customerName": "Maria", "description": "Mensalidade de novembro", "amount": 49.9,
		"dueDate": "2020-01-10", "reference": "fatura-1", "metadata": map[string]any{"pedido": 7}, "send": no,
	})
	ch := data(r)
	if r.Code != 200 || ch["status"] != "pending" || ch["amountCents"] != float64(4990) || ch["created"] != true || ch["sent"] != false || ch["overdue"] != true {
		t.Fatalf("cobrança criada: %s", r.Raw)
	}
	code, _ := ch["pixCode"].(string)
	if !pix.Valid(code) || !strings.Contains(code, "540549.90") || !strings.Contains(code, "contato@loja.com") {
		t.Fatalf("Pix gerado pela BOB: %s", code)
	}
	id := ch["id"].(string)

	// mesma referência não cria outra cobrança
	again := call(t, h, "POST", "/charge/create", token, map[string]any{"number": "5511999998888", "amount": 99, "reference": "fatura-1", "send": no})
	if data(again)["id"] != id || data(again)["created"] != false {
		t.Fatalf("referência repetida deve devolver a mesma cobrança: %s", again.Raw)
	}

	// consulta pelo id e pela referência, lista e QR Code
	if r := call(t, h, "GET", "/charge/fatura-1", token, nil); data(r)["id"] != id {
		t.Fatalf("consulta pela referência: %s", r.Raw)
	}
	if r := call(t, h, "GET", "/charge/list?status=overdue", token, nil); data(r)["total"] != float64(1) {
		t.Fatalf("lista de vencidas: %s", r.Raw)
	}
	if r := call(t, h, "GET", "/charge/list?status=paid", token, nil); data(r)["total"] != float64(0) {
		t.Fatalf("lista de pagas: %s", r.Raw)
	}
	if r := call(t, h, "GET", "/charge/"+id+"/qrcode", token, nil); r.Code != 200 || !strings.HasPrefix(r.Raw, "\x89PNG") {
		t.Fatalf("QR Code: %d", r.Code)
	}

	// instância desconectada: enviar falha com 409 e nada novo é gravado
	if r := call(t, h, "POST", "/charge/create", token, map[string]any{"number": "5511999998888", "amount": 5}); r.Code != http.StatusConflict {
		t.Fatalf("envio sem conexão deve dar 409: %d %s", r.Code, r.Raw)
	}
	if r := call(t, h, "POST", "/charge/"+id+"/send", token, nil); r.Code != http.StatusConflict {
		t.Fatalf("reenvio sem conexão deve dar 409: %d %s", r.Code, r.Raw)
	}
	if r := call(t, h, "GET", "/charge/list", token, nil); data(r)["total"] != float64(1) {
		t.Fatalf("a cobrança que não saiu não pode ficar gravada: %s", r.Raw)
	}

	// baixa (uma vez só) e bloqueios depois de paga
	paid := call(t, h, "POST", "/charge/"+id+"/pay", token, map[string]any{"notify": no})
	if data(paid)["status"] != "paid" || data(paid)["changed"] != true || data(paid)["paidAt"] == nil {
		t.Fatalf("baixa: %s", paid.Raw)
	}
	if r := call(t, h, "POST", "/charge/fatura-1/pay", token, map[string]any{"notify": no}); data(r)["changed"] != false || data(r)["status"] != "paid" {
		t.Fatalf("baixa repetida: %s", r.Raw)
	}
	if r := call(t, h, "POST", "/charge/"+id+"/cancel", token, nil); r.Code != http.StatusConflict {
		t.Fatalf("cobrança paga não pode ser cancelada: %s", r.Raw)
	}
	if r := call(t, h, "POST", "/charge/"+id+"/send", token, nil); r.Code != http.StatusConflict {
		t.Fatalf("cobrança paga não pode ser reenviada: %s", r.Raw)
	}
}

func TestChargeFromGatewayAndIsolation(t *testing.T) {
	h := newServer(t)
	a, b := chargeInstance(t, h, "a"), chargeInstance(t, h, "b")
	no := false
	gatewayPix, _ := pix.Code(pix.Params{Key: "12345678000190", KeyType: "cnpj", Name: "Gateway", AmountCents: 15000, TxID: "abc"})

	// Pix e link vindos de um gateway: não precisa de chave Pix na instância
	r := call(t, h, "POST", "/charge/create", a, map[string]any{
		"number": "5592988887777", "amountCents": 15000, "pixCode": gatewayPix, "paymentUrl": "https://pagar.exemplo/abc", "reference": "pay_1", "send": no,
	})
	ch := data(r)
	if r.Code != 200 || ch["pixCode"] != gatewayPix || ch["paymentUrl"] != "https://pagar.exemplo/abc" || ch["amount"] != float64(150) {
		t.Fatalf("cobrança de gateway: %s", r.Raw)
	}
	id := ch["id"].(string)

	// outra instância não enxerga nem mexe
	for _, p := range []string{"/charge/" + id, "/charge/pay_1"} {
		if r := call(t, h, "GET", p, b, nil); r.Code != http.StatusNotFound {
			t.Fatalf("%s de outra instância deve dar 404: %d", p, r.Code)
		}
	}
	if r := call(t, h, "POST", "/charge/"+id+"/pay", b, nil); r.Code != http.StatusNotFound {
		t.Fatal("outra instância não pode dar baixa")
	}
	if r := call(t, h, "GET", "/charge/list", b, nil); data(r)["total"] != float64(0) {
		t.Fatal("lista de outra instância deve vir vazia")
	}
	if r := call(t, h, "GET", "/charge/list", "", nil); r.Code != http.StatusUnauthorized {
		t.Fatal("sem token deve dar 401")
	}
	// chave global com o cabeçalho instance também vale
	if r := call(t, h, "GET", "/charge/"+id, key, nil, "instance", "a"); data(r)["id"] != id {
		t.Fatalf("chave global: %s", r.Raw)
	}

	// cancelar e bloquear a baixa depois
	if r := call(t, h, "POST", "/charge/"+id+"/cancel", a, nil); data(r)["status"] != "cancelled" {
		t.Fatalf("cancelar: %s", r.Raw)
	}
	if r := call(t, h, "POST", "/charge/"+id+"/pay", a, nil); r.Code != http.StatusConflict {
		t.Fatal("cobrança cancelada não pode ser paga")
	}
}

func TestChargeTexts(t *testing.T) {
	ch := &store.Charge{CustomerName: "Maria", Description: "Mensalidade", AmountCents: 123456789, Currency: "BRL", DueDate: "2026-11-20", PixCode: "x", PaymentURL: "https://p.test/1"}
	text := ChargeText(ch, "")
	for _, want := range []string{"Olá, Maria!", "Mensalidade", "R$ 1.234.567,89", "20/11/2026", "copia e cola", "https://p.test/1"} {
		if !strings.Contains(text, want) {
			t.Fatalf("texto da cobrança sem %q:\n%s", want, text)
		}
	}
	custom := ChargeText(ch, "Oi {nome}, sua {descricao} de {valor} vence em {vencimento}.")
	if !strings.HasPrefix(custom, "Oi Maria, sua Mensalidade de R$ 1.234.567,89 vence em 20/11/2026.") {
		t.Fatalf("texto personalizado: %s", custom)
	}
	if got := ReceiptText(ch, ""); !strings.Contains(got, "Pagamento confirmado") || !strings.Contains(got, "R$ 1.234.567,89") {
		t.Fatalf("recibo: %s", got)
	}
	if Money(5, "BRL") != "R$ 0,05" || Money(100000, "USD") != "USD 1.000,00" {
		t.Fatal("formato de valor")
	}
}
