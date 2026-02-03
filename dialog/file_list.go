package dialog

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/FyshOS/fancyfs"
)

type fileList struct {
	picker FilePicker

	content *container.Scroll
	view    ViewLayout

	files        []fyne.URI
	filtered     []fyne.URI
	activeFilter string

	// Sorting
	sortOrder FileSortOrder

	// Cached widgets
	grid *widget.GridWrap
	list *widget.List
}

type FileSortOrder int

const (
	SortNameAsc FileSortOrder = iota
	SortNameDesc
	SortSizeAsc
	SortSizeDesc
	SortDateAsc
	SortDateDesc
)

func newFileList(p FilePicker) *fileList {
	f := &fileList{
		picker:    p,
		sortOrder: SortNameAsc,
	}

	f.grid = widget.NewGridWrap(
		func() int { return len(f.filtered) },
		func() fyne.CanvasObject { return newFileItem(f.picker) },
		func(id widget.GridWrapItemID, o fyne.CanvasObject) {
			item := o.(*fileItem)
			item.id = int(id)
			if item.id < len(f.filtered) {
				item.setURI(f.filtered[item.id], f.view)
				item.setSelected(f.picker.IsSelected(f.filtered[item.id]))
			}
		},
	)

	f.list = widget.NewList(
		func() int { return len(f.filtered) },
		func() fyne.CanvasObject { return newFileItem(f.picker) },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			item := o.(*fileItem)
			item.id = id
			if item.id < len(f.filtered) {
				item.setURI(f.filtered[item.id], f.view)
				item.setSelected(f.picker.IsSelected(f.filtered[item.id]))
			}
		},
	)

	f.content = container.NewScroll(nil)
	return f
}

func (f *fileList) setView(view ViewLayout) {
	f.view = view
	f.refresh()

	if f.view == GridView {
		GetThumbnailManager().PrewarmDirectory(f.files)
	}
}

func (f *fileList) setFiles(files []fyne.URI) {
	f.files = files
	f.filterAndSort()
	f.refresh()

	if f.view == GridView {
		GetThumbnailManager().PrewarmDirectory(f.files)
	}
}

func (f *fileList) filterAndSort() {
	f.filtered = make([]fyne.URI, len(f.files))
	copy(f.filtered, f.files)
	f.sort()
}

func (f *fileList) setFilter(filter string) {
	f.activeFilter = strings.ToLower(filter)
	if filter == "" {
		f.filtered = make([]fyne.URI, len(f.files))
		copy(f.filtered, f.files)
	} else {
		f.filtered = nil
		for _, file := range f.files {
			if strings.Contains(strings.ToLower(file.Name()), f.activeFilter) {
				f.filtered = append(f.filtered, file)
			}
		}
	}
	f.sort()
	f.refresh()
}

func (f *fileList) setSortOrder(order FileSortOrder) {
	f.sortOrder = order
	f.sort()
	f.refresh()
}

func (f *fileList) sort() {
	sort.Slice(f.filtered, func(i, j int) bool {
		iDir, _ := storage.CanList(f.filtered[i])
		jDir, _ := storage.CanList(f.filtered[j])
		if iDir != jDir {
			return iDir
		}

		u1, u2 := f.filtered[i], f.filtered[j]
		name1 := strings.ToLower(u1.Name())
		name2 := strings.ToLower(u2.Name())

		// Smart Sort when filtering
		if f.activeFilter != "" {
			prefix1 := strings.HasPrefix(name1, f.activeFilter)
			prefix2 := strings.HasPrefix(name2, f.activeFilter)
			if prefix1 != prefix2 {
				// True comes first (Starts with filter)
				return prefix1
			}
			// Fallback to name sort
			return name1 < name2
		}

		switch f.sortOrder {
		case SortNameDesc:
			return name1 > name2
		case SortSizeAsc:
			// Just fallback to name for simplicity or implement size if needed
			// Ideally we use size from Lister if available
			return name1 < name2
		case SortDateDesc:
			return name1 > name2
		default:
			return name1 < name2
		}
	})
}

func (f *fileList) refresh() {
	var target fyne.CanvasObject
	if f.view == GridView {
		target = f.grid
	} else {
		target = f.list
	}

	if f.content.Content == nil || !isPadded(f.content.Content, target) {
		f.content.Content = container.NewPadded(target)
	}

	f.content.Refresh()
	if f.view == GridView {
		f.grid.Refresh()
	} else {
		f.list.Refresh()
	}
}

func isPadded(o fyne.CanvasObject, inner fyne.CanvasObject) bool {
	if p, ok := o.(*fyne.Container); ok {
		return len(p.Objects) > 0 && p.Objects[0] == inner
	}
	return false
}

// Item Implementation

type fileItem struct {
	widget.BaseWidget
	picker FilePicker
	id     int
	uri    fyne.URI

	icon       *widget.FileIcon
	customIcon *widget.Icon
	thumbnail  *canvas.Image
	label      *widget.Label
	bg         *canvas.Rectangle

	currentPath string
	currentView ViewLayout
	lastClick   time.Time
	loadTimer   *time.Timer
}

