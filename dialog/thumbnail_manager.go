package dialog

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"golang.org/x/image/draw"
)

type thumbnailRequest struct {
	uri      fyne.URI
	callback func(*canvas.Image)
}

type ThumbnailManager struct {
	cache      sync.Map // map[string]*canvas.Image
	requests   []thumbnailRequest
	reqLock    sync.Mutex
	reqCond    *sync.Cond
	ffmpegPath string
	cacheDir   string
}

var (
	MaxCacheSize  int64 = 500 * 1024 * 1024 // 500MB
	MaxCacheFiles int   = 10000
)

var (
	instance *ThumbnailManager
	once     sync.Once
)

func GetThumbnailManager() *ThumbnailManager {
	once.Do(func() {
		ffmpeg := "ffmpeg"
		if pref := fyne.CurrentApp().Preferences().String(ffmpegPathKey); pref != "" {
			ffmpeg = pref
		}
		instance = &ThumbnailManager{
			requests:   make([]thumbnailRequest, 0, 100),
			ffmpegPath: ffmpeg,
		}
		instance.reqCond = sync.NewCond(&instance.reqLock)

		// Setup persistent cache
		if userCache, err := os.UserCacheDir(); err == nil {
			instance.cacheDir = filepath.Join(userCache, "xfilepicker")
			_ = os.MkdirAll(instance.cacheDir, 0755)
			go instance.cleanupCache()
		}

		// Start workers
		for range 4 {
			go instance.worker()
		}
	})
	return instance
}

func (m *ThumbnailManager) SetFFmpegPath(path string) {
	m.ffmpegPath = path
	fyne.CurrentApp().Preferences().SetString(ffmpegPathKey, path)
}

// LoadMemoryOnly retrieves a thumbnail from memory cache only.
// Returns nil if not in memory.
func (m *ThumbnailManager) LoadMemoryOnly(path string) *canvas.Image {
	if cached, ok := m.cache.Load(path); ok {
		return cached.(*canvas.Image)
	}
	return nil
}

func (m *ThumbnailManager) Load(uri fyne.URI, callback func(*canvas.Image)) {
	if uri == nil || uri.Scheme() != "file" {
		// Not a local file, or nil
		return
	}

	ext := strings.ToLower(filepath.Ext(uri.Path()))
	if !isSupportedImage(ext) && !isSupportedVideo(ext) {
		// Not a supported format
		return
	}

	path := uri.Path()
	if cached, ok := m.cache.Load(path); ok {
		callback(cached.(*canvas.Image))
		return
	}

	// Check disk cache before queuing
	if m.cacheDir != "" {
		if key, err := m.generateCacheKey(path); err == nil {
			cachePath := filepath.Join(m.cacheDir, key+".jpg")
			if _, err := os.Stat(cachePath); err == nil {
				if img, err := loadImage(cachePath); err == nil {
					canvasImg := canvas.NewImageFromImage(img)
					canvasImg.FillMode = canvas.ImageFillContain
					m.cache.Store(path, canvasImg)
					callback(canvasImg)
					return
				}
			}
		}
	}

	// LIFO Queue Logic
	m.reqLock.Lock()
	// If queue is full, drop the OLDEST request (at index 0)
	// Keeps the set of pending requests small and relevant
	if len(m.requests) >= 100 {
		// Drop first
		m.requests = m.requests[1:]
	}
	m.requests = append(m.requests, thumbnailRequest{uri: uri, callback: callback})
	m.reqCond.Signal()
	m.reqLock.Unlock()
}

