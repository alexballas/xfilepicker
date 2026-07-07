package dialog

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alexballas/refyne/v2"
	"github.com/alexballas/refyne/v2/canvas"
	"github.com/alexballas/refyne/v2/container"
	"github.com/alexballas/refyne/v2/driver/desktop"
	"github.com/alexballas/refyne/v2/fancyfs"
	"github.com/alexballas/refyne/v2/lang"
	"github.com/alexballas/refyne/v2/storage"
	"github.com/alexballas/refyne/v2/theme"
	"github.com/alexballas/refyne/v2/widget"
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
	grid    *pageGridWrap
	list    *pageList
	overlay *selectionOverlay

	// State for drag optimization and click guarding
	lastDragSelection []int
	lastDragTime      time.Time
	dragSelecting     bool

	dragStartContent fyne.Position
	dragCurViewport  fyne.Position

	autoScrollAnim     *fyne.Animation
	autoScrollDir      int
	autoScrollVelocity float32
	autoScrollLastTick time.Time

	lastGridViewportWidth float32
	gridCols              int
	keyboardFocus         int
}

const (
	gridColumnHysteresisPx float32 = 2.0
	autoScrollBaseFrame            = 30 * time.Millisecond
	autoScrollMaxFrame             = 50 * time.Millisecond
)

type FileSortOrder int

const (
	SortNameAsc FileSortOrder = iota
	SortNameDesc
	SortSizeAsc
	SortSizeDesc
	SortDateAsc
	SortDateDesc

	SortFirstModified = SortDateAsc
	SortLastModified  = SortDateDesc
)

type fileSortKey struct {
	name       string
	isDir      bool
	modTime    time.Time
	hasModTime bool
}

func newFileList(p FilePicker) *fileList {
	f := &fileList{
		picker:        p,
		sortOrder:     SortNameAsc,
		zoom:          1.0,
		keyboardFocus: 0,
	}

	f.overlay = newSelectionOverlay(nil, f.onSelectionDrag, f.onSelectionEnd)

	itemSize := func(view ViewLayout, zoom float32) fyne.Size {
		return f.itemSizeWithZoom(view, zoom)
	}

	f.grid = newPageGridWrap(
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
		f.pageFromKeyboard,
		f.selectAllFromShortcut,
	)
	f.grid.StretchItems = true
	f.grid.OnSelected = func(id widget.GridWrapItemID) {
		// Space on the keyboard-focused item triggers the GridWrap's own
		// single-item selection. Discard that internal highlight and route the
		// intent through the picker so keyboard and mouse share one selection
		// state (and one highlight, drawn by fileItem). See selectFromKeyboard.
		f.grid.UnselectAll()
		f.selectFromKeyboard(int(id))
	}
	f.grid.OnReturn = func(widget.GridWrapItemID) { f.confirmFromKeyboard() }
	f.grid.OnKeyboardNavigated = f.navigateFromKeyboard
	f.grid.OnTypedRune = f.searchFromKeyboard
	f.grid.OnEscape = f.cancelSearchFromKeyboard

	f.list = newPageList(
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
		f.pageFromKeyboard,
		f.selectAllFromShortcut,
	)
	f.list.OnSelected = func(id widget.ListItemID) {
		// See the GridWrap OnSelected above: keep keyboard selection unified
		// with the picker and avoid a stale widget-owned highlight.
		f.list.UnselectAll()
		f.selectFromKeyboard(int(id))
	}
	f.list.OnReturn = func(widget.ListItemID) { f.confirmFromKeyboard() }
	f.list.OnKeyboardNavigated = f.navigateFromKeyboard
	f.list.OnTypedRune = f.searchFromKeyboard
	f.list.OnEscape = f.cancelSearchFromKeyboard

	f.content = container.NewScroll(nil)
	return f
}

type pageList struct {
	widget.List
	onPageKey   func(fyne.KeyName, fyne.KeyModifier)
	onSelectAll func()
}

func newPageList(length func() int, createItem func() fyne.CanvasObject, updateItem func(widget.ListItemID, fyne.CanvasObject), onPageKey func(fyne.KeyName, fyne.KeyModifier), onSelectAll func()) *pageList {
	l := &pageList{onPageKey: onPageKey, onSelectAll: onSelectAll}
	l.Length = length
	l.CreateItem = createItem
	l.UpdateItem = updateItem
	l.ExtendBaseWidget(l)
	return l
}