func newFileItem(p FilePicker) *fileItem {
	item := &fileItem{
		picker:     p,
		icon:       widget.NewFileIcon(nil),
		customIcon: widget.NewIcon(nil),
		thumbnail:  canvas.NewImageFromImage(nil),
		label:      widget.NewLabel(""),
		bg:         canvas.NewRectangle(theme.SelectionColor()),
	}
	item.thumbnail.FillMode = canvas.ImageFillContain
	item.thumbnail.Hide()
	item.customIcon.Hide()
	item.bg.Hide()
	item.label.Truncation = fyne.TextTruncateEllipsis
	item.ExtendBaseWidget(item)
	return item
}

func (i *fileItem) CreateRenderer() fyne.WidgetRenderer {
	return &fileItemRenderer{item: i}
}

func (i *fileItem) setURI(u fyne.URI, view ViewLayout) {
	i.uri = u
	i.icon.SetURI(u)
	name := u.Name()

	if i.currentPath == u.Path() && i.currentView == view {
		return
	}
	i.currentPath = u.Path()
	i.currentView = view

	if view == GridView {
		i.label.Alignment = fyne.TextAlignCenter
		i.label.Wrapping = fyne.TextWrapBreak
		i.label.Truncation = fyne.TextTruncateClip

		// Max 3 lines. Strict measurement to ensure it fits.
		// Use a safeLimit as heuristic to avoid the edge case where wrapping creates a 4th line.
		safeLimit := float32(2.4) * fileIconCellWidth

		textSize := theme.TextSize()
		textStyle := i.label.TextStyle
		measure := func(s string) float32 {
			size, _ := fyne.CurrentApp().Driver().RenderedTextSize(s, textSize, textStyle, nil)
			return size.Width
		}

		if measure(name) > safeLimit {
			ext := filepath.Ext(name)
			dots := ".."

			dotsWidth := measure(dots)
			extWidth := measure(ext)

			// Available width for the start of the filename
			targetHeadWidth := safeLimit - dotsWidth - extWidth

			if targetHeadWidth > 0 {
				base := name[:len(name)-len(ext)]

				// Binary search for the maximum length of base that fits
				low, high := 0, len(base)
				best := 0
				for low <= high {
					mid := (low + high) / 2
					if measure(base[:mid]) <= targetHeadWidth {
						best = mid
						low = mid + 1
					} else {
						high = mid - 1
					}
				}
				name = base[:best] + dots + ext
			} else {
				// Fallback if extension is extremely long
				name = dots + ext
			}
		}
	} else {
		i.label.Alignment = fyne.TextAlignLeading
		i.label.Wrapping = fyne.TextWrapOff
		i.label.Truncation = fyne.TextTruncateEllipsis
	}
	i.label.SetText(name)

	// Thumbnail handling
	i.icon.Show()
	i.customIcon.Hide()
	i.thumbnail.Hide()
	i.thumbnail.Image = nil
	i.thumbnail.Refresh()

	// Check for fancy folder details
	if isDir, _ := storage.CanList(u); isDir {
		if details, err := fancyfs.DetailsForFolder(u); err == nil && details != nil {
			if details.BackgroundResource != nil {
				i.customIcon.SetResource(details.BackgroundResource)
				i.icon.Hide()
				i.customIcon.Show()
			}
			if details.BackgroundURI != nil {
				// We can treat this like a pre-existing thumbnail
				i.thumbnail.File = details.BackgroundURI.Path()
				i.thumbnail.FillMode = details.BackgroundFill
				i.thumbnail.Refresh()
				i.icon.Hide()
				i.customIcon.Hide()
				i.thumbnail.Show()
				return
			}
		}
	}

	if view == GridView {
		if i.loadTimer != nil {
			i.loadTimer.Stop()
		}

		// Try instant memory hit
		if img := GetThumbnailManager().LoadMemoryOnly(u.Path()); img != nil {
			i.thumbnail.Image = img.Image
			i.thumbnail.Refresh()
			i.icon.Hide()
			i.thumbnail.Show()
			return
		}

		i.loadTimer = time.AfterFunc(200*time.Millisecond, func() {
			GetThumbnailManager().Load(u, func(img *canvas.Image) {
				// Ensure thread safety for UI updates using fyne.Do (available since v2.6.0)
				fyne.Do(func() {
					if i.currentPath != u.Path() {
						return
					}
					if img != nil {
						i.thumbnail.Image = img.Image
						i.thumbnail.Refresh()
						i.icon.Hide()
						i.thumbnail.Show()
					}
				})
			})
		})
	}
}

func (i *fileItem) setSelected(selected bool) {
	if selected {
		i.bg.Show()
	} else {
		i.bg.Hide()
	}
	i.Refresh()
}

