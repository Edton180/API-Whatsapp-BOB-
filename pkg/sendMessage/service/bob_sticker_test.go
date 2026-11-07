package send_service

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"testing"

	"github.com/chai2010/webp"
)

func bobImage(t *testing.T, w, h int, asGif bool) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{200, 30, 30, 255})
		}
	}
	var buf bytes.Buffer
	if asGif {
		if err := gif.Encode(&buf, img, nil); err != nil {
			t.Fatal(err)
		}
	} else if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestBobStickerConverteParaWebP512(t *testing.T) {
	for _, asGif := range []bool{false, true} {
		src := "data:image/png;base64," + base64.StdEncoding.EncodeToString(bobImage(t, 800, 300, asGif))
		out, err := prepareSticker(src)
		if err != nil {
			t.Fatalf("gif=%v: %v", asGif, err)
		}
		if !isWebP(out) {
			t.Fatalf("gif=%v: saída não é WebP", asGif)
		}
		cfg, err := webp.DecodeConfig(bytes.NewReader(out))
		if err != nil || cfg.Width != 512 || cfg.Height != 512 {
			t.Fatalf("gif=%v: tamanho %dx%d (%v)", asGif, cfg.Width, cfg.Height, err)
		}
	}
}

func TestBobStickerRecusaConteudoInvalido(t *testing.T) {
	if _, err := prepareSticker("isto não é imagem"); err == nil {
		t.Fatal("esperava erro")
	}
}
