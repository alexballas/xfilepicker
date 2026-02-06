package dialog

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// ShowFileOpen creates and shows a file dialog allowing the user to choose
// one or more files to open.
func ShowFileOpen(callback func(readers []fyne.URIReadCloser, err error), parent fyne.Window, allowMultiple bool) {
	d := NewFileOpen(callback, parent, allowMultiple)
	d.Show()
}

// NewFileOpen creates a file dialog allowing the user to choose one or more files to open.
func NewFileOpen(callback func(readers []fyne.URIReadCloser, err error), parent fyne.Window, allowMultiple bool) dialog.Dialog {
	d := &fileDialog{
		callback:      callback,
		parent:        parent,
		allowMultiple: allowMultiple,
		selected:      make(map[string]fyne.URI),
		dir:           effectiveStartingDir(),
		view:          defaultView, // Will be loaded from prefs
		zoomLevel:     defaultZoomLevelIndex,
		anchor:        -1,
	}
	d.loadPrefs()
	return d
}

type fileDialog struct {
	callback func([]fyne.URIReadCloser, error)
	parent   fyne.Window
	dir      fyne.ListableURI

	selected map[string]fyne.URI

	// Components
	sidebar    *sidebar
	fileList   *fileList
	breadcrumb *breadcrumb

	// UI
	win      *widget.PopUp
	fileName *widget.Label
	open     *widget.Button
	dismiss  *widget.Button

	view       ViewLayout
	showHidden bool
	zoomLevel  int

	allowMultiple bool
	anchor        int // Selection anchor for Shift-Select

	extensionFilter storage.FileFilter

	// Search & Sort
	searchEntry *widget.Entry

	originalOnTypedRune func(rune)
	originalOnTypedKey  func(*fyne.KeyEvent)
	activeMenu          *widget.PopUp

	zoomInBtn  *widget.Button
	zoomOutBtn *widget.Button
}

func (f *fileDialog) Show() {
	if fileOpenOSOverride(f) {
		return
	}

	content := f.makeUI()
	f.win = widget.NewModalPopUp(content, f.parent.Canvas())
	f.win.Resize(fyne.NewSize(1000, 700))

	f.win.Show()

	// Intercept keys for Type-to-Search
	// NOTE: We register hooks AFTER Show() to capture any hooks that ModalPopUp might set (e.g. for closing on Escape)
	f.originalOnTypedRune = f.parent.Canvas().OnTypedRune()
	f.parent.Canvas().SetOnTypedRune(f.typedRuneHook)
	f.originalOnTypedKey = f.parent.Canvas().OnTypedKey()
	f.parent.Canvas().SetOnTypedKey(f.typedKeyHook)
	f.refreshDir(f.dir)
}

func (f *fileDialog) Hide() {
	// Restore original handler
	if f.parent != nil && f.parent.Canvas() != nil {
		f.parent.Canvas().SetOnTypedRune(f.originalOnTypedRune)
		f.parent.Canvas().SetOnTypedKey(f.originalOnTypedKey)
	}

	if f.win != nil {
		f.win.Hide()
	}
}

func (f *fileDialog) Dismiss() {
	f.Hide()
}

func (f *fileDialog) MinSize() fyne.Size {
	return f.makeUI().MinSize()
}

func (f *fileDialog) SetOnClosed(closed func()) {
}

func (f *fileDialog) SetDismissText(text string) {
}

func (f *fileDialog) Refresh() {
}

func (f *fileDialog) Resize(size fyne.Size) {
	if f.win != nil {
		f.win.Resize(size)
	}
	f.DismissMenu()
}

func (f *fileDialog) Position() fyne.Position {
	return fyne.NewPos(0, 0)
}

// FilePicker Interface Implementation

func (f *fileDialog) SetLocation(dir fyne.ListableURI) {
	f.DismissMenu()
	if f.searchEntry != nil {
		f.searchEntry.SetText("")
	}
	if f.sidebar != nil {
		f.sidebar.SyncSelection(dir)
	}
	f.refreshDir(dir)
}

func (f *fileDialog) SetView(view ViewLayout) {
	f.DismissMenu()
	f.view = view
	fyne.CurrentApp().Preferences().SetInt(viewLayoutKey, int(view))
	f.fileList.setView(view)
}

func (f *fileDialog) GetView() ViewLayout {
	return f.view
}

func (f *fileDialog) zoomScale() float32 {
	f.zoomLevel = clampZoomLevelIndex(f.zoomLevel)
	return zoomLevels[f.zoomLevel]
}

