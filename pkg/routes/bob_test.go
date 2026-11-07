package routes

import (
	"fmt"
	"strings"
	"testing"
)

func TestBobPixPayloadValido(t *testing.T) {
	p := BobPixPayload
	if !strings.HasPrefix(p, "000201") || !strings.Contains(p, "0014br.gov.bcb.pix") {
		t.Fatalf("cabeçalho/GUI inválido: %s", p)
	}
	// Percorre os campos TLV de nível superior e confere os tamanhos.
	for i := 0; i < len(p)-8; {
		var n int
		fmt.Sscanf(p[i+2:i+4], "%d", &n)
		i += 4 + n
		if i > len(p) {
			t.Fatalf("tamanho de campo inválido na posição %d", i)
		}
	}
	body, crc := p[:len(p)-4], p[len(p)-4:]
	if got := fmt.Sprintf("%04X", PixCRC16(body)); got != crc {
		t.Fatalf("CRC esperado %s, recebido %s", got, crc)
	}
}
