package dialog

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/test"
)

func TestResizeLayout_OnResizeWhenExternalSizeChanges(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	callbacks := 0
	external := fyne.NewSize(1200, 800)
	r := &resizeLayout{
		internal: layout.NewStackLayout(),
		onResize: func() {
			callbacks++
		},
		externalSize: func() fyne.Size {
			return external
		},
	}

	contentSize := fyne.NewSize(700, 500)
	r.Layout(nil, contentSize)
	fyne.DoAndWait(func() {})
	if callbacks != 1 {
		t.Fatalf("expected 1 resize callback after initial layout, got %d", callbacks)
	}

	// No internal or external size change should not trigger callback.
	r.lastFired = time.Now().Add(-time.Second)
	r.Layout(nil, contentSize)
	fyne.DoAndWait(func() {})
	if callbacks != 1 {
		t.Fatalf("expected callback count to stay at 1, got %d", callbacks)
	}

	// External size change should trigger callback even when content size is unchanged.
	external = fyne.NewSize(1300, 800)
	r.lastFired = time.Now().Add(-time.Second)
	r.Layout(nil, contentSize)
	fyne.DoAndWait(func() {})
	if callbacks != 2 {
		t.Fatalf("expected callback count to be 2 after external resize, got %d", callbacks)
	}
}
