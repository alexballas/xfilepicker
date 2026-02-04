package dialog

import (
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

var zoomLevels = []float32{
	0.75,
	1.0,
	1.25,
	1.5,
	1.75,
	2.0,
}

const defaultZoomLevelIndex = 1 // 1.0

func clampZoomLevelIndex(i int) int {
	if i < 0 {
		return 0
	}
	if i >= len(zoomLevels) {
		return len(zoomLevels) - 1
	}
	return i
}

func isZoomModifierActive() bool {
	d, ok := fyne.CurrentApp().Driver().(desktop.Driver)
	if !ok {
		return false
	}

	mods := d.CurrentKeyModifiers()
	if mods&fyne.KeyModifierControl != 0 {
		return true
	}
	// Support Command+scroll on macOS (and Control elsewhere) by honoring the platform shortcut modifier.
	return mods&fyne.KeyModifierShortcutDefault != 0
}

type zoomScrollOverlay struct {
	widget.BaseWidget
	onStep func(steps int)
	accDY  float32
}

func newZoomScrollOverlay(onStep func(steps int)) *zoomScrollOverlay {
	z := &zoomScrollOverlay{onStep: onStep}
	z.ExtendBaseWidget(z)
	return z
}

func (z *zoomScrollOverlay) Visible() bool {
	if !z.BaseWidget.Visible() {
		return false
	}
	return isZoomModifierActive()
}

func (z *zoomScrollOverlay) Scrolled(e *fyne.ScrollEvent) {
	if z.onStep == nil {
		return
	}

	// Fyne scroll deltas are scaled; on typical mouse wheels, DY is ~40 per notch.
	// Accumulate so touchpads don't zoom too quickly.
	const notch = float32(40)

	if math.IsNaN(float64(e.Scrolled.DY)) || math.IsInf(float64(e.Scrolled.DY), 0) {
		return
	}

	z.accDY += e.Scrolled.DY

	var steps int
	for z.accDY >= notch {
		steps++
		z.accDY -= notch
	}
	for z.accDY <= -notch {
		steps--
		z.accDY += notch
	}

	if steps != 0 {
		z.onStep(steps)
	}
}

func (z *zoomScrollOverlay) CreateRenderer() fyne.WidgetRenderer {
	return &zoomScrollOverlayRenderer{}
}

var _ fyne.Scrollable = (*zoomScrollOverlay)(nil)

type zoomScrollOverlayRenderer struct{}

func (r *zoomScrollOverlayRenderer) Layout(fyne.Size) {}
func (r *zoomScrollOverlayRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, 0)
}
func (r *zoomScrollOverlayRenderer) Refresh()                     {}
func (r *zoomScrollOverlayRenderer) Objects() []fyne.CanvasObject { return nil }
func (r *zoomScrollOverlayRenderer) Destroy()                     {}
