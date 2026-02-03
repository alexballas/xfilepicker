package dialog

import (
	"testing"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestTypeToSearch(t *testing.T) {
	a := test.NewApp()
	w := a.NewWindow("Test")

	// Mock fileDialog structure minimal needed
	searchEntry := widget.NewEntry()
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
	// Fyne's test.Type types into the focused object.
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
	// Hook is called by Fyne loop usually. We simulate hook call.
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
