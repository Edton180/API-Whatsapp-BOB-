// API WhatsApp BOB - baseada na Evolution Go (Evolution Foundation), Apache License 2.0.
// Modificações: Edton Braz.

package routes

import (
	"os"
	"strings"
)

// BobVersion lê o arquivo VERSION (o mesmo que o Docker copia para /app/VERSION).
func BobVersion() string {
	for _, p := range []string{"VERSION", "/app/VERSION"} {
		if b, err := os.ReadFile(p); err == nil {
			if v := strings.TrimSpace(string(b)); v != "" {
				return v
			}
		}
	}
	return "dev"
}

const (
	BobCreator    = "Edton Kaik Braz Da Conceição (@Edton180)"
	BobRepository = "https://github.com/Edton180/API-Whatsapp-BOB-"
	// BobPixPayload é um BR Code estático (padrão EMV do Banco Central), valor livre.
	// Gerado por scripts/gerar-pix.py; o CRC16 é validado em bob_test.go.
	BobPixPayload = "00020126330014br.gov.bcb.pix0111046469472855204000053039865802BR5915EDTON KAIK BRAZ6007TUCURUI62070503***6304DBF1"
)

// BobPixInfo devolve os dados de doação ao projeto.
func BobPixInfo() map[string]string {
	return map[string]string{
		"project":      "API WhatsApp BOB",
		"creator":      BobCreator,
		"pix_key":      "04646947285",
		"pix_key_type": "CPF",
		"copia_e_cola": BobPixPayload,
		"qr_code":      BobRepository + "/blob/main/docs/pix-qrcode.png",
	}
}

// PixCRC16 calcula o CRC16-CCITT (0xFFFF, polinômio 0x1021) usado no BR Code.
func PixCRC16(s string) uint16 {
	crc := uint16(0xFFFF)
	for i := 0; i < len(s); i++ {
		crc ^= uint16(s[i]) << 8
		for b := 0; b < 8; b++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
