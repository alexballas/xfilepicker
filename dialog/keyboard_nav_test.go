package dialog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alexballas/refyne/v2"
	"github.com/alexballas/refyne/v2/storage"
	"github.com/alexballas/refyne/v2/test"
)

// newKeyboardNavDialog builds a multi-select file dialog populated with the
// given file names and pointed at a fresh temp directory. It deliberately avoids
// makeUI so the test exercises selection logic without constructing the full
// toolbar (which is irrelevant here).
func newKeyboardNavDialog(t *testing.T, names ...string) *fileDialog {
	t.Helper()

	a := test.NewApp()
	t.Cleanup(a.Quit)

	w := a.NewWindow("Test")
	root := t.TempDir()
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s failed: %v", name, err)
		}
	}

	d := NewFileOpen(func([]fyne.URIReadCloser, error) {}, w, true).(*fileDialog)
	d.fileList = newFileList(d)
	d.fileList.setView(ListView)

	lister, err := storage.ListerForURI(storage.NewFileURI(root))
	if err != nil {
		t.Fatalf("lister failed: %v", err)
	}
	d.refreshDir(lister)

	if len(d.fileList.filtered) != len(names) {
		t.Fatalf("expected %d files, got %d", len(names), len(d.fileList.filtered))
	}
	return d
}

// selectedIndices reports which filtered items are currently selected, in order.
func selectedIndices(d *fileDialog) []int {
	var ids []int
	for i, u := range d.fileList.filtered {
		if d.IsSelected(u) {
			ids = append(ids, i)
		}
	}
	return ids
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFileList_KeyboardArrow_SelectsLikeClick(t *testing.T) {
	d := newKeyboardNavDialog(t, "a.txt", "b.txt", "c.txt", "d.txt")

	d.Select(0)

	// A plain arrow must replace the selection with the newly focused item,
	// exactly as a single click does.
	d.fileList.navigateFromKeyboard(2, 0)

	if got := selectedIndices(d); !equalInts(got, []int{2}) {
		t.Fatalf("plain arrow should select only the focused item, got %v", got)
	}
	if d.anchor != 2 {
		t.Fatalf("plain arrow should move the anchor to the focused item, got %d", d.anchor)
	}
}

func TestFileList_KeyboardShiftArrow_ExtendsRangeFromAnchor(t *testing.T) {
	d := newKeyboardNavDialog(t, "a.txt", "b.txt", "c.txt", "d.txt", "e.txt")

	d.Select(1) // anchor is now 1

	// Shift+arrow extends from the anchor to the focused item.
	d.fileList.navigateFromKeyboard(3, fyne.KeyModifierShift)
	if got := selectedIndices(d); !equalInts(got, []int{1, 2, 3}) {
		t.Fatalf("shift extend should select range 1..3, got %v", got)
	}
	if d.anchor != 1 {
		t.Fatalf("shift extend must not move the anchor, got %d", d.anchor)
	}

	// Extending further grows the range.
	d.fileList.navigateFromKeyboard(4, fyne.KeyModifierShift)
	if got := selectedIndices(d); !equalInts(got, []int{1, 2, 3, 4}) {
		t.Fatalf("shift extend should grow range to 1..4, got %v", got)
	}

	// Moving back toward the anchor shrinks the range rather than leaving stale
	// items selected.
	d.fileList.navigateFromKeyboard(2, fyne.KeyModifierShift)
	if got := selectedIndices(d); !equalInts(got, []int{1, 2}) {
		t.Fatalf("shift back toward anchor should shrink range to 1..2, got %v", got)
	}
}

func TestFileList_KeyboardCtrlArrow_MovesFocusWithoutSelecting(t *testing.T) {
	d := newKeyboardNavDialog(t, "a.txt", "b.txt", "c.txt", "d.txt")

	d.Select(1)

	// Ctrl+arrow moves the focus cursor only; the selection and anchor are left
	// untouched.
	d.fileList.navigateFromKeyboard(3, fyne.KeyModifierControl)

	if got := selectedIndices(d); !equalInts(got, []int{1}) {
		t.Fatalf("ctrl move must keep the existing selection, got %v", got)
	}
	if d.anchor != 1 {
		t.Fatalf("ctrl move must not change the anchor, got %d", d.anchor)
	}
}

func TestFileList_ArrowKeyNavigationIsWired(t *testing.T) {
	d := newKeyboardNavDialog(t, "a.txt", "b.txt", "c.txt")

	if d.fileList.list.OnKeyboardNavigated == nil {
		t.Fatal("list keyboard navigation callback is not wired")
	}
	if d.fileList.grid.OnKeyboardNavigated == nil {
		t.Fatal("grid keyboard navigation callback is not wired")
	}

	// Driving the real widget TypedKey path (the test driver reports no
	// modifiers, so this is the plain-arrow case) must select the item the
	// cursor lands on. The highlight starts at item 0, so Down lands on item 1.
	d.fileList.list.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})

	if got := selectedIndices(d); !equalInts(got, []int{1}) {
		t.Fatalf("arrow key navigation should select the newly focused item, got %v", got)
	}
}