// PrewarmDirectory attempts to load thumbnails from disk cache into memory in the background.
func (m *ThumbnailManager) PrewarmDirectory(uris []fyne.URI) {
	if m.cacheDir == "" {
		return
	}

	go func() {
		for _, uri := range uris {
			if uri.Scheme() != "file" {
				continue
			}
			path := uri.Path()

			// Skip if already in memory
			if _, ok := m.cache.Load(path); ok {
				continue
			}

			// Generate key (this involves Stat() and reading 32KB, but it's background)
			key, err := m.generateCacheKey(path)
			if err != nil {
				continue
			}

			cachePath := filepath.Join(m.cacheDir, key+".jpg")
			if _, err := os.Stat(cachePath); err == nil {
				if img, err := loadImage(cachePath); err == nil {
					canvasImg := canvas.NewImageFromImage(img)
					canvasImg.FillMode = canvas.ImageFillContain
					m.cache.Store(path, canvasImg)
				}
			}
			// Small sleep to avoid I/O spikes
			time.Sleep(5 * time.Millisecond)
		}
	}()
}

func (m *ThumbnailManager) worker() {
	for {
		m.reqLock.Lock()
		for len(m.requests) == 0 {
			m.reqCond.Wait()
		}
		// Pop LAST request (LIFO)
		lastIdx := len(m.requests) - 1
		req := m.requests[lastIdx]
		m.requests = m.requests[:lastIdx]
		m.reqLock.Unlock()

		path := req.uri.Path()

		if cached, ok := m.cache.Load(path); ok {
			req.callback(cached.(*canvas.Image))
			continue
		}

		var img image.Image
		var err error

		ext := strings.ToLower(filepath.Ext(path))
		if isSupportedImage(ext) {
			img, err = loadImage(path)
		} else if isSupportedVideo(ext) {
			img, err = m.generateVideoThumbnail(path)
		}

		if err != nil || img == nil {
			continue
		}

		// Resize and letterbox
		// Use fileIconSize * 2 for high density displays (128px)
		const targetSize = 128
		dst := image.NewRGBA(image.Rect(0, 0, targetSize, targetSize))

		// Fill with black
		draw.Draw(dst, dst.Bounds(), &image.Uniform{image.Black}, image.Point{}, draw.Src)

		// Calculate scaled dimensions
		srcBounds := img.Bounds()
		srcW, srcH := srcBounds.Dx(), srcBounds.Dy()
		var scaledW, scaledH int

		// Avoid division by zero
		if srcW == 0 || srcH == 0 {
			continue
		}

		ratio := float64(srcW) / float64(srcH)
		if ratio > 1 {
			// Landscape or square
			scaledW = targetSize
			scaledH = int(float64(targetSize) / ratio)
		} else {
			// Portrait
			scaledH = targetSize
			scaledW = int(float64(targetSize) * ratio)
		}

		// Center
		xBase := (targetSize - scaledW) / 2
		yBase := (targetSize - scaledH) / 2
		targetRect := image.Rect(xBase, yBase, xBase+scaledW, yBase+scaledH)

		// Use ApproxBiLinear for speed
		draw.ApproxBiLinear.Scale(dst, targetRect, img, srcBounds, draw.Over, nil)

		canvasImg := canvas.NewImageFromImage(dst)
		canvasImg.FillMode = canvas.ImageFillContain

		m.cache.Store(path, canvasImg)

		// Save to disk cache
		if m.cacheDir != "" {
			if key, err := m.generateCacheKey(path); err == nil {
				cachePath := filepath.Join(m.cacheDir, key+".jpg")
				f, err := os.Create(cachePath)
				if err == nil {
					_ = jpeg.Encode(f, dst, &jpeg.Options{Quality: 85})
					f.Close()
				}
			}
		}

		req.callback(canvasImg)
	}
}

func loadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	return img, err
}