func (l *pageList) CreateRenderer() fyne.WidgetRenderer {
	r := l.List.CreateRenderer()
	l.ExtendBaseWidget(l)
	return r
}

func (l *pageList) MinSize() fyne.Size {
	size := l.List.MinSize()
	l.ExtendBaseWidget(l)
	return size
}

func (l *pageList) TypedKey(event *fyne.KeyEvent) {
	if l.handlePageKey(event.Name, currentKeyboardModifiers()) {
		return
	}
	l.List.TypedKey(event)
}

func (l *pageList) KeyDown(event *fyne.KeyEvent) {
	modifiers := currentKeyboardModifiers()
	if modifiers&fyne.KeyModifierControl == 0 {
		return
	}
	l.handlePageKey(event.Name, modifiers)
}

func (l *pageList) KeyUp(*fyne.KeyEvent) {}

func (l *pageList) TypedShortcut(shortcut fyne.Shortcut) {
	if _, ok := shortcut.(*fyne.ShortcutSelectAll); ok {
		if l.onSelectAll != nil {
			l.onSelectAll()
		}
		return
	}
}

func (l *pageList) handlePageKey(name fyne.KeyName, modifiers fyne.KeyModifier) bool {
	if name != fyne.KeyPageUp && name != fyne.KeyPageDown {
		return false
	}
	if l.onPageKey != nil {
		l.onPageKey(name, modifiers)
	}
	return true
}

type pageGridWrap struct {
	widget.GridWrap
	onPageKey   func(fyne.KeyName, fyne.KeyModifier)
	onSelectAll func()
}

func newPageGridWrap(length func() int, createItem func() fyne.CanvasObject, updateItem func(widget.GridWrapItemID, fyne.CanvasObject), onPageKey func(fyne.KeyName, fyne.KeyModifier), onSelectAll func()) *pageGridWrap {
	g := &pageGridWrap{onPageKey: onPageKey, onSelectAll: onSelectAll}
	g.Length = length
	g.CreateItem = createItem
	g.UpdateItem = updateItem
	g.ExtendBaseWidget(g)
	return g
}

func (g *pageGridWrap) CreateRenderer() fyne.WidgetRenderer {
	r := g.GridWrap.CreateRenderer()
	g.ExtendBaseWidget(g)
	return r
}

func (g *pageGridWrap) MinSize() fyne.Size {
	size := g.GridWrap.MinSize()
	g.ExtendBaseWidget(g)
	return size
}

func (g *pageGridWrap) TypedKey(event *fyne.KeyEvent) {
	if g.handlePageKey(event.Name, currentKeyboardModifiers()) {
		return
	}
	g.GridWrap.TypedKey(event)
}

func (g *pageGridWrap) KeyDown(event *fyne.KeyEvent) {
	modifiers := currentKeyboardModifiers()
	if modifiers&fyne.KeyModifierControl == 0 {
		return
	}
	g.handlePageKey(event.Name, modifiers)
}

func (g *pageGridWrap) KeyUp(*fyne.KeyEvent) {}

func (g *pageGridWrap) TypedShortcut(shortcut fyne.Shortcut) {
	if _, ok := shortcut.(*fyne.ShortcutSelectAll); ok {
		if g.onSelectAll != nil {
			g.onSelectAll()
		}
		return
	}
}

func (g *pageGridWrap) handlePageKey(name fyne.KeyName, modifiers fyne.KeyModifier) bool {
	if name != fyne.KeyPageUp && name != fyne.KeyPageDown {
		return false
	}
	if g.onPageKey != nil {
		g.onPageKey(name, modifiers)
	}
	return true
}

func currentKeyboardModifiers() fyne.KeyModifier {
	if app := fyne.CurrentApp(); app != nil {
		if drv, ok := app.Driver().(desktop.Driver); ok {
			return drv.CurrentKeyModifiers()
		}
	}
	return 0
}

var (
	_ desktop.Keyable   = (*pageList)(nil)
	_ desktop.Keyable   = (*pageGridWrap)(nil)
	_ fyne.Shortcutable = (*pageList)(nil)
	_ fyne.Shortcutable = (*pageGridWrap)(nil)
)

