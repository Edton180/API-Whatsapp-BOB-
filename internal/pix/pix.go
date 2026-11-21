// API WhatsApp BOB - Desenvolvido por Edton Braz.

// Package pix monta o BR Code do Pix (o "copia e cola"), no padrão EMV do Banco Central.
package pix

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Params são os dados de um Pix por chave.
type Params struct {
	Key         string // chave Pix (CPF, CNPJ, telefone, e-mail ou aleatória)
	KeyType     string // cpf, cnpj, phone, email ou random (vazio = descobre pelo formato)
	Name        string // nome do recebedor (até 25 letras)
	City        string // cidade do recebedor (até 15 letras)
	AmountCents int64  // 0 = quem paga digita o valor
	TxID        string // identificador da cobrança (até 25 letras e números; vazio = ***)
}

var (
	onlyDigits = regexp.MustCompile(`\D+`)
	notTxID    = regexp.MustCompile(`[^A-Za-z0-9]+`)
)

// Code devolve o BR Code estático com a chave, o valor e o identificador informados.
func Code(p Params) (string, error) {
	key, err := NormalizeKey(p.Key, p.KeyType)
	if err != nil {
		return "", err
	}
	name := clean(p.Name, 25)
	if name == "" {
		return "", errors.New("informe o nome do recebedor do Pix")
	}
	city := clean(p.City, 15)
	if city == "" {
		city = "BRASIL"
	}
	txid := notTxID.ReplaceAllString(p.TxID, "")
	if len(txid) > 25 {
		txid = txid[:25]
	}
	if txid == "" {
		txid = "***"
	}
	if p.AmountCents < 0 {
		return "", errors.New("o valor do Pix não pode ser negativo")
	}
	out := field("00", "01") + field("26", field("00", "br.gov.bcb.pix")+field("01", key)) + field("52", "0000") + field("53", "986")
	if p.AmountCents > 0 {
		out += field("54", fmt.Sprintf("%d.%02d", p.AmountCents/100, p.AmountCents%100))
	}
	out += field("58", "BR") + field("59", name) + field("60", city) + field("62", field("05", txid)) + "6304"
	return out + fmt.Sprintf("%04X", CRC16(out)), nil
}

// NormalizeKey deixa a chave no formato que o BR Code exige (telefone com +55, documentos só com dígitos).
func NormalizeKey(key, keyType string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", errors.New("informe a chave Pix")
	}
	d := onlyDigits.ReplaceAllString(key, "")
	switch strings.ToLower(strings.TrimSpace(keyType)) {
	case "cpf", "cnpj":
		if len(d) != 11 && len(d) != 14 {
			return "", errors.New("chave Pix de CPF ou CNPJ inválida")
		}
		return d, nil
	case "phone", "telefone", "celular":
		if len(d) < 10 {
			return "", errors.New("chave Pix de telefone inválida")
		}
		if len(d) <= 11 {
			d = "55" + d
		}
		return "+" + d, nil
	case "email", "e-mail":
		return strings.ToLower(key), nil
	case "random", "evp", "aleatoria", "aleatória":
		return strings.ToLower(key), nil
	}
	// sem tipo: descobre pelo formato
	switch {
	case strings.Contains(key, "@"):
		return strings.ToLower(key), nil
	case strings.HasPrefix(key, "+"):
		return "+" + d, nil
	case len(key) == 36 && strings.Count(key, "-") == 4:
		return strings.ToLower(key), nil
	case len(d) == 11 || len(d) == 14:
		return d, nil
	}
	if len(key) > 77 {
		return "", errors.New("chave Pix muito longa")
	}
	return key, nil
}

// CRC16 calcula o CRC16-CCITT (inicial 0xFFFF, polinômio 0x1021) usado no BR Code.
func CRC16(s string) uint16 {
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

// Valid confere o CRC de um BR Code recebido de fora (ex.: gerado por um gateway).
func Valid(code string) bool {
	code = strings.TrimSpace(code)
	if len(code) < 30 || !strings.HasPrefix(code, "0002") {
		return false
	}
	return strings.EqualFold(code[len(code)-4:], fmt.Sprintf("%04X", CRC16(code[:len(code)-4])))
}

func field(id, v string) string { return fmt.Sprintf("%s%02d%s", id, len(v), v) }

var accents = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a", "é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i", "ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u", "ç", "c", "ñ", "n",
	"Á", "A", "À", "A", "Â", "A", "Ã", "A", "Ä", "A", "É", "E", "È", "E", "Ê", "E", "Ë", "E",
	"Í", "I", "Ì", "I", "Î", "I", "Ï", "I", "Ó", "O", "Ò", "O", "Ô", "O", "Õ", "O", "Ö", "O",
	"Ú", "U", "Ù", "U", "Û", "U", "Ü", "U", "Ç", "C", "Ñ", "N",
)

// clean tira acentos e símbolos (o BR Code só aceita ASCII) e corta no tamanho máximo.
func clean(s string, max int) string {
	s = strings.ToUpper(accents.Replace(strings.TrimSpace(s)))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == ' ' || r == '.' || r == '-' {
			b.WriteRune(r)
		}
	}
	out := strings.Join(strings.Fields(b.String()), " ")
	if len(out) > max {
		out = strings.TrimSpace(out[:max])
	}
	return out
}
