package dialog

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/alexballas/refyne/v2/canvas"
	"github.com/alexballas/refyne/v2/storage"
)

func TestLoadAudioArtworkPrecedence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "track.mp3")
	embedded := encodeAudioArtworkTestImage(t, "jpeg", 20, 20, color.NRGBA{G: 220, A: 255})
	writeAudioArtworkTestFile(t, path, buildAudioArtworkTestMP3(3, embedded))
	writeAudioArtworkTestFile(t, filepath.Join(dir, "cover.jpg"), encodeAudioArtworkTestImage(t, "jpeg", 30, 30, color.NRGBA{B: 220, A: 255}))
	writeAudioArtworkTestFile(t, filepath.Join(dir, "TRACK.PNG"), encodeAudioArtworkTestImage(t, "png", 40, 20, color.NRGBA{R: 220, A: 255}))

	artwork, err := loadAudioArtwork(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := artwork.Bounds().Size(); got.X != 40 || got.Y != 20 {
		t.Fatalf("dimensions = %v, want track-stem sidecar", got)
	}
}

func TestLoadAudioArtworkEmbeddedFrontCover(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "track.mp3")
	other := encodeAudioArtworkTestImage(t, "png", 31, 17, color.NRGBA{R: 220, A: 255})
	front := encodeAudioArtworkTestImage(t, "jpeg", 19, 29, color.NRGBA{B: 220, A: 255})
	writeAudioArtworkTestFile(t, path, buildAudioArtworkTestMP3Pictures([]audioArtworkTestPicture{
		{pictureType: 4, mime: "image/png", data: other},
		{pictureType: 3, mime: "image/jpeg", data: front},
	}))

	artwork, err := loadAudioArtwork(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := artwork.Bounds().Size(); got.X != 19 || got.Y != 29 {
		t.Fatalf("dimensions = %v, want front cover", got)
	}
}

func TestLoadAudioArtworkMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plain.mp3")
	writeAudioArtworkTestFile(t, path, []byte("audio"))
	if artwork, err := loadAudioArtwork(path); err == nil || artwork != nil {
		t.Fatalf("artwork = %v, error = %v; want missing", artwork, err)
	}
}

func TestThumbnailManagerLoadsAudioArtwork(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "track.wav")
	writeAudioArtworkTestFile(t, path, []byte("audio"))
	writeAudioArtworkTestFile(t, filepath.Join(dir, "folder.png"), encodeAudioArtworkTestImage(t, "png", 48, 24, color.NRGBA{G: 220, A: 255}))

	manager := &ThumbnailManager{requests: make([]thumbnailRequest, 0, 1)}
	manager.reqCond = sync.NewCond(&manager.reqLock)
	go manager.worker()

	result := make(chan *canvas.Image, 1)
	manager.Load(storage.NewFileURI(path), func(img *canvas.Image) {
		result <- img
	})

	select {
	case img := <-result:
		if img == nil || img.Image == nil {
			t.Fatal("missing audio thumbnail")
		}
		if got := img.Image.Bounds().Size(); got.X != 128 || got.Y != 128 {
			t.Fatalf("dimensions = %v, want 128x128", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for audio thumbnail")
	}
}

func TestSupportedThumbnailPathAudio(t *testing.T) {
	for _, name := range []string{"track.mp3", "track.m4a", "track.mp4", "track.flac", "track.ogg", "track.oga", "track.opus", "track.wav"} {
		t.Run(name, func(t *testing.T) {
			if _, ok := supportedThumbnailPath(storage.NewFileURI(filepath.Join(t.TempDir(), name))); !ok {
				t.Fatal("audio path not supported")
			}
		})
	}
}

func TestAudioThumbnailCacheKeyIncludesSidecars(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "track.mp3")
	writeAudioArtworkTestFile(t, path, []byte("audio"))

	manager := &ThumbnailManager{}
	before, err := manager.generateCacheKey(path)
	if err != nil {
		t.Fatal(err)
	}
	writeAudioArtworkTestFile(t, filepath.Join(dir, "cover.jpg"), encodeAudioArtworkTestImage(t, "jpeg", 20, 20, color.NRGBA{R: 200, A: 255}))
	after, err := manager.generateCacheKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("cache key unchanged after sidecar addition")
	}
}

type audioArtworkTestPicture struct {
	pictureType byte
	mime        string
	data        []byte
}

func buildAudioArtworkTestMP3(pictureType byte, data []byte) []byte {
	return buildAudioArtworkTestMP3Pictures([]audioArtworkTestPicture{{
		pictureType: pictureType,
		mime:        "image/jpeg",
		data:        data,
	}})
}

func buildAudioArtworkTestMP3Pictures(pictures []audioArtworkTestPicture) []byte {
	var frames bytes.Buffer
	for _, picture := range pictures {
		var body bytes.Buffer
		body.WriteByte(0)
		body.WriteString(picture.mime)
		body.WriteByte(0)
		body.WriteByte(picture.pictureType)
		body.WriteByte(0)
		body.Write(picture.data)

		frames.WriteString("APIC")
		_ = binary.Write(&frames, binary.BigEndian, uint32(body.Len()))
		frames.Write([]byte{0, 0})
		frames.Write(body.Bytes())
	}

	size := frames.Len()
	header := []byte{
		'I', 'D', '3', 3, 0, 0,
		byte(size >> 21 & 0x7f),
		byte(size >> 14 & 0x7f),
		byte(size >> 7 & 0x7f),
		byte(size & 0x7f),
	}
	return append(header, frames.Bytes()...)
}

func encodeAudioArtworkTestImage(t *testing.T, format string, width, height int, fill color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.SetNRGBA(x, y, fill)
		}
	}

	var output bytes.Buffer
	var err error
	switch format {
	case "jpeg":
		err = jpeg.Encode(&output, img, &jpeg.Options{Quality: 92})
	case "png":
		err = png.Encode(&output, img)
	default:
		t.Fatalf("unsupported test format %q", format)
	}
	if err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func writeAudioArtworkTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
