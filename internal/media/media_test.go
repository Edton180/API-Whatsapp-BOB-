// API WhatsApp BOB - Desenvolvido por Edton Braz.

package media

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "golang.org/x/image/webp"
)

func samplePNG(w, h int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.NRGBA{uint8(x), uint8(y), 120, 255})
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func TestLoad(t *testing.T) {
	data := samplePNG(10, 10)
	b64 := base64.StdEncoding.EncodeToString(data)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(data) }))
	defer srv.Close()
	for _, src := range []string{b64, "data:image/png;base64," + b64, srv.URL} {
		got, mime, err := Load(context.Background(), src)
		if err != nil || !bytes.Equal(got, data) || mime != "image/png" {
			t.Fatalf("Load(%.30q): %v %s", src, err, mime)
		}
	}
	if _, _, err := Load(context.Background(), "não é arquivo!"); err == nil {
		t.Fatal("texto inválido deveria falhar")
	}
}

func TestSticker(t *testing.T) {
	out, animated, err := Sticker(samplePNG(300, 150))
	if err != nil || animated {
		t.Fatal(err)
	}
	img, format, err := image.Decode(bytes.NewReader(out))
	if err != nil || format != "webp" {
		t.Fatalf("figurinha deve ser WebP: %v %s", err, format)
	}
	if b := img.Bounds(); b.Dx() != 512 || b.Dy() != 512 {
		t.Fatalf("figurinha deve ter 512x512, tem %v", b)
	}
	if len(out) > 500*1024 {
		t.Fatal("figurinha acima de 500 KB")
	}
	anim := append([]byte("RIFF\x00\x00\x00\x00WEBPVP8X\x0a\x00\x00\x00\x02\x00\x00\x00"), []byte("ANIM....")...)
	if got, a, _ := Sticker(anim); !a || !bytes.Equal(got, anim) {
		t.Fatal("WebP animado deve passar direto")
	}
}

func TestThumbnailAndJPEG(t *testing.T) {
	if len(Thumbnail(samplePNG(400, 200))) == 0 {
		t.Fatal("miniatura vazia")
	}
	j, err := ImageJPEG(samplePNG(800, 600))
	if err != nil || Detect(j) != "image/jpeg" {
		t.Fatal("foto de perfil deve virar JPEG")
	}
}

func TestVoiceNote(t *testing.T) {
	if !HasFFmpeg() {
		t.Skip("ffmpeg não instalado")
	}
	// 1 s de silêncio em WAV (PCM 8 kHz mono)
	var wav bytes.Buffer
	n := 8000
	wav.WriteString("RIFF")
	le := func(v uint32, size int) {
		for i := 0; i < size; i++ {
			wav.WriteByte(byte(v >> (8 * i)))
		}
	}
	le(uint32(36+n), 4)
	wav.WriteString("WAVEfmt ")
	le(16, 4)
	le(1, 2)
	le(1, 2)
	le(8000, 4)
	le(8000, 4)
	le(1, 2)
	le(8, 2)
	wav.WriteString("data")
	le(uint32(n), 4)
	wav.Write(bytes.Repeat([]byte{128}, n))
	out, secs, err := VoiceNote(context.Background(), wav.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if string(out[:4]) != "OggS" || secs != 1 {
		t.Fatalf("esperava OGG de 1 s, veio %q %d s", out[:4], secs)
	}
}
