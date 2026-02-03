package dialog

import (
	"fmt"
	"image"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/storage"
)

func TestThumbnailManager_Video_AspectRatio(t *testing.T) {
	// 1. Check for ffmpeg
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not found")
	}

	// 2. Create a dummy video file with specific aspect ratio (e.g., 320x180 - 16:9)
	tmpDir := t.TempDir()
	videoPath := filepath.Join(tmpDir, "test_video_16_9.mp4")

	// Create a 1-second video with red color
	cmd := exec.Command("ffmpeg", "-f", "lavfi", "-i", "color=c=red:s=320x180:d=1",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-y", videoPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("failed to create video: %v, output: %s", err, out)
	}

	// 3. Initialize Manager
	manager := GetThumbnailManager()
	manager.SetFFmpegPath("ffmpeg")

	// 4. Request thumbnail
	var wg sync.WaitGroup
	wg.Add(1)

	var result *canvas.Image
	uri := storage.NewFileURI(videoPath)

	manager.Load(uri, func(img *canvas.Image) {
		result = img
		wg.Done()
	})

	// 5. Wait for result
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for thumbnail")
	}

	// 6. Verify result
	if result == nil || result.Image == nil {
		t.Fatal("Thumbnail generation failed")
	}

	bounds := result.Image.Bounds()
	if bounds.Dx() != 128 || bounds.Dy() != 128 {
		t.Errorf("Expected 128x128 thumbnail, got %dx%d", bounds.Dx(), bounds.Dy())
	}

	// Check for black bars (letterboxing)
	// Top row should be black for 16:9 video in square container
	// 16:9 fits as 128x72 in 128x128.
	// Height of image = 72. Top bar = (128-72)/2 = 28 pixels approx.
	// Check pixel at (64, 5) - should be black
	// Check pixel at (64, 64) - should be red (video content)

	rgba, ok := result.Image.(*image.RGBA)
	if !ok {
		t.Fatal("Expected RGBA image")
	}

	checkColor := func(x, y int, name string) (r, g, b, a uint32) {
		c := rgba.At(x, y)
		r, g, b, a = c.RGBA()
		return
	}

	// Top bar (Black)
	r, g, b, _ := checkColor(64, 5, "top bar")
	if r > 1000 || g > 1000 || b > 1000 { // Allow some slack but should be near 0
		t.Errorf("Expected black top bar, got R:%d G:%d B:%d", r, g, b)
	}

	// Center (Red)
	r, g, b, _ = checkColor(64, 64, "center")
	if r < 50000 || g > 10000 || b > 10000 { // Red should be high, others low
		t.Errorf("Expected red center, got R:%d G:%d B:%d", r, g, b)
	}

	fmt.Printf("Thumbnail generated successfully with letterboxing: %dx%d\n", bounds.Dx(), bounds.Dy())
}
