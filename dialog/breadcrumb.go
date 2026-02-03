package dialog

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
)

type breadcrumb struct {
	picker  FilePicker
	content *fyne.Container
	scroll  *container.Scroll
}

func newBreadcrumb(p FilePicker) *breadcrumb {
	b := &breadcrumb{
		picker:  p,
		content: container.NewHBox(),
	}
	b.scroll = container.NewHScroll(container.NewPadded(b.content))
	return b
}

func (b *breadcrumb) update(dir fyne.ListableURI) {
	if b == nil || b.content == nil {
		return
	}
	b.content.Objects = nil
	current := dir

	// Helper to prevent infinite loops if something goes wrong with parents
	// But standard storage handles it.

	pathObjects := []fyne.CanvasObject{}

	for current != nil {
		pathURI := current
		btn := widget.NewButton(current.Name(), func() {
			b.picker.SetLocation(pathURI)
		})
		pathObjects = append(pathObjects, btn)

		parent, err := storage.Parent(current)
		if err != nil || parent == nil || parent.String() == current.String() {
			break
		}

		// Move up
		current = nil
		if l, err := storage.ListerForURI(parent); err == nil {
			current = l
		}
	}

	// Reverse
	for i := len(pathObjects) - 1; i >= 0; i-- {
		b.content.Add(pathObjects[i])
	}

	b.content.Refresh()
	// Scroll to end
	// Note: We need to wait for layout? For now just try setting offset.
	// b.scroll.Offset.X = 10000
}
