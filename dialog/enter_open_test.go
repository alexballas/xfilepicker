package dialog

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestEnterTriggersOpenWhenSelectionPresent(t *testing.T) {
	a := test.NewApp()
	w := a.NewWindow("Test")
	w.SetContent(container.NewVBox(widget.NewLabel("content")))

	called := 0
	fd := &fileDialog{
		parent: w,
		win:    widget.NewModalPopUp(container.NewVBox(), w.Canvas()),
		open: widget.NewButton("Open", func() {
			called++
		}),
		selected: map[string]fyne.URI{
			"file:///tmp/test": storage.NewFileURI("/tmp/test"),
		},
	}

	w.Canvas().Unfocus()
	fd.typedKeyHook(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if called != 1 {
		t.Fatalf("expected Open to be triggered once, got %d", called)
	}

	fd.typedKeyHook(&fyne.KeyEvent{Name: fyne.KeyEnter})
	if called != 2 {
		t.Fatalf("expected Open to be triggered twice, got %d", called)
	}
}

func TestEnterDoesNotTriggerOpenWhenFocusIsEntry(t *testing.T) {
	a := test.NewApp()
	w := a.NewWindow("Test")

	searchEntry := widget.NewEntry()
	w.SetContent(container.NewVBox(searchEntry))

	called := 0
	fd := &fileDialog{
		parent:      w,
		win:         widget.NewModalPopUp(container.NewVBox(), w.Canvas()),
		searchEntry: searchEntry,
		open: widget.NewButton("Open", func() {
			called++
		}),
		selected: map[string]fyne.URI{
			"file:///tmp/test": storage.NewFileURI("/tmp/test"),
		},
	}

	w.Canvas().Focus(searchEntry)
	if w.Canvas().Focused() != searchEntry {
		t.Fatalf("expected entry to be focused, got %T", w.Canvas().Focused())
	}
	fd.typedKeyHook(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if called != 0 {
		t.Fatalf("expected Open to not be triggered, got %d", called)
	}
}

func TestEnterDoesNotTriggerOpenWhenDisabledOrEmpty(t *testing.T) {
	a := test.NewApp()
	w := a.NewWindow("Test")
	w.SetContent(container.NewVBox(widget.NewLabel("content")))

	disabledCalled := 0
	fdDisabled := &fileDialog{
		parent: w,
		win:    widget.NewModalPopUp(container.NewVBox(), w.Canvas()),
		open: widget.NewButton("Open", func() {
			disabledCalled++
		}),
		selected: map[string]fyne.URI{
			"file:///tmp/test": storage.NewFileURI("/tmp/test"),
		},
	}
	fdDisabled.open.Disable()

	w.Canvas().Unfocus()
	fdDisabled.typedKeyHook(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if disabledCalled != 0 {
		t.Fatalf("expected disabled Open to not be triggered, got %d", disabledCalled)
	}

	emptyCalled := 0
	fdEmpty := &fileDialog{
		parent: w,
		win:    widget.NewModalPopUp(container.NewVBox(), w.Canvas()),
		open: widget.NewButton("Open", func() {
			emptyCalled++
		}),
		selected: make(map[string]fyne.URI),
	}

	w.Canvas().Unfocus()
	fdEmpty.typedKeyHook(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if emptyCalled != 0 {
		t.Fatalf("expected Open to not be triggered with empty selection, got %d", emptyCalled)
	}
}
