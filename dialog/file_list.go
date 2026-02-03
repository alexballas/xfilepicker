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
	grid    *widget.GridWrap
	list    *widget.List
	overlay *selectionOverlay

	// State for drag optimization and click guarding
	lastDragSelection []int
	lastDragTime      time.Time
	dragSelecting     bool

	dragStartContent fyne.Position
	dragCurViewport  fyne.Position

	autoScrollTicker *time.Ticker
	autoScrollStop   chan struct{}
	autoScrollDir    int
	autoScrollStep   float32
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

	f.overlay = newSelectionOverlay(nil, f.onSelectionDrag, f.onSelectionEnd)

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

	if f.content.Content == nil || !isPadded(f.content.Content, f.overlay) {
		f.overlay.content = target
		f.content.Content = container.NewPadded(f.overlay)
	} else {
		f.overlay.content = target
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
		bg:         canvas.NewRectangle(theme.Color(theme.ColorNameSelection)),
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

	// Guard against accidental clicks after drag
	if fd, ok := i.picker.(*fileDialog); ok {
		if fd.fileList.dragSelecting || time.Since(fd.fileList.lastDragTime) < 200*time.Millisecond {
			return
		}
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

func (i *fileItem) MouseDown(e *desktop.MouseEvent) {
	i.picker.DismissMenu()
}
func (i *fileItem) MouseUp(e *desktop.MouseEvent) {
	if e.Button == desktop.MouseButtonSecondary {
		if !i.picker.IsMultiSelect() {
			return
		}
		i.showContextMenu(e.Position) // Relative position
		return
	}

	if e.Button != desktop.MouseButtonPrimary {
		return
	}

	// Guard against accidental clicks after drag
	if fd, ok := i.picker.(*fileDialog); ok {
		if fd.fileList.dragSelecting || time.Since(fd.fileList.lastDragTime) < 200*time.Millisecond {
			return
		}
	}

	if e.Modifier&fyne.KeyModifierControl != 0 {
		i.picker.ToggleSelection(i.id)
	} else if e.Modifier&fyne.KeyModifierShift != 0 {
		i.picker.ExtendSelection(i.id)
	} else {
		i.picker.Select(i.id)
	}
}

func (i *fileItem) showContextMenu(pos fyne.Position) {
	label := lang.L("Select")
	if i.picker.IsSelected(i.uri) {
		label = lang.L("Deselect")
	}

	menuItem := fyne.NewMenuItem(label, func() {
		i.picker.ToggleSelection(i.id)
		i.picker.DismissMenu()
	})

	menu := fyne.NewMenu("", menuItem)
	i.picker.ShowMenu(menu, pos, i)
}

func (i *fileItem) SecondaryTapped(e *fyne.PointEvent) {
	if !i.picker.IsMultiSelect() {
		return
	}
	i.showContextMenu(e.Position)
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
	return calculateItemSize(view)
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

func (f *fileList) getItemSize() fyne.Size {
	return calculateItemSize(f.view)
}

func (f *fileList) onSelectionDrag(start, cur fyne.Position) {
	// Mark as actively drag-selecting so MouseUp handlers on items don't override selection.
	// This is important because on some platforms the MouseUp event can fire before DragEnd.
	dragStart := !f.dragSelecting
	f.dragSelecting = true

	if len(f.filtered) == 0 {
		return
	}

	f.dragCurViewport = cur
	if dragStart {
		offset := f.currentScrollOffset()
		f.dragStartContent = fyne.NewPos(start.X, start.Y+offset)
	}

	f.updateAutoScroll()
	f.updateDragSelection()
}

func (f *fileList) updateDragSelection() {
	if !f.dragSelecting || len(f.filtered) == 0 {
		return
	}

	itemSize := f.getItemSize()
	offset := f.currentScrollOffset()

	// Adjust the on-screen selection rectangle so it stays anchored to the original content start position
	// even as the list auto-scrolls.
	startViewportY := f.dragStartContent.Y - offset
	f.overlay.setStartPos(fyne.NewPos(f.dragStartContent.X, startViewportY))

	curContent := fyne.NewPos(f.dragCurViewport.X, f.dragCurViewport.Y+offset)

	tl := fyne.NewPos(min32(f.dragStartContent.X, curContent.X), min32(f.dragStartContent.Y, curContent.Y))
	br := fyne.NewPos(max32(f.dragStartContent.X, curContent.X), max32(f.dragStartContent.Y, curContent.Y))

	var ids []int
	if f.view == GridView {
		pad := f.grid.Theme().Size(theme.SizeNamePadding)

		cols := f.grid.ColumnCount()
		if cols < 1 {
			cols = 1
		}

		stepX := itemSize.Width + pad
		stepY := itemSize.Height + pad

		// Robust Logic:
		// 1. Calculate the range of rows that the rectangle touches.
		// 2. Iterate only through items in those rows.
		// 3. Perform strict intersection check.

		startRow := int(tl.Y / stepY)
		endRow := int(br.Y / stepY)

		// Clamp rows
		maxRow := (len(f.filtered) - 1) / cols
		if startRow < 0 {
			startRow = 0
		}
		if endRow > maxRow {
			endRow = maxRow
		}

		startCol := int(tl.X / stepX)
		endCol := int(br.X / stepX)
		if startCol < 0 {
			startCol = 0
		}
		if endCol > cols-1 {
			endCol = cols - 1
		}

		for row := startRow; row <= endRow; row++ {
			for col := startCol; col <= endCol; col++ {
				i := row*cols + col
				if i < 0 || i >= len(f.filtered) {
					continue
				}

				x1 := float32(col) * stepX
				y1 := float32(row) * stepY
				x2 := x1 + itemSize.Width
				y2 := y1 + itemSize.Height

				if x1 < br.X && x2 > tl.X && y1 < br.Y && y2 > tl.Y {
					ids = append(ids, i)
				}
			}
		}

	} else {
		// List View
		pad := f.list.Theme().Size(theme.SizeNamePadding)

		width := f.list.Size().Width
		height := itemSize.Height
		stepY := height + pad

		for i := 0; i < len(f.filtered); i++ {
			y1 := float32(i) * stepY
			y2 := y1 + height

			// In list view, width is full width
			if 0 < br.X && width > tl.X && y1 < br.Y && y2 > tl.Y {
				ids = append(ids, i)
			}
		}
	}

	// Optimization: check if selection actually changed
	if sameSelection(f.lastDragSelection, ids) {
		return
	}
	f.lastDragSelection = ids

	f.picker.SelectMultiple(ids)
}

func (f *fileList) onSelectionEnd() {
	f.stopAutoScroll()
	f.lastDragSelection = nil
	f.dragSelecting = false
	f.lastDragTime = time.Now()
	f.overlay.setDebugRects(nil)
}

func (f *fileList) currentScrollOffset() float32 {
	if f.view == GridView {
		return f.grid.GetScrollOffset()
	}
	return f.list.GetScrollOffset()
}

func (f *fileList) maxScrollOffset() float32 {
	if len(f.filtered) == 0 {
		return 0
	}

	itemSize := f.getItemSize()
	if f.view == GridView {
		pad := f.grid.Theme().Size(theme.SizeNamePadding)
		stepY := itemSize.Height + pad

		cols := f.grid.ColumnCount()
		if cols < 1 {
			cols = 1
		}
		rows := (len(f.filtered) + cols - 1) / cols
		total := float32(rows) * stepY
		max := total - f.grid.Size().Height
		if max < 0 {
			return 0
		}
		return max
	}

	pad := f.list.Theme().Size(theme.SizeNamePadding)
	stepY := itemSize.Height + pad
	total := float32(len(f.filtered)) * stepY
	max := total - f.list.Size().Height
	if max < 0 {
		return 0
	}
	return max
}

func (f *fileList) updateAutoScroll() {
	if !f.dragSelecting {
		f.stopAutoScroll()
		return
	}

	size := f.overlay.Size()
	if size.Height <= 0 {
		f.stopAutoScroll()
		return
	}

	zone := theme.Padding() * 4
	if zone < 24 {
		zone = 24
	}
	if zone > size.Height/2 {
		zone = size.Height / 2
	}

	var dir int
	var intensity float32
	if f.dragCurViewport.Y < zone {
		dir = -1
		intensity = (zone - f.dragCurViewport.Y) / zone
	} else if f.dragCurViewport.Y > size.Height-zone {
		dir = 1
		intensity = (f.dragCurViewport.Y - (size.Height - zone)) / zone
	}
	if intensity > 1 {
		intensity = 1
	}

	if dir == 0 || intensity <= 0 {
		f.stopAutoScroll()
		return
	}

	maxStep := f.getItemSize().Height * 0.5
	if maxStep < 12 {
		maxStep = 12
	}
	if maxStep > 80 {
		maxStep = 80
	}

	f.autoScrollDir = dir
	f.autoScrollStep = intensity * maxStep
	f.startAutoScroll()
}

func (f *fileList) startAutoScroll() {
	if f.autoScrollTicker != nil {
		return
	}
	f.autoScrollTicker = time.NewTicker(30 * time.Millisecond)
	f.autoScrollStop = make(chan struct{})

	stop := f.autoScrollStop
	ticker := f.autoScrollTicker
	go func() {
		for {
			select {
			case <-ticker.C:
				fyne.Do(func() {
					f.autoScrollTick()
				})
			case <-stop:
				return
			}
		}
	}()
}

func (f *fileList) stopAutoScroll() {
	if f.autoScrollTicker == nil {
		return
	}
	f.autoScrollTicker.Stop()
	f.autoScrollTicker = nil
	if f.autoScrollStop != nil {
		close(f.autoScrollStop)
		f.autoScrollStop = nil
	}
	f.autoScrollDir = 0
	f.autoScrollStep = 0
}

func (f *fileList) autoScrollTick() {
	if !f.dragSelecting || f.autoScrollDir == 0 || f.autoScrollStep <= 0 {
		f.stopAutoScroll()
		return
	}

	offset := f.currentScrollOffset()
	maxOffset := f.maxScrollOffset()
	if maxOffset <= 0 {
		f.stopAutoScroll()
		return
	}

	next := offset + float32(f.autoScrollDir)*f.autoScrollStep
	if next < 0 {
		next = 0
	} else if next > maxOffset {
		next = maxOffset
	}

	if next == offset {
		// Hit the end, no need to keep ticking.
		f.stopAutoScroll()
		return
	}

	if f.view == GridView {
		f.grid.ScrollToOffset(next)
	} else {
		f.list.ScrollToOffset(next)
	}

	// Scrolling changes the content coordinates of the current cursor position (viewport + offset),
	// so refresh selection while the pointer is held at the edge.
	f.updateDragSelection()
}

func min32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func max32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func sameSelection(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	// ids are appended in order in loop, so they should be sorted if grid traversal is consistent.
	// Our traversal (row/col or linear) produces sorted indices.
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
