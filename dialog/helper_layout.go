package dialog

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

func calculateItemSize(view ViewLayout) fyne.Size {
	// Standard Text Height Measurement
	// We use "A" as a representative character for line height
	s, _ := fyne.CurrentApp().Driver().RenderedTextSize("A", theme.TextSize(), fyne.TextStyle{}, nil)
	lineHeight := s.Height

	if view == GridView {
		// Grid View: Fixed Cell Width, Height based on Icon + 3.5 lines of text + padding
		// Note: We use 3.0 padding to match legacy renderer logic, ensuring loose fit
		return fyne.NewSize(fileIconCellWidth, fileIconSize+lineHeight*3.5+theme.Padding()*3.0)
	}

	// List View:
	// Height: Icon or Text Height + Padding
	iconSize := fileInlineIconSize
	textMinHeight := lineHeight
	// Height is Max(icon, text) + inner padding?
	// Renderer min sizes usually include padding.
	// We use standard list item sizing heuristics here.
	return fyne.NewSize(0, fyne.Max(float32(iconSize), textMinHeight+theme.Padding()))
}
