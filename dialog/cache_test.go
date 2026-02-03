package dialog

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestThumbnailManager_GenerateCacheKey(t *testing.T) {
	tm := &ThumbnailManager{}

	// Create a dummy file
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.mp4")
	_ = os.WriteFile(filePath, make([]byte, 100*1024), 0644)

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
	f, _ := os.OpenFile(filePath, os.O_WRONLY, 0644)
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
		_ = os.WriteFile(path, []byte("fake image data"), 0644)
		// Set distinct modification times (oldest first)
		mtime := time.Now().Add(time.Duration(i-100) * time.Minute)
		_ = os.Chtimes(path, mtime, mtime)
	}

	tm.cleanupCache()

	// Verify that we are under or equal to the 80% watermark of MaxCacheFiles (which is 4)
	files, _ := os.ReadDir(tmpDir)
	if len(files) > 4 {
		t.Errorf("Cleanup failed to evict enough files. Got %d, expected <= 4", len(files))
	}

	// Verify that the files remaining are the newest ones
	// The files created were a.jpg (oldest) ... j.jpg (newest)
	// Remaining should be g.jpg, h.jpg, i.jpg, j.jpg (or similar)
	for _, f := range files {
		if f.Name() < "g.jpg" {
			t.Errorf("Cleanup deleted newest file or kept oldest: %s", f.Name())
		}
	}
}
