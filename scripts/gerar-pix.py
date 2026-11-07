#!/usr/bin/env python3
"""Gera o Pix Copia e Cola (BR Code estático, padrão EMV do Banco Central).

Uso: python3 scripts/gerar-pix.py [chave] [nome] [cidade] [valor]
Para gerar a imagem: pip install segno && python3 scripts/gerar-pix.py --png docs/pix-qrcode.png
"""
import sys


def campo(i, v):
    return f"{i}{len(v):02d}{v}"


def crc16(s):
    c = 0xFFFF
    for b in s.encode():
        c ^= b << 8
        for _ in range(8):
            c = ((c << 1) ^ 0x1021) if c & 0x8000 else c << 1
            c &= 0xFFFF
    return f"{c:04X}"


def br_code(chave, nome, cidade, valor=""):
    conta = campo("00", "br.gov.bcb.pix") + campo("01", chave)
    p = campo("00", "01") + campo("26", conta) + campo("52", "0000") + campo("53", "986")
    if valor:
        p += campo("54", valor)
    p += campo("58", "BR") + campo("59", nome[:25].upper()) + campo("60", cidade[:15].upper())
    p += campo("62", campo("05", "***")) + "6304"
    return p + crc16(p)


if __name__ == "__main__":
    args = [a for a in sys.argv[1:] if not a.startswith("--")]
    chave, nome, cidade, valor = (args + ["04646947285", "EDTON KAIK BRAZ", "TUCURUI", ""][len(args):])[:4]
    codigo = br_code(chave, nome, cidade, valor)
    print(codigo)
    if "--png" in sys.argv:
        import segno
        destino = sys.argv[sys.argv.index("--png") + 1]
        segno.make(codigo, error="m").save(destino, scale=8, border=4)