func (m *ThumbnailManager) generateVideoThumbnail(path string) (image.Image, error) {

	// 1. Get duration
	duration, err := m.getVideoDuration(path)
	if err != nil {
		// Fallback to 1 second if duration parsing fails
		duration = 1 * time.Second
	}

	// 2. Calculate seek time (middle)
	seekTime := duration / 2
	seekStr := fmt.Sprintf("%02d:%02d:%02d.%03d",
		int(seekTime.Hours()),
		int(seekTime.Minutes())%60,
		int(seekTime.Seconds())%60,
		seekTime.Milliseconds()%1000)

	// ffmpeg -ss <seek> -i <file> -vframes 1 -f image2 -
	// Note: Putting -ss before -i is faster (input seeking) but less accurate.
	// For thumbnails, input seeking is usually fine and much faster.
	cmd := exec.Command(m.ffmpegPath, "-ss", seekStr, "-i", path, "-vframes", "1", "-f", "image2", "-strict", "unofficial", "-")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	img, _, err := image.Decode(&buf)
	return img, err
}

func (m *ThumbnailManager) getVideoDuration(path string) (time.Duration, error) {
	// ffmpeg -i <file> 2>&1 | grep "Duration"
	cmd := exec.Command(m.ffmpegPath, "-i", path)
	// ffmpeg prints to stderr
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	// Run usually fails because no output file is specified, but we get the info
	_ = cmd.Run()

	out := stderr.String()
	// Regex for "Duration: HH:MM:SS.mm"
	re := regexp.MustCompile(`Duration: (\d{2}):(\d{2}):(\d{2})\.(\d{2})`)
	matches := re.FindStringSubmatch(out)
	if len(matches) < 5 {
		return 0, fmt.Errorf("could not find duration in output")
	}

	hours := 0
	minutes := 0
	seconds := 0
	centiseconds := 0

	fmt.Sscanf(matches[1], "%d", &hours)
	fmt.Sscanf(matches[2], "%d", &minutes)
	fmt.Sscanf(matches[3], "%d", &seconds)
	fmt.Sscanf(matches[4], "%d", &centiseconds)

	duration := time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second +
		time.Duration(centiseconds*10)*time.Millisecond

	return duration, nil
}
func isSupportedImage(ext string) bool {
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png"
}

func isSupportedVideo(ext string) bool {
	return ext == ".mp4" || ext == ".mkv" || ext == ".avi" || ext == ".webm" || ext == ".mov"
}

func (m *ThumbnailManager) generateCacheKey(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return "", err
	}

	h := sha256.New()
	// Key factor 1 & 2: Path and ModTime
	h.Write([]byte(absPath))
	h.Write([]byte(info.ModTime().String()))
	h.Write([]byte(fmt.Sprintf("%d", info.Size())))

	// Key factor 3: Partial content (32KB)
	f, err := os.Open(absPath)
	if err == nil {
		defer f.Close()
		buf := make([]byte, 32*1024)
		n, _ := f.Read(buf)
		h.Write(buf[:n])
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func (m *ThumbnailManager) cleanupCache() {
	if m.cacheDir == "" {
		return
	}

	files, err := os.ReadDir(m.cacheDir)
	if err != nil {
		return
	}

	type fileInfo struct {
		name string
		size int64
		time time.Time
	}

	var cachedFiles []fileInfo
	var totalSize int64

	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".jpg" {
			continue
		}
		info, err := f.Info()
		if err != nil {
			continue
		}
		cachedFiles = append(cachedFiles, fileInfo{
			name: f.Name(),
			size: info.Size(),
			time: info.ModTime(),
		})
		totalSize += info.Size()
	}

	if totalSize <= MaxCacheSize && len(cachedFiles) <= MaxCacheFiles {
		return
	}

	// LRU: Sort by time ascending (oldest first)
	sort.Slice(cachedFiles, func(i, j int) bool {
		return cachedFiles[i].time.Before(cachedFiles[j].time)
	})

	for _, f := range cachedFiles {
		if totalSize <= int64(float64(MaxCacheSize)*0.8) && len(cachedFiles) <= int(float64(MaxCacheFiles)*0.8) {
			break
		}
		_ = os.Remove(filepath.Join(m.cacheDir, f.name))
		totalSize -= f.size
		cachedFiles = cachedFiles[1:]
	}
}
