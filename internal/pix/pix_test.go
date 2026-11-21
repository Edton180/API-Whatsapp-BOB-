// API WhatsApp BOB - Desenvolvido por Edton Braz.

package pix

import (
	"strings"
	"testing"
)

func TestCode(t *testing.T) {
	code, err := Code(Params{Key: "(11) 99999-8888", KeyType: "phone", Name: "João da Silva Conceição Ltda", City: "São Paulo", AmountCents: 4990, TxID: "PED-123"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"br.gov.bcb.pix", "0114+5511999998888", "540549.90", "5925JOAO DA SILVA CONCEICAO", "6009SAO PAULO", "0506PED123"} {
		if !strings.Contains(code, want) {
			t.Fatalf("BR Code sem %q: %s", want, code)
		}
	}
	if !Valid(code) {
		t.Fatal("CRC do BR Code não confere")
	}
	if Valid(code[:len(code)-1]+"0") && Valid(code[:len(code)-1]+"1") {
		t.Fatal("BR Code adulterado não pode ser válido")
	}
	// sem valor: quem paga digita
	free, _ := Code(Params{Key: "04646947285", Name: "Fulano"})
	if strings.Contains(free, "5404") || !strings.Contains(free, "6006BRASIL") || !strings.Contains(free, "0503***") {
		t.Fatalf("BR Code de valor livre errado: %s", free)
	}
	if _, err := Code(Params{Key: "", Name: "x"}); err == nil {
		t.Fatal("chave vazia deve dar erro")
	}
	if _, err := Code(Params{Key: "123", KeyType: "cpf", Name: "x"}); err == nil {
		t.Fatal("CPF inválido deve dar erro")
	}
	if CRC16("123456789") != 0x29B1 {
		t.Fatal("CRC16 fora do padrão")
	}
}

func TestNormalizeKey(t *testing.T) {
	cases := map[[2]string]string{
		{"046.469.472-85", ""}:                       "04646947285",
		{"Contato@Loja.com", ""}:                     "contato@loja.com",
		{"11999998888", "phone"}:                     "+5511999998888",
		{"+55 11 99999-8888", ""}:                    "+5511999998888",
		{"123E4567-E89B-12D3-A456-426614174000", ""}: "123e4567-e89b-12d3-a456-426614174000",
		{"12.345.678/0001-90", "cnpj"}:               "12345678000190",
	}
	for in, want := range cases {
		got, err := NormalizeKey(in[0], in[1])
		if err != nil || got != want {
			t.Fatalf("NormalizeKey(%q,%q) = %q, %v; esperado %q", in[0], in[1], got, err, want)
		}
	}
}