func (i *fileItem) Tapped(e *fyne.PointEvent) {
	if fyne.CurrentDevice().IsMobile() {
		i.picker.Select(i.id)
		return
	}

	now := time.Now()
	// Detect double click
	if now.Sub(i.lastClick) < fyne.CurrentApp().Driver().DoubleTapDelay() {
		// Follow symlinks: try to see if it's listable (folder or symlink to folder)
		if l, err := storage.ListerForURI(i.uri); err == nil {
			i.picker.SetLocation(l)
		} else {
			i.picker.Select(i.id)
			i.picker.OpenSelection()
		}
	}
	i.lastClick = now
}

var _ desktop.Mouseable = (*fileItem)(nil)

func (i *fileItem) MouseDown(*desktop.MouseEvent) {}
func (i *fileItem) MouseUp(e *desktop.MouseEvent) {
	if e.Button != desktop.MouseButtonPrimary {
		return
	}

	if e.Modifier&fyne.KeyModifierControl != 0 {
		i.picker.ToggleSelection(i.id)
	} else if e.Modifier&fyne.KeyModifierShift != 0 {
		i.picker.ExtendSelection(i.id)
	} else {
		i.picker.Select(i.id)
	}
}

func (i *fileItem) SecondaryTapped(e *fyne.PointEvent) {
	if !i.picker.IsMultiSelect() {
		return
	}

	label := lang.L("Select")
	if i.picker.IsSelected(i.uri) {
		label = lang.L("Deselect")
	}

	menuItem := fyne.NewMenuItem(label, func() {
		i.picker.ToggleSelection(i.id)
	})

	menu := fyne.NewMenu("", menuItem)
	widget.ShowPopUpMenuAtPosition(menu, fyne.CurrentApp().Driver().CanvasForObject(i), e.AbsolutePosition)
}

type fileItemRenderer struct {
	item *fileItem
}

func (r *fileItemRenderer) Layout(size fyne.Size) {
	r.item.bg.Resize(size)

	view := r.item.picker.GetView()

	if view == GridView {
		iconSize := fyne.NewSquareSize(fileIconSize)
		r.item.icon.Resize(iconSize)
		r.item.icon.Move(fyne.NewPos((size.Width-iconSize.Width)/2, theme.Padding()))

		r.item.customIcon.Resize(iconSize)
		r.item.customIcon.Move(fyne.NewPos((size.Width-iconSize.Width)/2, theme.Padding()))

		if r.item.thumbnail.Visible() {
			r.item.thumbnail.Resize(iconSize)
			r.item.thumbnail.Move(fyne.NewPos((size.Width-iconSize.Width)/2, theme.Padding()))
		}

		// Strictly cap height to exactly 3 lines of text
		// Use the theme's text size and style to measure a single line
		s, _ := fyne.CurrentApp().Driver().RenderedTextSize("A", theme.TextSize(), r.item.label.TextStyle, nil)
		lineHeight := s.Height
		textHeight := lineHeight * 4.0
		r.item.label.Resize(fyne.NewSize(size.Width, textHeight))
		r.item.label.Move(fyne.NewPos(0, iconSize.Height+theme.Padding()*1.5))
	} else {
		iconSize := fyne.NewSquareSize(fileInlineIconSize)
		r.item.icon.Resize(iconSize)
		r.item.icon.Move(fyne.NewPos(theme.Padding(), (size.Height-iconSize.Height)/2))

		r.item.customIcon.Resize(iconSize)
		r.item.customIcon.Move(fyne.NewPos(theme.Padding(), (size.Height-iconSize.Height)/2))

		labelSize := fyne.NewSize(size.Width-iconSize.Width-theme.Padding()*3, size.Height)
		r.item.label.Resize(labelSize)
		r.item.label.Move(fyne.NewPos(iconSize.Width+theme.Padding()*2, 0))
	}
}

func (r *fileItemRenderer) MinSize() fyne.Size {
	view := r.item.picker.GetView()
	if view == GridView {
		s, _ := fyne.CurrentApp().Driver().RenderedTextSize("A", theme.TextSize(), r.item.label.TextStyle, nil)
		lineHeight := s.Height
		return fyne.NewSize(fileIconCellWidth, fileIconSize+lineHeight*4.0+theme.Padding()*3.0)
	}

	iconSize := fileInlineIconSize
	textMin := r.item.label.MinSize()
	return fyne.NewSize(float32(iconSize)+textMin.Width+theme.Padding()*4, fyne.Max(float32(iconSize), textMin.Height+theme.Padding()))
}

func (r *fileItemRenderer) Refresh() {
	r.item.bg.Refresh()
	r.item.icon.Refresh()
	r.item.customIcon.Refresh()
	r.item.thumbnail.Refresh()
	r.item.label.Refresh()
}

func (r *fileItemRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.item.bg, r.item.icon, r.item.customIcon, r.item.thumbnail, r.item.label}
}

func (r *fileItemRenderer) Destroy() {
	if r.item.loadTimer != nil {
		r.item.loadTimer.Stop()
	}
}
