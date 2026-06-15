package dialog

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexballas/refyne/v2/canvas"
	"github.com/alexballas/refyne/v2/storage"
)

func TestThumbnailManager_GenerateCacheKey(t *testing.T) {
	tm := &ThumbnailManager{}

	// Create a dummy file
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.mp4")
	_ = os.WriteFile(filePath, make([]byte, 100*1024), 0o644)

	key1, err := tm.generateCacheKey(filePath)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	// Same file, same time -> same key
	key2, err := tm.generateCacheKey(filePath)
	if err != nil {
		t.Fatalf("Failed to generate key2: %v", err)
	}

	if key1 != key2 {
		t.Errorf("Keys should be identical for same file: %s != %s", key1, key2)
	}

	// Modify modification time -> different key
	time.Sleep(10 * time.Millisecond)
	now := time.Now()
	_ = os.Chtimes(filePath, now, now)

	key3, err := tm.generateCacheKey(filePath)
	if err != nil {
		t.Fatalf("Failed to generate key3: %v", err)
	}

	if key3 == key1 {
		t.Error("Key should change when modification time changes")
	}

	// Modify content (within first 32KB) -> different key
	f, _ := os.OpenFile(filePath, os.O_WRONLY, 0o644)
	f.Write([]byte("change"))
	f.Close()
	_ = os.Chtimes(filePath, now, now) // Reset time to isolate content change

	key4, err := tm.generateCacheKey(filePath)
	if err != nil {
		t.Fatalf("Failed to generate key4: %v", err)
	}
	if key4 == key3 {
		t.Error("Key should change when first 32KB content changes")
	}
}

func TestThumbnailManager_CleanupCache(t *testing.T) {
	tmpDir := t.TempDir()
	tm := &ThumbnailManager{
		cacheDir: tmpDir,
	}

	// Temporarily lower limits
	oldSize := MaxCacheSize
	oldFiles := MaxCacheFiles
	MaxCacheSize = 100 // tiny limit
	MaxCacheFiles = 5  // tiny limit
	defer func() {
		MaxCacheSize = oldSize
		MaxCacheFiles = oldFiles
	}()

	// Create 10 files
	for i := 0; i < 10; i++ {
		path := filepath.Join(tmpDir, string(rune('a'+i))+".jpg")
		_ = os.WriteFile(path, []byte("fake image data"), 0o644)
		// Cache file mtime is the app-maintained last access time (oldest first).
		mtime := time.Now().Add(time.Duration(i-100) * time.Minute)
		_ = os.Chtimes(path, mtime, mtime)
	}

	tm.cleanupCache()

	// Verify that we are under or equal to the 80% watermark of MaxCacheFiles (which is 4)
	files, _ := os.ReadDir(tmpDir)
	if len(files) > 4 {
		t.Errorf("Cleanup failed to evict enough files. Got %d, expected <= 4", len(files))
	}

	// Verify that the files remaining are the most recently accessed ones.
	// The access times were a.jpg (oldest) ... j.jpg (newest).
	// Remaining should be g.jpg, h.jpg, i.jpg, j.jpg (or similar)
	for _, f := range files {
		if f.Name() < "g.jpg" {
			t.Errorf("Cleanup deleted newest file or kept oldest: %s", f.Name())
		}
	}
}

func TestThumbnailManager_CleanupCacheUsesLastAccessTime(t *testing.T) {
	tmpDir := t.TempDir()
	tm := &ThumbnailManager{
		cacheDir: tmpDir,
	}

	oldSize := MaxCacheSize
	oldFiles := MaxCacheFiles
	MaxCacheSize = 1024 * 1024
	MaxCacheFiles = 3
	defer func() {
		MaxCacheSize = oldSize
		MaxCacheFiles = oldFiles
	}()

	base := time.Now().Add(-time.Hour)
	for i, name := range []string{"a.jpg", "b.jpg", "c.jpg", "d.jpg"} {
		path := filepath.Join(tmpDir, name)
		_ = os.WriteFile(path, []byte("fake image data"), 0o644)
		accessed := base.Add(time.Duration(i) * time.Minute)
		_ = os.Chtimes(path, accessed, accessed)
	}

	tm.markCacheAccessed(filepath.Join(tmpDir, "a.jpg"))
	tm.cleanupCache()

	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}

	remaining := make(map[string]bool, len(files))
	for _, f := range files {
		remaining[f.Name()] = true
	}

	if len(remaining) > 2 {
		t.Fatalf("cleanup should reduce to the 80%% watermark of 2 files, got %d", len(remaining))
	}
	if !remaining["a.jpg"] {
		t.Fatalf("recently accessed cache file was evicted; remaining files: %v", remaining)
	}
}

func TestThumbnailManager_LoadCachedHitsDiskOnly(t *testing.T) {
	tmpDir := t.TempDir()
	tm := &ThumbnailManager{
		cacheDir: tmpDir,
	}

	sourcePath := filepath.Join(tmpDir, "source.jpg")
	if err := os.WriteFile(sourcePath, []byte("source content"), 0o644); err != nil {
		t.Fatalf("WriteFile source failed: %v", err)
	}

	key, err := tm.generateCacheKey(sourcePath)
	if err != nil {
		t.Fatalf("generateCacheKey failed: %v", err)
	}

	cachePath := filepath.Join(tmpDir, key+".jpg")
	cacheFile, err := os.Create(cachePath)
	if err != nil {
		t.Fatalf("Create cache file failed: %v", err)
	}
	cacheImg := image.NewRGBA(image.Rect(0, 0, 2, 2))
	cacheImg.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := jpeg.Encode(cacheFile, cacheImg, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatalf("Encode cache file failed: %v", err)
	}
	if err := cacheFile.Close(); err != nil {
		t.Fatalf("Close cache file failed: %v", err)
	}

	oldAccess := time.Now().Add(-time.Hour)
	if err := os.Chtimes(cachePath, oldAccess, oldAccess); err != nil {
		t.Fatalf("Chtimes cache file failed: %v", err)
	}

	var got *canvas.Image
	tm.LoadCached(storage.NewFileURI(sourcePath), func(img *canvas.Image) {
		got = img
	})
	if got == nil || got.Image == nil {
		t.Fatal("LoadCached should return the disk-cached thumbnail")
	}

	info, err := os.Stat(cachePath)
	if err != nil {
		t.Fatalf("Stat cache file failed: %v", err)
	}
	if !info.ModTime().After(oldAccess) {
		t.Fatalf("LoadCached should update cache access time, got %s, want after %s", info.ModTime(), oldAccess)
	}

	missingSource := filepath.Join(tmpDir, "missing.jpg")
	if err := os.WriteFile(missingSource, []byte("missing cache content"), 0o644); err != nil {
		t.Fatalf("WriteFile missing source failed: %v", err)
	}
	calledOnMiss := false
	tm.LoadCached(storage.NewFileURI(missingSource), func(img *canvas.Image) {
		calledOnMiss = true
	})
	if calledOnMiss {
		t.Fatal("LoadCached should not call back or generate on cache miss")
	}
}