// selectFromKeyboard routes a selection originating from the underlying
// GridWrap/List keyboard handling (Space on the focused item) through the
// picker, so keyboard and mouse share a single selection state. In
// multi-select mode Space toggles the focused item; otherwise it replaces the
// current selection (ToggleSelection falls back to Select when not
// multi-selecting).
func (f *fileList) selectFromKeyboard(id int) {
	if id < 0 || id >= len(f.filtered) {
		return
	}
	f.keyboardFocus = id
	f.picker.ToggleSelection(id)
}

func (f *fileList) selectAllFromShortcut() {
	if fd, ok := f.picker.(*fileDialog); ok {
		fd.SelectAll()
	}
}

// searchFromKeyboard forwards a printable character typed while the list/grid
// holds focus to the dialog's type-to-search box. Selecting a file focuses the
// list/grid (so arrow keys keep working), but a focused list/grid otherwise
// swallows typed runes: the canvas-level rune hook only fires when nothing is
// focused, so without this the user would have to click the search box manually
// after selecting. Space is skipped because the list/grid binds it to toggling
// the focused item's selection.
func (f *fileList) searchFromKeyboard(r rune) {
	if r == ' ' {
		return
	}
	if fd, ok := f.picker.(*fileDialog); ok {
		fd.appendRuneToSearch(r)
	}
}

// navigateFromKeyboard handles arrow-key navigation reported by the underlying
// GridWrap/List. The widget has already moved its highlight cursor to id and
// scrolled it into view; here we mirror the move into the picker's selection so
// keyboard navigation matches mouse behaviour, taking the held modifiers into
// account (Nautilus/GNOME Files style):
//
//   - plain arrow: select the newly focused item, replacing the selection,
//     exactly as a single click would.
//   - Shift+arrow: extend the selection range from the anchor to the newly
//     focused item, enabling multi-selection.
//   - Ctrl+arrow: move the focus cursor only, leaving the selection untouched.
func (f *fileList) navigateFromKeyboard(id int, modifiers fyne.KeyModifier) {
	if id < 0 || id >= len(f.filtered) {
		return
	}

	f.keyboardFocus = id
	switch {
	case modifiers&fyne.KeyModifierShift != 0:
		f.picker.ExtendSelection(id)
	case modifiers&fyne.KeyModifierControl != 0:
		// Focus-only move: the widget already repainted its highlight cursor, so
		// there is nothing to do to the picker selection.
	default:
		f.picker.Select(id)
	}
}

func (f *fileList) pageFromKeyboard(name fyne.KeyName, modifiers fyne.KeyModifier) {
	if len(f.filtered) == 0 {
		return
	}

	current := clampIndex(f.keyboardFocus, len(f.filtered))
	target := f.pageNavigationTarget(current, name)
	if target == current {
		return
	}

	f.setKeyboardHighlight(target)
	f.navigateFromKeyboard(target, modifiers)
}

func (f *fileList) setKeyboardHighlight(id int) {
	if id < 0 || id >= len(f.filtered) {
		return
	}

	if f.view == GridView && f.grid != nil {
		f.grid.SetHighlight(id)
	} else if f.list != nil {
		f.list.SetHighlight(id)
	}
	f.keyboardFocus = id
}

func (f *fileList) pageNavigationTarget(current int, name fyne.KeyName) int {
	if len(f.filtered) == 0 {
		return 0
	}

	delta := f.pageNavigationDelta()
	if name == fyne.KeyPageUp {
		delta = -delta
	}
	return clampIndex(current+delta, len(f.filtered))
}

func (f *fileList) pageNavigationDelta() int {
	if f.view == GridView {
		return f.gridPageDelta()
	}
	return f.listPageDelta()
}

func (f *fileList) listPageDelta() int {
	itemSize := f.itemSizeWithZoom(ListView, f.getZoom())
	stepY := itemSize.Height + theme.Padding()
	if f.list != nil {
		stepY = itemSize.Height + f.list.Theme().Size(theme.SizeNamePadding)
	}

	rows := rowsPerPage(f.activeViewportHeight(), stepY)
	return maxInt(1, rows-1)
}