func (f *fileDialog) adjustZoom(steps int) {
	if steps == 0 {
		return
	}
	f.setZoomLevel(f.zoomLevel + steps)
}

func (f *fileDialog) setZoomLevel(level int) {
	level = clampZoomLevelIndex(level)
	if f.zoomLevel == level {
		return
	}

	f.zoomLevel = level
	fyne.CurrentApp().Preferences().SetInt(zoomLevelKey, f.zoomLevel)

	if f.fileList != nil {
		f.fileList.setZoom(f.zoomScale())
	}
	f.updateZoomButtons()
}

func (f *fileDialog) updateZoomButtons() {
	if f.zoomOutBtn != nil {
		if f.zoomLevel <= 0 {
			f.zoomOutBtn.Disable()
		} else {
			f.zoomOutBtn.Enable()
		}
	}
	if f.zoomInBtn != nil {
		if f.zoomLevel >= len(zoomLevels)-1 {
			f.zoomInBtn.Disable()
		} else {
			f.zoomInBtn.Enable()
		}
	}
}

func (f *fileDialog) IsMultiSelect() bool {
	return f.allowMultiple
}

func (f *fileDialog) ShowMenu(menu *fyne.Menu, pos fyne.Position, obj fyne.CanvasObject) {
	f.DismissMenu()

	canvas := f.parent.Canvas()
	if f.win != nil {
		canvas = f.win.Canvas
	}

	m := widget.NewMenu(menu)
	m.OnDismiss = f.DismissMenu

	// Manually calculate absolute position since PopUp doesn't have ShowAtRelativePosition
	absPos := fyne.CurrentApp().Driver().AbsolutePositionForObject(obj).Add(pos)

	f.activeMenu = widget.NewPopUp(m, canvas)
	f.activeMenu.ShowAtPosition(absPos)
}

func (f *fileDialog) DismissMenu() {
	if f.activeMenu != nil {
		f.activeMenu.Hide()
		f.activeMenu = nil
	}
}

func (f *fileDialog) Select(id int) {
	if id < 0 || id >= len(f.fileList.filtered) {
		return
	}
	uri := f.fileList.filtered[id]
	f.selected = make(map[string]fyne.URI)
	f.selected[uri.String()] = uri
	f.anchor = id
	f.updateFooter()
	f.fileList.refresh()
}

func (f *fileDialog) SelectMultiple(ids []int) {
	if !f.allowMultiple {
		return
	}
	f.selected = make(map[string]fyne.URI)
	for _, id := range ids {
		if id < 0 || id >= len(f.fileList.filtered) {
			continue
		}
		uri := f.fileList.filtered[id]
		f.selected[uri.String()] = uri
	}
	if len(ids) > 0 {
		f.anchor = ids[len(ids)-1]
	}
	f.updateFooter()
	f.fileList.refresh()
}

func (f *fileDialog) ToggleSelection(id int) {
	if !f.allowMultiple {
		f.Select(id)
		return
	}
	if id < 0 || id >= len(f.fileList.filtered) {
		return
	}
	uri := f.fileList.filtered[id]
	if f.IsSelected(uri) {
		delete(f.selected, uri.String())
	} else {
		f.selected[uri.String()] = uri
	}
	f.anchor = id
	f.updateFooter()
	f.fileList.refresh()
}

func (f *fileDialog) ExtendSelection(id int) {
	if !f.allowMultiple {
		f.Select(id)
		return
	}
	if id < 0 || id >= len(f.fileList.filtered) {
		return
	}

	if f.anchor == -1 {
		f.anchor = 0
	}

	start, end := f.anchor, id
	if start > end {
		start, end = end, start
	}

	f.selected = make(map[string]fyne.URI)
	for i := start; i <= end; i++ {
		u := f.fileList.filtered[i]
		f.selected[u.String()] = u
	}

	f.updateFooter()
	f.fileList.refresh()
}

func (f *fileDialog) IsSelected(uri fyne.URI) bool {
	_, ok := f.selected[uri.String()]
	return ok
}

func (f *fileDialog) OpenSelection() {
	f.open.OnTapped()
}

func (f *fileDialog) CopyPath(uri fyne.URI) {
	if uri == nil {
		return
	}

	path := uri.Path()
	if path == "" {
		path = uri.String()
	}
	if path == "" {
		return
	}

	if app := fyne.CurrentApp(); app != nil {
		app.Clipboard().SetContent(path)
	}
}

