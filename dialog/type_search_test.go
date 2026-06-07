package dialog

import (
	"testing"

	"github.com/alexballas/refyne/v2"
	"github.com/alexballas/refyne/v2/container"
	"github.com/alexballas/refyne/v2/test"
	"github.com/alexballas/refyne/v2/widget"
)

// TestTypeToSearchFromFocusedList reproduces the regression where selecting a
// file (which focuses the list/grid so arrow keys keep working) stopped
// type-to-search: a focused list/grid swallows typed runes, so the canvas-level
// rune hook — which only fires when nothing is focused — never ran. The fix
// routes the widgets' OnTypedRune to the search box.
func TestTypeToSearchFromFocusedList(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)
	w := a.NewWindow("Test")

	searchEntry := newTypeSearchEntry()
	fd := &fileDialog{
		searchEntry: searchEntry,
		parent:      w,
	}
	fd.fileList = newFileList(fd)
	// The search box must live in the canvas tree for Focus to register under the
	// test driver, mirroring the real dialog where it sits in the toolbar.
	w.SetContent(container.NewVBox(searchEntry))

	// Typing while the list holds focus must land in the search box, exactly as
	// if the user had clicked it first.
	fd.fileList.list.TypedRune('h')
	fd.fileList.list.TypedRune('i')

	if searchEntry.Text != "hi" {
		t.Fatalf("list type-to-search: expected %q, got %q", "hi", searchEntry.Text)
	}
	if w.Canvas().Focused() != searchEntry {
		t.Fatal("list type-to-search: search entry should be focused")
	}

	// The grid view behaves the same way.
	searchEntry.SetText("")
	w.Canvas().Unfocus()
	fd.fileList.grid.TypedRune('z')
	if searchEntry.Text != "z" {
		t.Fatalf("grid type-to-search: expected %q, got %q", "z", searchEntry.Text)
	}
	if w.Canvas().Focused() != searchEntry {
		t.Fatal("grid type-to-search: search entry should be focused")
	}

	// Space is reserved for toggling the focused item's selection, so it must not
	// leak into the search box.
	searchEntry.SetText("")
	fd.fileList.list.TypedRune(' ')
	if searchEntry.Text != "" {
		t.Fatalf("space should not type-to-search, got %q", searchEntry.Text)
	}
}

// TestSearchEntryEscapeClearsAndUnfocuses verifies that Escape cancels an
// in-progress type-ahead search: it clears the text (which resets the filter via
// OnChanged) and releases focus, rather than leaving the typed filter applied
// with the box still focused.
func TestSearchEntryEscapeClearsAndUnfocuses(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)
	w := a.NewWindow("Test")

	entry := newTypeSearchEntry()

	var filter string
	entry.OnChanged = func(s string) { filter = s }
	entry.onEscape = func() { w.Canvas().Unfocus() }

	w.SetContent(container.NewVBox(entry))
	w.Canvas().Focus(entry)
	entry.SetText("abc")
	if filter != "abc" {
		t.Fatalf("setup: expected filter %q, got %q", "abc", filter)
	}

	entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})

	if entry.Text != "" {
		t.Fatalf("escape should clear the search text, got %q", entry.Text)
	}
	if filter != "" {
		t.Fatalf("escape should reset the filter via OnChanged, got %q", filter)
	}
	if w.Canvas().Focused() != nil {
		t.Fatalf("escape should release focus, still focused on %T", w.Canvas().Focused())
	}

	// A non-Escape key must still reach the embedded Entry (type-ahead keeps
	// working after the box is focused).
	w.Canvas().Focus(entry)
	entry.TypedRune('q')
	if entry.Text != "q" {
		t.Fatalf("normal typing should still work, got %q", entry.Text)
	}
}

func TestTypeToSearch(t *testing.T) {
	a := test.NewApp()
	w := a.NewWindow("Test")

	// Mock fileDialog structure minimal needed
	searchEntry := newTypeSearchEntry()
	fd := &fileDialog{
		searchEntry: searchEntry,
		parent:      w,
		win:         widget.NewModalPopUp(container.NewVBox(searchEntry), w.Canvas()),
	}
	fd.win.Show()

	// Manually simulating Show() behavior regarding hook
	fd.originalOnTypedRune = w.Canvas().OnTypedRune()
	w.Canvas().SetOnTypedRune(fd.typedRuneHook)

	// 1. Initial State
	if searchEntry.Text != "" {
		t.Errorf("Expected empty search, got %s", searchEntry.Text)
	}

	// 2. Type 'a' (Simulate Global Type)
	// We call the hook directly because test.Type goes to focused widget only.
	// But in real app, Canvas().OnTypedRune is called.
	w.Canvas().SetOnTypedRune(func(r rune) {
		fd.typedRuneHook(r)
	})
	// Simulate typing on canvas
	// refyne's test.Type types into the focused object.
	// If nothing focused?
	// We can manually invoke the hook for unit testing logic.
	fd.typedRuneHook('a')

	if searchEntry.Text != "a" {
		t.Errorf("Expected 'a', got %s", searchEntry.Text)
	}

	if w.Canvas().Focused() != searchEntry {
		t.Error("Search entry should be focused")
	}

	// 3. Type 'b'
	// Now searchEntry IS focused.
	// Hook should return early.
	fd.typedRuneHook('b')

	// Because we are calling hook directly, and hook returns early, text shouldn't change via hook.
	// In real app, the event continues to the focused ENTRY, which types 'b'.
	// Here we verify hook doesn't DOUBLE type.
	if searchEntry.Text != "a" {
		t.Errorf("Expected 'a' (hook should skip), got %s", searchEntry.Text)
	}

	// 3. Test Overlay Entry (Simulation of "New Folder")
	overlayEntry := widget.NewEntry()
	// Add overlayEntry to the window content
	w.SetContent(container.NewVBox(searchEntry, overlayEntry))
	w.Canvas().Focus(overlayEntry)

	// Verify focus
	// if w.Canvas().Focused() != overlayEntry {
	// 	t.Fatal("Overlay entry should be focused")
	// }

	searchEntry.SetText("")
	// Type 'x' into overlay
	overlayEntry.TypedRune('x')
	// Hook is called by refyne loop usually. We simulate hook call.
	// IMPORTANT: In reality, if overlayEntry handles it, does hook run?
	// If hook runs, it must check focused.
	fd.typedRuneHook('x')

	if searchEntry.Text != "" {
		t.Errorf("Hook stole input from overlay! Search text: '%s'", searchEntry.Text)
	}
	if overlayEntry.Text != "x" {
		t.Errorf("Overlay entry missing input. Text: '%s'", overlayEntry.Text)
	}
}