func (f *fileList) gridPageDelta() int {
	cols := 1
	if f.grid != nil {
		cols = f.grid.ColumnCount()
	}
	if cols < 1 {
		itemSize := f.itemSizeWithZoom(GridView, f.getZoom())
		pad := theme.Padding()
		if f.grid != nil {
			pad = f.grid.Theme().Size(theme.SizeNamePadding)
		}
		cols = gridColumnCount(f.gridViewportWidthForLayout(), itemSize.Width, pad)
	}
	if cols < 1 {
		cols = 1
	}

	itemSize := f.itemSizeWithZoom(GridView, f.getZoom())
	stepY := itemSize.Height + theme.Padding()
	if f.grid != nil {
		stepY = itemSize.Height + f.grid.Theme().Size(theme.SizeNamePadding)
	}

	rows := rowsPerPage(f.activeViewportHeight(), stepY)
	return maxInt(1, rows-1) * cols
}

func (f *fileList) activeViewportHeight() float32 {
	if f.view == GridView && f.grid != nil && f.grid.Size().Height > 0 {
		return f.grid.Size().Height
	}
	if f.view != GridView && f.list != nil && f.list.Size().Height > 0 {
		return f.list.Size().Height
	}
	if f.content != nil && f.content.Size().Height > 0 {
		return max32(0, f.content.Size().Height-theme.Padding()*2)
	}
	return 0
}

func rowsPerPage(viewportHeight, stepY float32) int {
	if viewportHeight <= 0 || stepY <= 0 {
		return 1
	}
	rows := int(viewportHeight / stepY)
	if rows < 1 {
		return 1
	}
	return rows
}

// clearWidgetSelection drops any selection the GridWrap/List is tracking
// internally. The picker is the single source of truth for selection, so we
// must not leave a stale widget highlight behind when the contents change
// (e.g. when navigating into a different folder).
func (f *fileList) clearWidgetSelection() {
	if f.grid != nil {
		f.grid.UnselectAll()
	}
	if f.list != nil {
		f.list.UnselectAll()
	}
}

// confirmFromKeyboard handles Return/Enter pressed while the focused list/grid
// holds keyboard focus, emulating the dialog's confirm (OK) button.
func (f *fileList) confirmFromKeyboard() {
	if fd, ok := f.picker.(*fileDialog); ok {
		fd.confirmFromKeyboard()
	}
}

// cancelSearchFromKeyboard handles Escape pressed while the list/grid holds
// focus, clearing any in-progress type-ahead search the user started before
// clicking into the file area. See fileDialog.cancelSearchOnEscape.
func (f *fileList) cancelSearchFromKeyboard() {
	if fd, ok := f.picker.(*fileDialog); ok {
		fd.cancelSearchOnEscape()
	}
}

// focusActiveView gives keyboard focus to whichever of the list/grid is
// currently showing, leaving its highlight cursor untouched so navigation
// resumes from wherever it last was.
func (f *fileList) focusActiveView(c fyne.Canvas) {
	if c == nil {
		return
	}

	var target fyne.Focusable
	if f.view == GridView && f.grid != nil {
		target = f.grid
	} else if f.list != nil {
		target = f.list
	}

	if target != nil {
		c.Focus(target)
	}
}

// focusForKeyboardNav makes the active list/grid the keyboard navigation target
// and moves its highlight cursor to id. It is called when a file is chosen with
// the pointer so that the keyboard takes over from the clicked item without an
// explicit Tab, and without leaving a stray highlight on a different row.
func (f *fileList) focusForKeyboardNav(c fyne.Canvas, id int) {
	if c == nil {
		return
	}

	var target fyne.Focusable
	if f.view == GridView && f.grid != nil {
		f.grid.SetHighlight(id)
		target = f.grid
	} else if f.list != nil {
		f.list.SetHighlight(id)
		target = f.list
	}

	if target != nil {
		if id >= 0 && id < len(f.filtered) {
			f.keyboardFocus = id
		}
		c.Focus(target)
	}
}

// focusForKeyboardNavPreserveScroll is used after marquee selection. Setting
// the keyboard highlight scrolls the active widget to the highlighted item, but
// releasing a marquee while auto-scroll is active should leave the viewport
// exactly where auto-scroll ended.
func (f *fileList) focusForKeyboardNavPreserveScroll(c fyne.Canvas, id int) {
	offset := f.currentScrollOffset()
	f.focusForKeyboardNav(c, id)
	f.restoreScrollOffset(offset)
}