func (f *fileDialog) SetFilter(filter storage.FileFilter) {
	f.extensionFilter = filter
	if f.win != nil {
		f.refreshDir(f.dir)
	}
}

func (f *fileDialog) typedRuneHook(r rune) {
	if f.originalOnTypedRune != nil {
		f.originalOnTypedRune(r)
	}

	if f.win == nil {
		return
	}

	focused := f.parent.Canvas().Focused()

	// If search entry is already focused, let standard handler work.
	if focused == f.searchEntry {
		return
	}

	// Safe to type-to-search ONLY if focus is:
	// 1. Nil (nothing focused)
	// 2. Navigation components (Sidebar, File List)
	//
	// If anything else is focused (e.g. New Folder Entry, Rename Entry), we MUST NOT interfere.

	allowed := focused == nil

	if !allowed && f.sidebar != nil && focused == f.sidebar.list {
		allowed = true
	}
	if !allowed && f.fileList != nil {
		if focused == f.fileList.list || focused == f.fileList.grid {
			allowed = true
		}
	}

	if !allowed {
		return
	}

	// Focus search and append the character
	f.parent.Canvas().Focus(f.searchEntry)
	f.searchEntry.SetText(f.searchEntry.Text + string(r))
	f.searchEntry.CursorColumn = len(f.searchEntry.Text)
	f.searchEntry.Refresh()
}

func (f *fileDialog) typedKeyHook(ev *fyne.KeyEvent) {
	if f.originalOnTypedKey != nil {
		f.originalOnTypedKey(ev)
	}
	if f.win == nil || ev == nil {
		return
	}

	if ev.Name != fyne.KeyReturn && ev.Name != fyne.KeyEnter {
		return
	}

	// Only trigger Open when focus is on the file list (or nothing focused).
	// We must not interfere with dialogs/forms (e.g. New Folder) or text inputs.
	focused := f.parent.Canvas().Focused()
	allowed := focused == nil
	if !allowed && f.fileList != nil {
		if focused == f.fileList.list || focused == f.fileList.grid {
			allowed = true
		}
	}
	if !allowed {
		return
	}

	if f.open == nil || f.open.Disabled() || len(f.selected) == 0 {
		return
	}
	f.open.OnTapped()
}

// Internal Logic

