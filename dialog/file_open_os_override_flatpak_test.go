//go:build flatpak && !windows && !android && !ios && !wasm && !js

package dialog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alexballas/refyne/v2/storage"
)

func TestResolveDocPortalURI(t *testing.T) {
	base := t.TempDir()
	docDir := filepath.Join(base, "doc", "15a797ef")
	registered := filepath.Join(docDir, "Alex_Videos")
	if err := os.MkdirAll(registered, 0o755); err != nil {
		t.Fatal(err)
	}

	// A deduplicated document keeps the picked basename in the returned URI
	// even though the FUSE mount only exposes the registered one.
	picked := storage.NewFileURI(filepath.Join(docDir, "Videos"))
	resolved := resolveDocPortalURI(picked)
	if resolved.Path() != registered {
		t.Fatalf("resolved %q, want %q", resolved.Path(), registered)
	}

	// An existing path is returned unchanged.
	existing := storage.NewFileURI(registered)
	if got := resolveDocPortalURI(existing); got.Path() != registered {
		t.Fatalf("existing path changed to %q", got.Path())
	}

	// Paths outside a doc directory are returned unchanged.
	outside := storage.NewFileURI(filepath.Join(base, "nope", "missing"))
	if got := resolveDocPortalURI(outside); got.Path() != outside.Path() {
		t.Fatalf("outside path changed to %q", got.Path())
	}

	// A doc directory with multiple entries is ambiguous; leave the URI alone.
	multi := filepath.Join(base, "doc", "aa11bb22")
	for _, name := range []string{"one", "two"} {
		if err := os.MkdirAll(filepath.Join(multi, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	ambiguous := storage.NewFileURI(filepath.Join(multi, "missing"))
	if got := resolveDocPortalURI(ambiguous); got.Path() != ambiguous.Path() {
		t.Fatalf("ambiguous path changed to %q", got.Path())
	}
}