func (f *fileList) restoreScrollOffset(offset float32) {
	if f.view == GridView && f.grid != nil {
		f.grid.ScrollToOffset(offset)
		return
	}
	if f.list != nil {
		f.list.ScrollToOffset(offset)
	}
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
	f.keyboardFocus = 0
	// New contents: ensure no widget-owned selection highlight survives from the
	// previous folder. The picker's selection is reset separately by refreshDir.
	f.clearWidgetSelection()
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
	f.keyboardFocus = clampIndex(f.keyboardFocus, len(f.filtered))
	f.refresh()
}

func (f *fileList) setSortOrder(order FileSortOrder) {
	f.sortOrder = order
	f.sort()
	f.refresh()
}

func (f *fileList) sort() {
	includeModTime := f.activeFilter == "" && (f.sortOrder == SortDateAsc || f.sortOrder == SortDateDesc)
	sortKeys := make(map[string]fileSortKey, len(f.filtered))
	for _, file := range f.filtered {
		sortKeys[file.String()] = buildFileSortKey(file, includeModTime)
	}

	sort.Slice(f.filtered, func(i, j int) bool {
		u1, u2 := f.filtered[i], f.filtered[j]
		key1 := sortKeys[u1.String()]
		key2 := sortKeys[u2.String()]
		if key1.isDir != key2.isDir {
			return key1.isDir
		}

		// Smart Sort when filtering
		if f.activeFilter != "" {
			prefix1 := strings.HasPrefix(key1.name, f.activeFilter)
			prefix2 := strings.HasPrefix(key2.name, f.activeFilter)
			if prefix1 != prefix2 {
				// True comes first (Starts with filter)
				return prefix1
			}
			// Fallback to name sort
			return key1.name < key2.name
		}

		switch f.sortOrder {
		case SortNameDesc:
			return key1.name > key2.name
		case SortSizeAsc:
			// Just fallback to name for simplicity or implement size if needed
			// Ideally we use size from Lister if available
			return key1.name < key2.name
		case SortDateAsc:
			if before, ok := compareModTime(key1, key2, false); ok {
				return before
			}
			return key1.name < key2.name
		case SortDateDesc:
			if before, ok := compareModTime(key1, key2, true); ok {
				return before
			}
			return key1.name < key2.name
		default:
			return key1.name < key2.name
		}
	})
}

func buildFileSortKey(u fyne.URI, includeModTime bool) fileSortKey {
	key := fileSortKey{name: strings.ToLower(u.Name())}
	key.isDir, _ = storage.CanList(u)

	if includeModTime {
		key.modTime, key.hasModTime = fileModifiedTime(u)
	}

	return key
}

func compareModTime(a, b fileSortKey, newestFirst bool) (bool, bool) {
	if a.hasModTime != b.hasModTime {
		return a.hasModTime, true
	}
	if !a.hasModTime || a.modTime.Equal(b.modTime) {
		return false, false
	}
	if newestFirst {
		return a.modTime.After(b.modTime), true
	}
	return a.modTime.Before(b.modTime), true
}

func fileModifiedTime(u fyne.URI) (time.Time, bool) {
	if u == nil || u.Scheme() != "file" {
		return time.Time{}, false
	}

	info, err := os.Stat(filepath.FromSlash(u.Path()))
	if err != nil {
		return time.Time{}, false
	}
	return info.ModTime(), true
}