func (f *fileDialog) makeUI() fyne.CanvasObject {
	// Init sub-components
	f.sidebar = newSidebar(f)
	f.fileList = newFileList(f)
	f.breadcrumb = newBreadcrumb(f)

	f.fileList.setView(f.view)
	f.fileList.setZoom(f.zoomScale())

	// Footer
	f.fileName = widget.NewLabel("")
	f.fileName.Truncation = fyne.TextTruncateEllipsis

	f.open = widget.NewButton(lang.L("Open"), func() {
		if len(f.selected) == 1 {
			var u fyne.URI
			for _, val := range f.selected {
				u = val
			}
			if isDir, _ := storage.CanList(u); isDir {
				if l, err := storage.ListerForURI(u); err == nil {
					f.SetLocation(l)
					return
				}
			}
		}

		var readers []fyne.URIReadCloser
		for _, u := range f.selected {
			r, err := storage.Reader(u)
			if err == nil {
				readers = append(readers, r)
			}
		}
		f.Hide()
		f.callback(readers, nil)
	})
	f.open.Importance = widget.HighImportance
	f.open.Disable()

	f.dismiss = widget.NewButton(lang.L("Cancel"), func() {
		f.Hide()
		f.callback(nil, nil)
	})

	footer := container.NewBorder(nil, nil, nil, container.NewHBox(f.dismiss, f.open), container.NewHScroll(f.fileName))

	// Header / TopBar
	f.searchEntry = widget.NewEntry()
	f.searchEntry.SetPlaceHolder(lang.L("Search..."))
	f.searchEntry.OnChanged = func(s string) {
		f.DismissMenu()
		f.fileList.setFilter(s)
	}

	viewToggle := widget.NewButtonWithIcon("", theme.GridIcon(), nil)
	updateViewToggleIcon := func() {
		if f.view == GridView {
			viewToggle.SetIcon(theme.ListIcon())
		} else {
			viewToggle.SetIcon(theme.GridIcon())
		}
	}
	viewToggle.OnTapped = func() {
		if f.view == GridView {
			f.SetView(ListView)
		} else {
			f.SetView(GridView)
		}
		updateViewToggleIcon()
	}
	updateViewToggleIcon()

	newFolderBtn := widget.NewButtonWithIcon("", theme.FolderNewIcon(), func() {
		newFolderEntry := widget.NewEntry()
		d := dialog.NewForm(lang.L("New Folder"), lang.L("Create Folder"), lang.L("Cancel"), []*widget.FormItem{
			{Text: lang.X("file.name", "Name"), Widget: newFolderEntry},
		}, func(s bool) {
			if !s || newFolderEntry.Text == "" {
				return
			}
			newFolderPath := filepath.Join(f.dir.Path(), newFolderEntry.Text)
			if err := os.MkdirAll(newFolderPath, 0o750); err != nil {
				dialog.ShowError(err, f.parent)
			}
			f.refreshDir(f.dir)
		}, f.parent)
		d.Show()
		f.parent.Canvas().Focus(newFolderEntry)
	})

	optionsBtn := widget.NewButtonWithIcon("", theme.SettingsIcon(), nil)
	optionsBtn.OnTapped = func() {
		hiddenFiles := widget.NewCheck(lang.L("Show Hidden Files"), func(changed bool) {
			f.showHidden = changed
			fyne.CurrentApp().Preferences().SetBool(showHiddenKey, changed)
			f.refreshDir(f.dir)
		})
		hiddenFiles.Checked = f.showHidden

		content := container.NewVBox(
			hiddenFiles,
		)
		pop := widget.NewPopUp(content, f.win.Canvas)
		pop.ShowAtPosition(fyne.CurrentApp().Driver().AbsolutePositionForObject(optionsBtn).Add(fyne.NewPos(0, optionsBtn.Size().Height)))
	}

	sortSelect := widget.NewSelect([]string{
		lang.L("Name (A-Z)"),
		lang.L("Name (Z-A)"),
		lang.L("Size"),
		lang.L("Date"),
	}, func(s string) {
		var order FileSortOrder
		switch s {
		case lang.L("Name (Z-A)"):
			order = SortNameDesc
		case lang.L("Size"):
			order = SortSizeAsc // TODO: Descending?
		case lang.L("Date"):
			order = SortDateDesc
		default:
			order = SortNameAsc
		}
		f.fileList.setSortOrder(order)
	})
	sortSelect.PlaceHolder = lang.L("Sort By")
	sortSelect.SetSelected(lang.L("Name (A-Z)"))

	// Group controls into two rows.
	searchWrapper := container.NewGridWrap(fyne.NewSize(220, 36), f.searchEntry)
	f.zoomOutBtn = widget.NewButtonWithIcon("", theme.ZoomOutIcon(), func() {
		f.adjustZoom(-1)
	})
	f.zoomInBtn = widget.NewButtonWithIcon("", theme.ZoomInIcon(), func() {
		f.adjustZoom(1)
	})
	f.updateZoomButtons()

	controlsRow := container.NewHBox(searchWrapper, sortSelect, newFolderBtn, f.zoomOutBtn, f.zoomInBtn, viewToggle, optionsBtn)

	// Top Bar with Title and Controls
	titleText := lang.L("Open File")
	if f.allowMultiple {
		titleText = lang.L("Open Files")
	}
	titleLabel := widget.NewLabelWithStyle(titleText, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	topBarContent := container.NewBorder(nil, nil, titleLabel, controlsRow, nil)
	topBarScroll := container.NewHScroll(topBarContent)
	topBarScroll.Direction = container.ScrollHorizontalOnly

	// Global Header (Top Bar + Separator)
	// Usage of VBox ensures the separator is below the toolbar
	globalHeader := container.NewVBox(topBarScroll, widget.NewSeparator())

	// File List Header: just the breadcrumbs
	// We keep the Padded container for consistent spacing
	breadcrumbsArea := container.NewPadded(f.breadcrumb.scroll)

	zoomOverlay := newZoomScrollOverlay(func(steps int) {
		f.adjustZoom(steps)
	})

	split := container.NewHSplit(
		container.NewPadded(f.sidebar.list),
		container.NewBorder(breadcrumbsArea, nil, nil, nil, container.NewStack(f.fileList.content, zoomOverlay)),
	)
	split.SetOffset(0.25)

	// Wrap in a custom layout that detects resize
	return container.New(&resizeLayout{
		internal: layout.NewStackLayout(),
		onResize: func() {
			f.DismissMenu()
			if f.fileList != nil {
				f.fileList.onResize()
			}
		},
		externalSize: func() fyne.Size {
			if f.parent == nil || f.parent.Canvas() == nil {
				return fyne.Size{}
			}
			return f.parent.Canvas().Size()
		},
	}, container.NewBorder(globalHeader, footer, nil, nil, split))
}

type resizeLayout struct {
	internal fyne.Layout
	onResize func()

	externalSize     func() fyne.Size
	lastSize         fyne.Size
	lastExternalSize fyne.Size
	lastFired        time.Time
	timer            *time.Timer
}

func (r *resizeLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	r.internal.Layout(objects, size)
	if r.onResize != nil {
		internalChanged := abs32(size.Width-r.lastSize.Width) >= 0.5 || abs32(size.Height-r.lastSize.Height) >= 0.5
		if internalChanged {
			r.lastSize = size
		}

		externalChanged := false
		if r.externalSize != nil {
			external := r.externalSize()
			externalChanged = abs32(external.Width-r.lastExternalSize.Width) >= 0.5 || abs32(external.Height-r.lastExternalSize.Height) >= 0.5
			if externalChanged {
				r.lastExternalSize = external
			}
		}

		// Only react to real size changes (layouts can run for other reasons).
		if !internalChanged && !externalChanged {
			return
		}

		r.scheduleResize()
	}
}

func (r *resizeLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return r.internal.MinSize(objects)
}

func (r *resizeLayout) scheduleResize() {
	// Defer the resize callback to avoid modifying the UI during layout
	// which can cause panics in Fyne driver. Coalesce bursts during window resize.
	const minInterval = 60 * time.Millisecond

	if r.onResize == nil {
		return
	}

	now := time.Now()
	elapsed := now.Sub(r.lastFired)
	if elapsed >= minInterval {
		r.lastFired = now
		fyne.Do(r.onResize)
		return
	}

	delay := minInterval - elapsed
	if delay < 0 {
		delay = 0
	}

	if r.timer == nil {
		r.timer = time.AfterFunc(delay, func() {
			fyne.Do(func() {
				r.timer = nil
				r.lastFired = time.Now()
				if r.onResize != nil {
					r.onResize()
				}
			})
		})
		return
	}
	r.timer.Reset(delay)
}

func (f *fileDialog) refreshDir(dir fyne.ListableURI) {
	f.dir = dir

	if f.breadcrumb != nil {
		f.breadcrumb.update(dir)
	}

	files, err := dir.List()
	if err != nil {
		return
	}

	// Filter hidden & extensions
	var filteredFiles []fyne.URI
	for _, file := range files {
		if !f.showHidden && isHidden(file) {
			continue
		}

		if isDir, _ := storage.CanList(file); isDir {
			// Always show directories
			filteredFiles = append(filteredFiles, file)
			continue
		}

		if f.extensionFilter == nil || f.extensionFilter.Matches(file) {
			filteredFiles = append(filteredFiles, file)
		}
	}
	files = filteredFiles

	if f.fileList != nil {
		f.fileList.setFiles(files)
	}
	f.selected = make(map[string]fyne.URI)
	f.anchor = -1
	f.updateFooter()
}

func (f *fileDialog) updateFooter() {
	if f.fileName == nil {
		return
	}
	var names []string
	hasDir := false
	for _, u := range f.selected {
		names = append(names, u.Name())
		if isDir, _ := storage.CanList(u); isDir {
			hasDir = true
		}
	}
	f.fileName.SetText(strings.Join(names, ", "))

	// Logic: Only disable when multiselecting and folders are involved
	if len(f.selected) == 0 {
		f.open.Disable()
	} else if len(f.selected) > 1 && hasDir {
		f.open.Disable()
	} else {
		f.open.Enable()
	}
}

func (f *fileDialog) loadPrefs() {
	f.showHidden = fyne.CurrentApp().Preferences().Bool(showHiddenKey)

	view := ViewLayout(fyne.CurrentApp().Preferences().Int(viewLayoutKey))
	if view != GridView && view != ListView {
		view = GridView
	}
	f.view = view

	f.zoomLevel = clampZoomLevelIndex(fyne.CurrentApp().Preferences().Int(zoomLevelKey))
}

// Helpers

func isHidden(file fyne.URI) bool {
	if file.Scheme() != "file" {
		return false
	}
	name := filepath.Base(file.Path())
	return name == "" || name[0] == '.'
}

func effectiveStartingDir() fyne.ListableURI {
	// Try home dir
	dir, err := os.UserHomeDir()
	if err == nil {
		lister, err := storage.ListerForURI(storage.NewFileURI(dir))
		if err == nil {
			return lister
		}
	}
	// Root
	lister, _ := storage.ListerForURI(storage.NewFileURI("/"))
	return lister
}
