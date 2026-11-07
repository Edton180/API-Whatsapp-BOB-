// API WhatsApp BOB - baseada na Evolution Go (Evolution Foundation), Apache License 2.0.
// Modificações: Edton Braz. Figurinhas aceitam URL ou base64, GIF/PNG/JPG/WebP, e saem em
// WebP 512x512 (formato exigido pelo WhatsApp). WebP já pronto (inclusive animado) vai como está.

package send_service

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/chai2010/webp"
	xdraw "golang.org/x/image/draw"
)

const stickerSize = 512
const stickerMaxDownload = 10 << 20

var stickerHTTP = &http.Client{Timeout: 30 * time.Second}

// loadStickerSource aceita http(s)://, data:image/...;base64,... ou base64 puro.
func loadStickerSource(src string) ([]byte, error) {
	src = strings.TrimSpace(src)
	switch {
	case strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://"):
		resp, err := stickerHTTP.Get(src)
		if err != nil {
			return nil, fmt.Errorf("não foi possível baixar a figurinha: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("não foi possível baixar a figurinha: HTTP %d", resp.StatusCode)
		}
		return io.ReadAll(io.LimitReader(resp.Body, stickerMaxDownload))
	case strings.HasPrefix(src, "data:"):
		if i := strings.Index(src, ","); i > 0 {
			src = src[i+1:]
		}
		fallthrough
	default:
		b, err := base64.StdEncoding.DecodeString(src)
		if err != nil {
			return nil, fmt.Errorf("figurinha inválida: envie uma URL ou a imagem em base64")
		}
		return b, nil
	}
}

func isWebP(b []byte) bool {
	return len(b) > 12 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "WEBP"
}

// prepareSticker devolve o WebP pronto para enviar.
func prepareSticker(src string) ([]byte, error) {
	raw, err := loadStickerSource(src)
	if err != nil {
		return nil, err
	}
	if isWebP(raw) {
		if cfg, err := webp.DecodeConfig(bytes.NewReader(raw)); err != nil || (cfg.Width == stickerSize && cfg.Height == stickerSize) || bytes.Contains(raw[:min(len(raw), 64)], []byte("ANIM")) {
			// Já no tamanho certo, ou animado (redimensionar perderia a animação)
			return raw, nil
		}
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("formato de imagem não suportado para figurinha: %v", err)
	}
	b := img.Bounds()
	scale := float64(stickerSize) / float64(max(b.Dx(), b.Dy()))
	w, h := int(float64(b.Dx())*scale), int(float64(b.Dy())*scale)
	canvas := image.NewNRGBA(image.Rect(0, 0, stickerSize, stickerSize))
	xdraw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.Transparent), image.Point{}, xdraw.Src)
	off := image.Pt((stickerSize-w)/2, (stickerSize-h)/2)
	xdraw.CatmullRom.Scale(canvas, image.Rectangle{Min: off, Max: off.Add(image.Pt(w, h))}, img, b, xdraw.Over, nil)

	for _, q := range []float32{85, 70, 55} {
		var buf bytes.Buffer
		if err := webp.Encode(&buf, canvas, &webp.Options{Lossless: false, Quality: q}); err != nil {
			return nil, fmt.Errorf("falha ao gerar o WebP: %v", err)
		}
		if buf.Len() <= 500*1024 || q == 55 {
			return buf.Bytes(), nil
		}
	}
	return nil, fmt.Errorf("falha ao gerar a figurinha")
}
