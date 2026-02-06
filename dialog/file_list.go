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
	zoom    float32

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

	lastGridViewportWidth float32
	gridCols              int
}

const gridColumnHysteresisPx float32 = 2.0

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
		zoom:      1.0,
	}

	f.overlay = newSelectionOverlay(nil, f.onSelectionDrag, f.onSelectionEnd)

	itemSize := func(view ViewLayout, zoom float32) fyne.Size {
		return f.itemSizeWithZoom(view, zoom)
	}

	f.grid = widget.NewGridWrap(
		func() int { return len(f.filtered) },
		func() fyne.CanvasObject { return newFileItem(f.picker, f.getZoom, itemSize) },
		func(id widget.GridWrapItemID, o fyne.CanvasObject) {
			item := o.(*fileItem)
			item.id = int(id)
			if item.id < len(f.filtered) {
				item.setURI(f.filtered[item.id], f.view)
				item.setSelected(f.picker.IsSelected(f.filtered[item.id]))
			}
		},
	)
	f.grid.StretchItems = true

	f.list = widget.NewList(
		func() int { return len(f.filtered) },
		func() fyne.CanvasObject { return newFileItem(f.picker, f.getZoom, itemSize) },
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

func (f *fileList) onResize() {
	if f == nil || f.view != GridView || f.grid == nil {
		return
	}

	width := f.gridViewportWidthForLayout()
	if width <= 0 {
		return
	}

	// Ignore tiny jitter to avoid resize-trigger loops.
	if abs32(width-f.lastGridViewportWidth) < 0.5 {
		return
	}

	// Capture scroll position as a ratio of max scroll before layout changes.
	// Using a ratio rather than an item ID works better when column count
	// changes significantly (which alters row positions).
	oldOffset := f.grid.GetScrollOffset()
	oldMax := f.maxScrollOffset()
	scrollRatio := float32(0)
	if oldMax > 0 {
		scrollRatio = oldOffset / oldMax
	}

	zoom := f.getZoom()
	f.recomputeGridCols(width, zoom)
	f.lastGridViewportWidth = width

	// GridWrap caches its column count and item MinSizes (which we make width-dependent
	// to stretch cells and avoid dead space). Force a recalculation on resize.
	// Refresh re-measures item MinSize (our items depend on viewport width); Resize clears its internal column cache.
	f.grid.Resize(f.grid.Size())

	// Restore scroll position using the same ratio.
	newMax := f.maxScrollOffset()
	if newMax > 0 && scrollRatio > 0 {
		targetOffset := scrollRatio * newMax
		f.grid.ScrollToOffset(targetOffset)
	}
}

func (f *fileList) getZoom() float32 {
	if f.zoom <= 0 {
		return 1.0
	}
	return f.zoom
}

func (f *fileList) setZoom(zoom float32) {
	if zoom <= 0 {
		zoom = 1.0
	}
	if f.zoom == zoom {
		return
	}

	// Zoom should be context-aware: keep the items currently in view centered.
	// We anchor on the item ID at the viewport center (grid uses center column),
	// then scroll so that same item remains at the viewport center after zoom.
	oldZoom := f.getZoom()
	view := f.view
	anchorID := f.centerAnchorID(view, oldZoom)

	f.zoom = zoom
	f.refresh()

	f.scrollCenterOnID(view, anchorID, zoom)
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
		f.lastGridViewportWidth = 0
		f.gridCols = 0
		// Ensure the grid is repainted even if we don't have a viewport size yet (e.g. before first layout).
		f.grid.Refresh()
		// Prime the grid sizing for the current viewport even if the window isn't being resized
		// (e.g. view toggle). This avoids transient column/scrollbar jitter.
		f.onResize()
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
	zoom   func() float32
	itemSz func(view ViewLayout, zoom float32) fyne.Size
	id     int
	uri    fyne.URI

	icon       *widget.FileIcon
	customIcon *widget.Icon
	thumbnail  *canvas.Image
	label      *widget.Label
	bg         *canvas.Rectangle

	rawName         string
	gridLabelWidth  float32
	gridTextSize    float32
	gridLabelQueued bool

	currentPath string
	currentView ViewLayout
	currentZoom float32
	lastClick   time.Time
	loadTimer   *time.Timer
}

func newFileItem(p FilePicker, zoom func() float32, itemSize func(view ViewLayout, zoom float32) fyne.Size) *fileItem {
	item := &fileItem{
		picker:     p,
		zoom:       zoom,
		itemSz:     itemSize,
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

func (i *fileItem) zoomScale() float32 {
	if i.zoom == nil {
		return 1.0
	}
	z := i.zoom()
	if z <= 0 {
		return 1.0
	}
	return z
}

func (i *fileItem) setURI(u fyne.URI, view ViewLayout) {
	zoom := i.zoomScale()
	path := ""
	if u != nil {
		path = u.Path()
	}

	// Fast path: avoid re-doing expensive work (icon/thumbnail resets, timers) during resize/layout churn.
	// Grid/list virtualization can call UpdateItem repeatedly even when the underlying URI hasn't changed.
	if i.currentPath == path && i.currentView == view && i.currentZoom == zoom {
		i.uri = u
		return
	}

	i.uri = u
	i.icon.SetURI(u)
	i.rawName = u.Name()
	name := i.rawName

	i.currentPath = path
	i.currentView = view
	i.currentZoom = zoom

	if view == GridView {
		i.label.Alignment = fyne.TextAlignCenter
		// We manually wrap with '\n' so we can keep file extensions intact.
		i.label.Wrapping = fyne.TextWrapOff
		i.label.Truncation = fyne.TextTruncateClip

		cellWidth := float32(fileIconCellWidth) * zoom
		if i.itemSz != nil {
			if s := i.itemSz(GridView, zoom); s.Width > 0 {
				cellWidth = s.Width
			}
		}
		name = formatGridFileName(name, cellWidth, i.label.TextStyle)
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
	i.thumbnail.File = ""
	i.thumbnail.Resource = nil
	i.thumbnail.FillMode = canvas.ImageFillContain

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
			i.thumbnail.File = ""
			i.thumbnail.Resource = nil
			i.thumbnail.FillMode = canvas.ImageFillContain
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
						i.thumbnail.File = ""
						i.thumbnail.Resource = nil
						i.thumbnail.FillMode = canvas.ImageFillContain
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

func formatGridFileName(name string, width float32, style fyne.TextStyle) string {
	if name == "" || width <= 0 {
		return name
	}

	// Safety margin to avoid clipping due to rounding differences between
	// RenderedTextSize measurements and actual rendering.
	// Increased to 4x Padding to be absolutely safe against clipping.
	width = max32(width-theme.Padding()*4, 0)

	textSize := theme.TextSize()
	measure := func(s string) float32 {
		size, _ := fyne.CurrentApp().Driver().RenderedTextSize(s, textSize, style, nil)
		return size.Width
	}
	return formatGridFileNameWithMeasure(name, width, measure)
}

func formatGridFileNameWithMeasure(name string, width float32, measure func(string) float32) string {
	if name == "" || width <= 0 {
		return name
	}

	const maxLines = 3

	// If the full name fits on one line, keep it as-is.
	if measure(name) <= width {
		return name
	}

	// Only "protect" extensions when there's a base name to show.
	ext := filepath.Ext(name)
	extText := strings.TrimPrefix(ext, ".")
	base := strings.TrimSuffix(name, ext)
	if ext == "" || base == "" {
		// No extension or just an extension (like ".bashrc") - wrap across lines if needed.
		return wrapTextToLines(name, width, maxLines, measure)
	}

	// Let the full name flow naturally across 3 lines first. This avoids forcing
	// the extension onto an extra line when earlier lines still have room.
	if lines, ok := wrapTextToLinesStrict(name, width, maxLines, measure); ok {
		return strings.Join(lines, "\n")
	}

	// If we need truncation, keep the extension visible and truncate the base
	// from right-to-left, inserting "..." directly before the extension.
	const dots = "..."
	truncSuffix := dots + extText
	baseRunes := []rune(base)
	for keep := len(baseRunes) - 1; keep >= 0; keep-- {
		candidate := string(baseRunes[:keep]) + truncSuffix
		if lines, ok := wrapTextToLinesStrict(candidate, width, maxLines, measure); ok {
			return strings.Join(lines, "\n")
		}
	}

	// Extremely narrow columns: show as much of the truncation suffix as possible.
	if lines, ok := wrapTextToLinesStrict(truncSuffix, width, maxLines, measure); ok {
		return strings.Join(lines, "\n")
	}

	return wrapTextToLines(truncSuffix, width, maxLines, measure)
}

// wrapTextToLinesStrict wraps text across multiple lines and reports whether the
// full text fits in at most maxLines lines (no truncation).
func wrapTextToLinesStrict(text string, width float32, maxLines int, measure func(string) float32) ([]string, bool) {
	if text == "" || width <= 0 || maxLines <= 0 {
		return []string{text}, false
	}

	lines := make([]string, 0, maxLines)
	remaining := text
	for len(lines) < maxLines && remaining != "" {
		head := fitPrefixByWidth(remaining, width, measure)
		if head == "" {
			return lines, false
		}
		lines = append(lines, head)
		remaining = strings.TrimPrefix(remaining, head)
	}

	return lines, remaining == ""
}

// wrapTextToLines wraps text across multiple lines, each fitting within width.
// The last line is truncated from the start (suffix-fit) if needed to ensure visibility.
func wrapTextToLines(text string, width float32, maxLines int, measure func(string) float32) string {
	if text == "" || width <= 0 || maxLines <= 0 {
		return text
	}
	if measure(text) <= width {
		return text
	}

	lines := make([]string, 0, maxLines)
	remaining := text

	for len(lines) < maxLines && remaining != "" {
		if len(lines) == maxLines-1 {
			// Last line: fit suffix to ensure end is visible.
			line := fitSuffixByWidth(remaining, width, measure)
			if line != "" {
				lines = append(lines, line)
			}
			break
		}
		head := fitPrefixByWidth(remaining, width, measure)
		if head == "" {
			// Can't fit anything, just take the suffix on last line.
			lines = append(lines, fitSuffixByWidth(remaining, width, measure))
			break
		}
		lines = append(lines, head)
		remaining = strings.TrimPrefix(remaining, head)
	}

	return strings.Join(lines, "\n")
}

func fitPrefixByWidth(s string, width float32, measure func(string) float32) string {
	if s == "" || width <= 0 {
		return ""
	}
	if measure(s) <= width {
		return s
	}

	runes := []rune(s)
	low, high := 0, len(runes)
	best := 0
	for low <= high {
		mid := (low + high) / 2
		if mid == 0 {
			low = 1
			continue
		}
		if measure(string(runes[:mid])) <= width {
			best = mid
			low = mid + 1
		} else {
			high = mid - 1
		}
	}
	if best == 0 {
		return ""
	}
	return string(runes[:best])
}

func fitSuffixByWidth(s string, width float32, measure func(string) float32) string {
	if s == "" || width <= 0 {
		return ""
	}
	if measure(s) <= width {
		return s
	}

	runes := []rune(s)
	low, high := 0, len(runes)
	bestStart := len(runes)
	for low <= high {
		mid := (low + high) / 2
		if mid >= len(runes) {
			break
		}
		if measure(string(runes[mid:])) <= width {
			bestStart = mid
			high = mid - 1
		} else {
			low = mid + 1
		}
	}
	return string(runes[bestStart:])
}

type fileItemRenderer struct {
	item *fileItem
}

func (r *fileItemRenderer) Layout(size fyne.Size) {
	r.item.bg.Resize(size)

	view := r.item.picker.GetView()
	zoom := r.item.zoomScale()

	if view == GridView {
		iconSize := fyne.NewSquareSize(float32(fileIconSize) * zoom)
		r.item.icon.Resize(iconSize)
		r.item.icon.Move(fyne.NewPos((size.Width-iconSize.Width)/2, theme.Padding()))

		r.item.customIcon.Resize(iconSize)
		r.item.customIcon.Move(fyne.NewPos((size.Width-iconSize.Width)/2, theme.Padding()))

		if r.item.thumbnail.Visible() {
			r.item.thumbnail.Resize(iconSize)
			r.item.thumbnail.Move(fyne.NewPos((size.Width-iconSize.Width)/2, theme.Padding()))
		}

		// Size the label using the available height so the last line (extension)
		// never gets clipped due to rounding/padding differences.
		labelY := iconSize.Height + theme.Padding()*2
		labelH := size.Height - labelY - theme.Padding()
		if labelH < 0 {
			labelH = 0
		}
		r.item.label.Resize(fyne.NewSize(size.Width, labelH))
		r.item.label.Move(fyne.NewPos(0, labelY))

		// Recompute label text for the current width. This is important when the grid
		// flexes item widths during resize (e.g. when a column is added/removed), so
		// we don't end up with wrapped/clipped extensions.
		r.item.ensureGridLabel(size.Width)

	} else {
		iconSize := fyne.NewSquareSize(float32(fileInlineIconSize) * zoom)
		r.item.icon.Resize(iconSize)
		r.item.icon.Move(fyne.NewPos(theme.Padding(), (size.Height-iconSize.Height)/2))

		r.item.customIcon.Resize(iconSize)
		r.item.customIcon.Move(fyne.NewPos(theme.Padding(), (size.Height-iconSize.Height)/2))

		labelSize := fyne.NewSize(size.Width-iconSize.Width-theme.Padding()*3, size.Height)
		r.item.label.Resize(labelSize)
		r.item.label.Move(fyne.NewPos(iconSize.Width+theme.Padding()*2, 0))

	}
}

func (i *fileItem) ensureGridLabel(width float32) {
	if i == nil || i.label == nil || i.currentView != GridView || i.rawName == "" || width <= 0 {
		return
	}

	// Avoid churn during continuous resize.
	if abs32(width-i.gridLabelWidth) < 1.0 && i.gridTextSize == theme.TextSize() {
		return
	}

	// Defer updates from layout callbacks to avoid re-entrant layout panics.
	if i.gridLabelQueued {
		return
	}
	i.gridLabelQueued = true
	fyne.Do(func() {
		defer func() { i.gridLabelQueued = false }()

		if i == nil || i.label == nil || i.currentView != GridView || i.rawName == "" {
			return
		}
		curWidth := i.label.Size().Width
		if curWidth <= 0 {
			return
		}
		curTextSize := theme.TextSize()
		if abs32(curWidth-i.gridLabelWidth) < 1.0 && i.gridTextSize == curTextSize {
			return
		}

		newText := formatGridFileName(i.rawName, curWidth, i.label.TextStyle)
		i.gridLabelWidth = curWidth
		i.gridTextSize = curTextSize
		if i.label.Text != newText {
			i.label.SetText(newText)
		}
	})
}

func (r *fileItemRenderer) MinSize() fyne.Size {
	view := r.item.picker.GetView()
	zoom := r.item.zoomScale()
	// Return stable base size. Fyne's GridWrap.StretchItems handles stretching
	// at layout time to avoid feedback loops.
	return calculateItemSizeWithZoom(view, zoom)
}

func (r *fileItemRenderer) Refresh() {
	r.item.bg.Refresh()
	r.item.icon.Refresh()
	r.item.customIcon.Refresh()
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
	return f.itemSizeWithZoom(f.view, f.getZoom())
}

func (f *fileList) itemSizeWithZoom(view ViewLayout, zoom float32) fyne.Size {
	// Return stable base size. Fyne's GridWrap.StretchItems handles stretching at layout time.
	return calculateItemSizeWithZoom(view, zoom)
}

func (f *fileList) gridViewportWidthForLayout() float32 {
	// When the GridWrap is embedded in our outer Scroll (for clipping/overlay),
	// its Size() can temporarily reflect the scroll content size (MinSize) during
	// layout churn. That creates a feedback loop where item MinSize depends on
	// grid Size and scrollbars can oscillate. Use the viewport (Scroll) width
	// instead when available.
	if f != nil && f.content != nil {
		w := f.content.Size().Width
		if w > 0 {
			// content is wrapped in container.NewPadded(...)
			w -= theme.Padding() * 2
			if w > 0 {
				return w
			}
		}
	}
	if f != nil && f.grid != nil {
		return f.grid.Size().Width
	}
	return 0
}

func (f *fileList) recomputeGridCols(viewportWidth float32, zoom float32) {
	if f == nil || f.grid == nil || viewportWidth <= 0 {
		return
	}

	base := calculateItemSizeWithZoom(GridView, zoom)
	pad := f.grid.Theme().Size(theme.SizeNamePadding)
	if pad < 0 {
		pad = 0
	}

	candidate := gridColumnCount(viewportWidth, base.Width, pad)
	if candidate < 1 {
		candidate = 1
	}

	cur := f.gridCols
	if cur < 1 {
		f.gridCols = candidate
		return
	}

	requiredWidth := func(cols int) float32 {
		if cols < 1 {
			return 0
		}
		return float32(cols)*base.Width + float32(cols-1)*pad
	}

	// If current column count no longer fits at base width, we must reduce immediately.
	if viewportWidth < requiredWidth(cur) {
		f.gridCols = candidate
		return
	}

	// Apply hysteresis around the thresholds to prevent rapid toggling.
	switch {
	case candidate > cur:
		next := requiredWidth(cur + 1)
		if viewportWidth < next+gridColumnHysteresisPx {
			candidate = cur
		}
	case candidate < cur:
		this := requiredWidth(cur)
		if viewportWidth > this-gridColumnHysteresisPx {
			candidate = cur
		}
	}

	f.gridCols = candidate
}

func (f *fileList) centerAnchorID(view ViewLayout, zoom float32) int {
	if len(f.filtered) == 0 {
		return 0
	}

	offset := f.currentScrollOffset()

	switch view {
	case GridView:
		if f.grid == nil {
			return 0
		}
		viewport := f.grid.Size()
		pad := f.grid.Theme().Size(theme.SizeNamePadding)
		itemSize := f.itemSizeWithZoom(GridView, zoom)

		cols := gridColumnCount(viewport.Width, itemSize.Width, pad)
		stepX := itemSize.Width + pad
		stepY := itemSize.Height + pad

		centerX := viewport.Width / 2
		centerY := offset + viewport.Height/2

		row := int(centerY / stepY)
		col := int(centerX / stepX)
		id := row*cols + col
		return clampIndex(id, len(f.filtered))
	default:
		if f.list == nil {
			return 0
		}
		viewport := f.list.Size()
		pad := f.list.Theme().Size(theme.SizeNamePadding)
		itemSize := calculateItemSizeWithZoom(ListView, zoom)

		stepY := itemSize.Height + pad
		centerY := offset + viewport.Height/2

		id := int(centerY / stepY)
		return clampIndex(id, len(f.filtered))
	}
}

func (f *fileList) scrollCenterOnID(view ViewLayout, id int, zoom float32) {
	if len(f.filtered) == 0 {
		return
	}
	id = clampIndex(id, len(f.filtered))

	switch view {
	case GridView:
		if f.grid == nil {
			return
		}

		viewport := f.grid.Size()
		pad := f.grid.Theme().Size(theme.SizeNamePadding)
		itemSize := f.itemSizeWithZoom(GridView, zoom)

		// Force column count recalculation for the new item width.
		f.grid.Resize(viewport)

		cols := gridColumnCount(viewport.Width, itemSize.Width, pad)
		if cols < 1 {
			cols = 1
		}
		stepY := itemSize.Height + pad
		rows := (len(f.filtered) + cols - 1) / cols
		contentHeight := float32(rows)*stepY - pad

		row := id / cols
		desiredCenterY := float32(row)*stepY + itemSize.Height/2
		targetOffset := desiredCenterY - viewport.Height/2
		targetOffset = clampOffset(targetOffset, contentHeight-viewport.Height)

		f.grid.ScrollToOffset(targetOffset)
	default:
		if f.list == nil {
			return
		}

		viewport := f.list.Size()
		pad := f.list.Theme().Size(theme.SizeNamePadding)
		itemSize := calculateItemSizeWithZoom(ListView, zoom)

		f.list.Resize(viewport)

		stepY := itemSize.Height + pad
		contentHeight := float32(len(f.filtered))*stepY - pad

		desiredCenterY := float32(id)*stepY + itemSize.Height/2
		targetOffset := desiredCenterY - viewport.Height/2
		targetOffset = clampOffset(targetOffset, contentHeight-viewport.Height)

		f.list.ScrollToOffset(targetOffset)
	}
}

func clampIndex(i int, length int) int {
	if length <= 0 {
		return 0
	}
	if i < 0 {
		return 0
	}
	if i >= length {
		return length - 1
	}
	return i
}

func clampOffset(offset, max float32) float32 {
	if offset < 0 {
		return 0
	}
	if max < 0 {
		return 0
	}
	if offset > max {
		return max
	}
	return offset
}

func gridColumnCount(width, itemWidth, padding float32) int {
	if itemWidth <= 0 {
		return 1
	}
	cols := 1
	if width > itemWidth {
		cols = int((width + padding) / (itemWidth + padding))
		if cols < 1 {
			cols = 1
		}
	}
	return cols
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

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
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
