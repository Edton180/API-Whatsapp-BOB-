// API WhatsApp BOB - Desenvolvido por Edton Braz.

// Package media lê arquivos (URL, base64, data URI) e prepara figurinhas, áudios e miniaturas.
package media

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/chai2010/webp"
	"github.com/gabriel-vasile/mimetype"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// MaxDownload limita o tamanho de um arquivo baixado (100 MB).
const MaxDownload = 100 << 20

var client = &http.Client{Timeout: 2 * time.Minute}

// Load aceita uma URL http(s), um data URI ou base64 puro e devolve os bytes e o tipo.
func Load(ctx context.Context, src string) ([]byte, string, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil, "", errors.New("arquivo não informado")
	}
	var data []byte
	switch {
	case strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://"):
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
		if err != nil {
			return nil, "", err
		}
		req.Header.Set("User-Agent", "API-WhatsApp-BOB")
		res, err := client.Do(req)
		if err != nil {
			return nil, "", fmt.Errorf("não consegui baixar o arquivo: %w", err)
		}
		defer res.Body.Close()
		if res.StatusCode >= 400 {
			return nil, "", fmt.Errorf("o endereço do arquivo respondeu %d", res.StatusCode)
		}
		data, err = io.ReadAll(io.LimitReader(res.Body, MaxDownload+1))
		if err != nil {
			return nil, "", err
		}
		if len(data) > MaxDownload {
			return nil, "", errors.New("arquivo maior que 100 MB")
		}
	case strings.HasPrefix(src, "data:"):
		_, b64, ok := strings.Cut(src, ",")
		if !ok {
			return nil, "", errors.New("data URI inválido")
		}
		d, err := decodeB64(b64)
		if err != nil {
			return nil, "", err
		}
		data = d
	default:
		d, err := decodeB64(src)
		if err != nil {
			return nil, "", errors.New("informe uma URL http(s), um data URI ou um base64 válido")
		}
		data = d
	}
	return data, Detect(data), nil
}

func decodeB64(s string) ([]byte, error) {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' {
			return -1
		}
		return r
	}, s)
	if d, err := base64.StdEncoding.DecodeString(s); err == nil {
		return d, nil
	}
	return base64.RawStdEncoding.DecodeString(strings.TrimRight(s, "="))
}

// Detect devolve o tipo MIME pelo conteúdo.
func Detect(data []byte) string {
	mt := mimetype.Detect(data).String()
	if i := strings.Index(mt, ";"); i > 0 {
		mt = mt[:i]
	}
	return mt
}

// Extension sugere uma extensão para o tipo MIME.
func Extension(mime string) string {
	if m := mimetype.Lookup(mime); m != nil {
		return m.Extension()
	}
	return ""
}

// IsAnimatedWebP diz se o WebP tem animação (chunk ANIM).
func IsAnimatedWebP(data []byte) bool {
	return len(data) > 30 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP" && bytes.Contains(data[:min(len(data), 4096)], []byte("ANIM"))
}

// Sticker converte PNG, JPG, GIF (1º quadro) ou WebP para WebP 512x512 com fundo transparente,
// até 500 KB, como o WhatsApp exige. WebP animado passa direto.
func Sticker(data []byte) ([]byte, bool, error) {
	if IsAnimatedWebP(data) {
		return data, true, nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, false, fmt.Errorf("imagem da figurinha inválida: %w", err)
	}
	canvas := image.NewNRGBA(image.Rect(0, 0, 512, 512))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.Transparent), image.Point{}, draw.Src)
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return nil, false, errors.New("imagem vazia")
	}
	scale := 512.0 / float64(max(w, h))
	nw, nh := int(float64(w)*scale), int(float64(h)*scale)
	off := image.Pt((512-nw)/2, (512-nh)/2)
	draw.CatmullRom.Scale(canvas, image.Rectangle{Min: off, Max: off.Add(image.Pt(nw, nh))}, img, b, draw.Over, nil)
	for q := float32(90); q >= 30; q -= 15 {
		var buf bytes.Buffer
		if err := webp.Encode(&buf, canvas, &webp.Options{Quality: q}); err != nil {
			return nil, false, err
		}
		if buf.Len() <= 500*1024 || q <= 30 {
			return buf.Bytes(), false, nil
		}
	}
	return nil, false, errors.New("não consegui reduzir a figurinha para 500 KB")
}

// Thumbnail gera uma miniatura JPEG (até 72 px) para pré-visualizar imagens.
func Thumbnail(data []byte) []byte {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	b := img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return nil
	}
	scale := 72.0 / float64(max(b.Dx(), b.Dy()))
	dst := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(b.Dx())*scale)), max(1, int(float64(b.Dy())*scale))))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	var buf bytes.Buffer
	if jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 60}) != nil {
		return nil
	}
	return buf.Bytes()
}

// ImageJPEG converte qualquer imagem para JPEG (fotos de perfil e de grupo).
func ImageJPEG(data []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("imagem inválida: %w", err)
	}
	b := img.Bounds()
	side := min(b.Dx(), b.Dy())
	crop := image.Rect(b.Min.X+(b.Dx()-side)/2, b.Min.Y+(b.Dy()-side)/2, 0, 0)
	crop.Max = crop.Min.Add(image.Pt(side, side))
	size := min(side, 640)
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, crop, draw.Src, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// HasFFmpeg diz se o ffmpeg está instalado (o Docker da BOB já traz).
func HasFFmpeg() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// VoiceNote converte qualquer áudio para OGG/Opus mono (nota de voz) e devolve os segundos.
func VoiceNote(ctx context.Context, data []byte) ([]byte, uint32, error) {
	if !HasFFmpeg() {
		return nil, 0, errors.New("ffmpeg não instalado")
	}
	dir, err := os.MkdirTemp("", "bob-audio-")
	if err != nil {
		return nil, 0, err
	}
	defer os.RemoveAll(dir)
	in, out := filepath.Join(dir, "in"), filepath.Join(dir, "out.ogg")
	if err := os.WriteFile(in, data, 0o600); err != nil {
		return nil, 0, err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-loglevel", "error", "-i", in, "-vn", "-ac", "1", "-ar", "48000", "-c:a", "libopus", "-b:a", "48k", "-application", "voip", out)
	if msg, err := cmd.CombinedOutput(); err != nil {
		return nil, 0, fmt.Errorf("ffmpeg: %v %s", err, strings.TrimSpace(string(msg)))
	}
	res, err := os.ReadFile(out)
	if err != nil {
		return nil, 0, err
	}
	return res, duration(ctx, out), nil
}

func duration(ctx context.Context, path string) uint32 {
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=nw=1:nk=1", path).Output()
	if err != nil {
		return 0
	}
	var f float64
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%f", &f); err != nil {
		return 0
	}
	return uint32(f + 0.5)
}
