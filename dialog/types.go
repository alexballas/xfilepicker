package dialog

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
)

// ViewLayout can be passed to SetView() to set the view of
// a FileDialog
type ViewLayout int

const (
	defaultView ViewLayout = iota
	// ListView lists files in a vertical list
	ListView
	// GridView lists files in a grid
	GridView
)

const (
	fileIconSize       = 64
	fileInlineIconSize = 24
	fileIconCellWidth  = fileIconSize * 1.8 // Increased from 1.6 to better fit 3 lines
	viewLayoutKey      = "fyne:fileDialogViewLayout"
	ffmpegPathKey      = "fyne:fileDialogFFmpegPath"
	showHiddenKey      = "fyne:fileDialogShowHidden"
)

type favoriteItem struct {
	locName string
	locIcon fyne.Resource
	loc     fyne.ListableURI
}

// FilePicker is the main interface for the file dialog logic
type FilePicker interface {
	SetLocation(dir fyne.ListableURI)
	Refresh()
	SetView(view ViewLayout)
	GetView() ViewLayout
	Select(id int)
	ToggleSelection(id int)
	ExtendSelection(id int)
	IsSelected(uri fyne.URI) bool
	OpenSelection()
	SetFilter(filter storage.FileFilter)
	IsMultiSelect() bool
	ShowMenu(menu *fyne.Menu, pos fyne.Position, obj fyne.CanvasObject)
	DismissMenu()
}
