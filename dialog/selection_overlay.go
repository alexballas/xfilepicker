package dialog

import (
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type selectionOverlay struct {
	widget.BaseWidget
	content fyne.CanvasObject

	rect *canvas.Rectangle

	startPos fyne.Position
	curPos   fyne.Position
	dragging bool

	onChanged func(tl, br fyne.Position)
	onEnd     func()

	debugRects []fyne.CanvasObject
}

func newSelectionOverlay(content fyne.CanvasObject, onChanged func(tl, br fyne.Position), onEnd func()) *selectionOverlay {
	s := &selectionOverlay{
		content:   content,
		rect:      canvas.NewRectangle(color.Transparent),
		onChanged: onChanged,
		onEnd:     onEnd,
	}
	s.rect.StrokeColor = theme.Color(theme.ColorNamePrimary)
	s.rect.StrokeWidth = 2
	s.rect.FillColor = theme.Color(theme.ColorNameFocus)
	// Make transparent
	r, g, b, _ := s.rect.FillColor.RGBA()
	s.rect.FillColor = color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 64}

	s.rect.Hide()
	s.ExtendBaseWidget(s)
	return s
}

func (s *selectionOverlay) setDebugRects(rects []fyne.CanvasObject) {
	s.debugRects = rects
	s.Refresh()
}

func (s *selectionOverlay) CreateRenderer() fyne.WidgetRenderer {
	return &selectionOverlayRenderer{s: s}
}

func (s *selectionOverlay) Dragged(e *fyne.DragEvent) {
	if !s.dragging {
		s.dragging = true
		s.startPos = e.PointEvent.Position.Subtract(e.Dragged)
		s.rect.Show()
	}

	s.curPos = e.PointEvent.Position
	s.refreshRect()

	if s.onChanged != nil {
		s.onChanged(s.rectCoords())
	}
}

func (s *selectionOverlay) DragEnd() {
	if !s.dragging {
		return
	}
	s.dragging = false
	s.rect.Hide()
	s.rect.Refresh()

	if s.onEnd != nil {
		s.onEnd()
	}
}

func (s *selectionOverlay) rectCoords() (fyne.Position, fyne.Position) {
	x1 := math.Min(float64(s.startPos.X), float64(s.curPos.X))
	y1 := math.Min(float64(s.startPos.Y), float64(s.curPos.Y))
	x2 := math.Max(float64(s.startPos.X), float64(s.curPos.X))
	y2 := math.Max(float64(s.startPos.Y), float64(s.curPos.Y))
	return fyne.NewPos(float32(x1), float32(y1)), fyne.NewPos(float32(x2), float32(y2))
}

func (s *selectionOverlay) refreshRect() {
	tl, br := s.rectCoords()
	s.rect.Move(tl)
	s.rect.Resize(fyne.NewSize(br.X-tl.X, br.Y-tl.Y))
}

// Ensure overlay passes Tapped events to children if needed, but Draggable usually coexists well.
// If we need to pass Tapped, we rely on Fyne's event bubbling.
// However, since we wrap the content, the content is a child.
// Fyne widgets don't automatically forward events to children if the parent handles them?
// Actually, `Dragged` is distinct.
// But if `selectionOverlay` is the top-level widget, does it block interaction?
// `BaseWidget` doesn't block by default unless it implements the interface.
// `selectionOverlay` implements `Draggable`.
// It does NOT implement `Tappable`. So taps should go through to children (if hit test passes).
// But wait, the renderer puts `s.rect` *after* `s.content`?
// If `s.rect` is hidden, it shouldn't block.
// If `s.rect` is shown, it might block, but we are dragging anyway.

// The issue might be that `s.content` is managed by the renderer.
// The `selectionOverlay` widget receives the events because it "is" the container.
// If we want the children to receive Taps, we need to make sure the overlay doesn't swallow them.
// By NOT implementing Tapped, we allow Fyne to find the child under the cursor that Implement Tapped.

// Important: If we wrap `container.NewScroll`, the scroll might handle Drag.
// If the Scroll container handles Drag (for scrolling), it might conflict with our selection drag.
// Standard `container.Scroll` handles Drag for panning on mobile, but on desktop it's usually scrollbar or wheel.
// On desktop, `Scroll` does NOT implement `Draggable` generally?
// Actually `Scroll` (widget) might. `container.Scroll` is a container.
// Let's verify if `Drag` conflicts with Scroll.
// Usually, we want:
// - Click on item -> Select item
// - Drag on background -> Rectangle Select
// - Drag on item -> Drag and drop (future) or Rectangle Select?
// Native file pickers: Drag starting on item usually starts a D&D operation. Drag starting on empty space starts Rectangle.
// Current implementation doesn't support D&D of files yet. So Drag on item can also be Rectangle Select for now, OR we ignore it.
// If I implement Drag on the Overlay, and the Item does NOT implement Drag, the Overlay gets it.
// Good.

type selectionOverlayRenderer struct {
	s *selectionOverlay
}

func (r *selectionOverlayRenderer) Layout(size fyne.Size) {
	r.s.content.Resize(size)
	r.s.content.Move(fyne.NewPos(0, 0))
}

func (r *selectionOverlayRenderer) MinSize() fyne.Size {
	return r.s.content.MinSize()
}

func (r *selectionOverlayRenderer) Refresh() {
	r.s.content.Refresh()
	r.s.rect.Refresh()
}

func (r *selectionOverlayRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.s.content}
	objs = append(objs, r.s.debugRects...)
	objs = append(objs, r.s.rect)
	return objs
}

func (r *selectionOverlayRenderer) Destroy() {}

// Custom timer wrapper for potential double-click logic if needed, but unnecessary here.
var _ fyne.Draggable = (*selectionOverlay)(nil)