func (f *fileList) refresh() {
	var target fyne.CanvasObject
	if f.view == GridView {
		target = f.grid
	} else {
		target = f.list
	}

	inner := target
	if f.picker.IsMultiSelect() {
		f.overlay.content = target
		inner = f.overlay
	}

	if f.content.Content == nil || !isPadded(f.content.Content, inner) {
		f.content.Content = container.NewPadded(inner)
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
	gridTruncWidth  float32
	gridTextSize    float32
	gridLabelQueued bool

	currentPath  string
	currentView  ViewLayout
	currentZoom  float32
	currentIsDir bool
	lastClick    time.Time
	loadTimer    *time.Timer
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
	isDir := false
	if u != nil {
		isDir, _ = storage.CanList(u)
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
	i.currentIsDir = isDir

	if view == GridView {
		i.label.Alignment = fyne.TextAlignCenter
		// We manually wrap with '\n' so we can keep file extensions intact.
		i.label.Wrapping = fyne.TextWrapOff
		i.label.Truncation = fyne.TextTruncateClip

		// Keep formatting stable while GridWrap stretches cells during resize.
		truncWidth := i.gridBaseWidthForZoom(zoom)
		name = i.formatGridName(name, truncWidth, i.label.TextStyle)
		i.gridTruncWidth = truncWidth
		i.gridTextSize = theme.TextSize()
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
	if isDir {
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

		// Try instant memory hit.
		if img := GetThumbnailManager().LoadMemoryOnly(path); img != nil {
			i.showThumbnail(img)
			return
		}

		i.loadTimer = time.AfterFunc(200*time.Millisecond, func() {
			GetThumbnailManager().Load(u, func(img *canvas.Image) {
				// Ensure thread safety for UI updates using fyne.Do (available since v2.6.0)
				fyne.Do(func() {
					if i.currentPath != path {
						return
					}
					i.showThumbnail(img)
				})
			})
		})

		go GetThumbnailManager().LoadCached(u, func(img *canvas.Image) {
			fyne.Do(func() {
				if i.currentPath != path {
					return
				}
				if i.loadTimer != nil {
					i.loadTimer.Stop()
				}
				i.showThumbnail(img)
			})
		})
	}
}

func (i *fileItem) showThumbnail(img *canvas.Image) {
	if img == nil {
		return
	}

	i.thumbnail.File = ""
	i.thumbnail.Resource = nil
	i.thumbnail.FillMode = canvas.ImageFillContain
	i.thumbnail.Image = img.Image
	i.thumbnail.Refresh()
	i.icon.Hide()
	i.thumbnail.Show()
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

	// Hand keyboard focus to the file list so the user can keep navigating with
	// the arrow keys (and confirm with Return) without first pressing Tab.
	if fd, ok := i.picker.(*fileDialog); ok {
		fd.focusFileList(i.id)
	}
}

func (i *fileItem) showContextMenu(pos fyne.Position) {
	label := lang.L("Select")
	if i.picker.IsSelected(i.uri) {
		label = lang.L("Deselect")
	}

	toggleItem := fyne.NewMenuItem(label, func() {
		i.picker.ToggleSelection(i.id)
		i.picker.DismissMenu()
	})

	copyPathItem := fyne.NewMenuItem(lang.L("Copy Path"), func() {
		i.picker.CopyPath(i.uri)
		i.picker.DismissMenu()
	})

	menu := fyne.NewMenu("", toggleItem, copyPathItem)
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
	// Keep a slightly larger buffer to avoid edge clipping during gradual resize.
	width = max32(width-theme.Padding()*5, 0)

	textSize := theme.TextSize()
	measure := func(s string) float32 {
		size, _ := fyne.CurrentApp().Driver().RenderedTextSize(s, textSize, style, nil)
		return size.Width
	}
	return formatGridFileNameWithMeasure(name, width, measure)
}

func formatGridFolderName(name string, width float32, style fyne.TextStyle) string {
	if name == "" || width <= 0 {
		return name
	}

	// Safety margin to avoid clipping due to rounding differences between
	// RenderedTextSize measurements and actual rendering.
	// Keep a slightly larger buffer to avoid edge clipping during gradual resize.
	width = max32(width-theme.Padding()*5, 0)

	textSize := theme.TextSize()
	measure := func(s string) float32 {
		size, _ := fyne.CurrentApp().Driver().RenderedTextSize(s, textSize, style, nil)
		return size.Width
	}
	return formatGridFolderNameWithMeasure(name, width, measure)
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
	// from right-to-left while preserving a short basename tail:
	// "prefix...tail.ext".
	const dots = "..."
	baseRunes := []rune(base)
	const tailKeepRunes = 5
	for tailKeep := minInt(tailKeepRunes, len(baseRunes)); tailKeep >= 0; tailKeep-- {
		tail := string(baseRunes[len(baseRunes)-tailKeep:])
		for prefixKeep := len(baseRunes) - tailKeep; prefixKeep >= 0; prefixKeep-- {
			prefix := string(baseRunes[:prefixKeep])
			candidate := prefix + dots + tail + ext
			if lines, ok := wrapTextToLinesStrict(candidate, width, maxLines, measure); ok {
				return strings.Join(lines, "\n")
			}
		}
	}

	// Extremely narrow columns: show as much of the truncation suffix as possible.
	truncSuffix := dots + extText
	if lines, ok := wrapTextToLinesStrict(truncSuffix, width, maxLines, measure); ok {
		return strings.Join(lines, "\n")
	}

	return wrapTextToLines(truncSuffix, width, maxLines, measure)
}

func formatGridFolderNameWithMeasure(name string, width float32, measure func(string) float32) string {
	if name == "" || width <= 0 {
		return name
	}

	const maxLines = 3

	if measure(name) <= width {
		return name
	}

	// Keep full folder names when they naturally fit in 3 lines.
	if lines, ok := wrapTextToLinesStrict(name, width, maxLines, measure); ok {
		return strings.Join(lines, "\n")
	}

	// Folder names have no extension semantics; preserve a short tail and
	// progressively trim the prefix from right to left.
	const dots = "..."
	runes := []rune(name)
	tailKeep := minInt(5, len(runes))
	tail := string(runes[len(runes)-tailKeep:])

	for prefixKeep := len(runes) - tailKeep; prefixKeep >= 0; prefixKeep-- {
		prefix := string(runes[:prefixKeep])
		candidate := prefix + dots + tail
		if lines, ok := wrapTextToLinesStrict(candidate, width, maxLines, measure); ok {
			return strings.Join(lines, "\n")
		}
	}

	truncSuffix := dots + tail
	if lines, ok := wrapTextToLinesStrict(truncSuffix, width, maxLines, measure); ok {
		return strings.Join(lines, "\n")
	}

	return wrapTextToLines(truncSuffix, width, maxLines, measure)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
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

func stableGridLabelWidth(baseWidth, actualWidth float32) float32 {
	if baseWidth <= 0 {
		return actualWidth
	}
	// Keep truncation stable at the minimum stretched width. If a cell is ever
	// narrower than base (very tight layouts), fall back to actual width.
	if actualWidth > 0 && actualWidth < baseWidth {
		return actualWidth
	}
	return baseWidth
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

	targetWidth := i.gridTruncationWidth(width)

	// Avoid churn during continuous resize.
	if abs32(targetWidth-i.gridTruncWidth) < 1.0 && i.gridTextSize == theme.TextSize() {
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
		targetWidth := i.gridTruncationWidth(curWidth)
		if abs32(targetWidth-i.gridTruncWidth) < 1.0 && i.gridTextSize == curTextSize {
			return
		}

		newText := i.formatGridName(i.rawName, targetWidth, i.label.TextStyle)
		i.gridTruncWidth = targetWidth
		i.gridTextSize = curTextSize
		if i.label.Text != newText {
			i.label.SetText(newText)
		}
	})
}

func (i *fileItem) formatGridName(name string, width float32, style fyne.TextStyle) string {
	if i != nil && i.currentIsDir {
		return formatGridFolderName(name, width, style)
	}
	return formatGridFileName(name, width, style)
}

func (i *fileItem) gridBaseWidthForZoom(zoom float32) float32 {
	baseWidth := float32(fileIconCellWidth) * zoom
	if i.itemSz != nil {
		if s := i.itemSz(GridView, zoom); s.Width > 0 {
			baseWidth = s.Width
		}
	}
	return baseWidth
}

func (i *fileItem) gridBaseWidth() float32 {
	return i.gridBaseWidthForZoom(i.zoomScale())
}

func (i *fileItem) gridTruncationWidth(actualWidth float32) float32 {
	return stableGridLabelWidth(i.gridBaseWidth(), actualWidth)
}

func (r *fileItemRenderer) MinSize() fyne.Size {
	view := r.item.picker.GetView()
	zoom := r.item.zoomScale()
	// Return stable base size. refyne's GridWrap.StretchItems handles stretching
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
	// Return stable base size. refyne's GridWrap.StretchItems handles stretching at layout time.
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

// gridStretchedCellWidth returns the on-screen width of a grid cell, accounting
// for GridWrap.StretchItems. When stretching is enabled the grid widens each cell
// so the columns fill the viewport evenly with no dead space on the right. This
// mirrors the formula in refyne's gridWrapLayout.updateGrid so marquee
// hit-testing uses the same cell stride the grid actually rendered; otherwise
// horizontal selection drifts once the window is resized wide enough to stretch.
func gridStretchedCellWidth(baseWidth, viewportWidth, padding float32, cols int) float32 {
	if cols < 1 || viewportWidth <= 0 {
		return baseWidth
	}
	stretched := (viewportWidth - float32(cols-1)*padding) / float32(cols)
	if stretched > baseWidth {
		return stretched
	}
	return baseWidth
}

func (f *fileList) onSelectionDrag(start, cur fyne.Position) {
	if !f.picker.IsMultiSelect() {
		return
	}

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
	if !f.picker.IsMultiSelect() || !f.dragSelecting || len(f.filtered) == 0 {
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

		// GridWrap.StretchItems widens cells to fill the viewport once the window
		// is wide enough, so the rendered cell stride is larger than the base item
		// width. Hit-test against the same stretched width the grid drew with, or
		// the accumulated per-column error offsets horizontal selection.
		cellWidth := itemSize.Width
		if f.grid.StretchItems {
			cellWidth = gridStretchedCellWidth(itemSize.Width, f.grid.Size().Width, pad, cols)
		}

		stepX := cellWidth + pad
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
				x2 := x1 + cellWidth
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

	// Hand keyboard focus to the list once the marquee is released, so the
	// keyboard takes over without an explicit Tab (mirrors a pointer click; see
	// fileItem.MouseUp). Park the cursor on the last item the marquee covered,
	// which matches the selection anchor SelectMultiple sets. ids are appended in
	// ascending order, so the final entry is the anchor.
	focusID := -1
	if n := len(f.lastDragSelection); n > 0 {
		focusID = f.lastDragSelection[n-1]
	}

	f.lastDragSelection = nil
	f.dragSelecting = false
	f.lastDragTime = time.Now()
	f.overlay.setDebugRects(nil)

	// Only take focus when the marquee actually covered something; an empty drag
	// over blank space shouldn't steal focus.
	if focusID >= 0 {
		if fd, ok := f.picker.(*fileDialog); ok {
			fd.focusFileListPreserveScroll(focusID)
		}
	}
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
	f.autoScrollVelocity = autoScrollVelocity(intensity * maxStep)
	f.startAutoScroll()
}

func (f *fileList) startAutoScroll() {
	if f.autoScrollAnim != nil {
		return
	}
	f.autoScrollLastTick = time.Now()
	f.autoScrollAnim = fyne.NewAnimation(time.Second, func(float32) {
		f.autoScrollTick()
	})
	f.autoScrollAnim.Curve = fyne.AnimationLinear
	f.autoScrollAnim.RepeatCount = fyne.AnimationRepeatForever
	f.autoScrollAnim.Start()
}

func (f *fileList) stopAutoScroll() {
	if f.autoScrollAnim != nil {
		f.autoScrollAnim.Stop()
		f.autoScrollAnim = nil
	}
	f.autoScrollDir = 0
	f.autoScrollVelocity = 0
	f.autoScrollLastTick = time.Time{}
}

func (f *fileList) autoScrollTick() {
	if !f.dragSelecting || f.autoScrollDir == 0 || f.autoScrollVelocity <= 0 {
		f.stopAutoScroll()
		return
	}

	offset := f.currentScrollOffset()
	maxOffset := f.maxScrollOffset()
	if maxOffset <= 0 {
		f.stopAutoScroll()
		return
	}

	now := time.Now()
	elapsed := now.Sub(f.autoScrollLastTick)
	f.autoScrollLastTick = now
	distance := autoScrollDistance(f.autoScrollVelocity, elapsed)
	if distance <= 0 {
		return
	}

	next := offset + float32(f.autoScrollDir)*distance
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

func autoScrollVelocity(stepPerBaseFrame float32) float32 {
	return stepPerBaseFrame / float32(autoScrollBaseFrame.Seconds())
}

func autoScrollDistance(velocity float32, elapsed time.Duration) float32 {
	if elapsed <= 0 || velocity <= 0 {
		return 0
	}
	if elapsed > autoScrollMaxFrame {
		elapsed = autoScrollMaxFrame
	}
	return velocity * float32(elapsed.Seconds())
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
