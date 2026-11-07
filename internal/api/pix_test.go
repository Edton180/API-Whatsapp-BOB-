// API WhatsApp BOB - Desenvolvido por Edton Braz.

package api

import (
	"fmt"
	"os"
	"strings"
	"testing"

	qrcode "github.com/skip2/go-qrcode"
)

func TestPixPayload(t *testing.T) {
	p := PixPayload()
	if !strings.HasPrefix(p, "000201") || !strings.Contains(p, "br.gov.bcb.pix") {
		t.Fatalf("BR Code inválido: %s", p)
	}
	body, crc := p[:len(p)-4], p[len(p)-4:]
	if want := fmt.Sprintf("%04X", PixCRC16(body)); crc != want {
		t.Fatalf("CRC %s, esperado %s", crc, want)
	}
	if !strings.Contains(p, "6006BRASIL") {
		t.Fatal("o campo de cidade do BR Code deve ser genérico (BRASIL)")
	}
	// CRC conferido com o valor de referência do algoritmo CCITT-FALSE
	if PixCRC16("123456789") != 0x29B1 {
		t.Fatal("CRC16 fora do padrão")
	}
	// BOB_GERAR_PIX=1 go test ./internal/api -run Pix  regenera as imagens do QR Code
	if os.Getenv("BOB_GERAR_PIX") == "1" {
		for _, out := range []string{"../../docs/imagens/pix-qrcode.png", "../panel/static/pix.png"} {
			if err := qrcode.WriteFile(p, qrcode.Medium, 512, out); err != nil {
				t.Fatal(err)
			}
		}
	}
}
