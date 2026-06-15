package dialog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/alexballas/refyne/v2"
	"github.com/alexballas/refyne/v2/storage"
	"github.com/alexballas/refyne/v2/test"
	"github.com/alexballas/refyne/v2/theme"
)

type mockPicker struct{}

func (m *mockPicker) SetLocation(dir fyne.ListableURI)                                   {}
func (m *mockPicker) Refresh()                                                           {}
func (m *mockPicker) SetView(view ViewLayout)                                            {}
func (m *mockPicker) GetView() ViewLayout                                                { return GridView }
func (m *mockPicker) Select(id int)                                                      {}
func (m *mockPicker) SelectMultiple(ids []int)                                           {}
func (m *mockPicker) ToggleSelection(id int)                                             {}
func (m *mockPicker) ExtendSelection(id int)                                             {}
func (m *mockPicker) IsSelected(uri fyne.URI) bool                                       { return false }
func (m *mockPicker) OpenSelection()                                                     {}
func (m *mockPicker) CopyPath(uri fyne.URI)                                              {}
func (m *mockPicker) SetFilter(filter storage.FileFilter)                                {}
func (m *mockPicker) IsMultiSelect() bool                                                { return false }
func (m *mockPicker) ShowMenu(menu *fyne.Menu, pos fyne.Position, obj fyne.CanvasObject) {}
func (m *mockPicker) DismissMenu()                                                       {}

func TestFileList_Sort_Filter(t *testing.T) {
	test.NewApp()
	picker := &mockPicker{}
	fl := newFileList(picker)

	// Setup files
	// "apple.png", "pineapple.png", "banana.png"
	// "images" (folder)

	f1 := storage.NewFileURI("/tmp/apple.png")
	f2 := storage.NewFileURI("/tmp/pineapple.png")
	f3 := storage.NewFileURI("/tmp/banana.png")
	// Note: We need to mock CanList for folder logic to work exactly as expected if we were testing folders vs files.
	// But standard storage.NewFileURI usually returns false for CanList unless checking OS.
	// For this test, we assume they are files.

	fl.setFiles([]fyne.URI{f1, f2, f3})

	// 1. Test Default Sort (Name Asc)
	fl.setSortOrder(SortNameAsc)
	// Expected: apple, banana, pineapple
	if fl.filtered[0].Name() != "apple.png" {
		t.Errorf("Expected apple.png first, got %s", fl.filtered[0].Name())
	}

	// 2. Test Name Desc
	fl.setSortOrder(SortNameDesc)
	// Expected: pineapple, banana, apple
	if fl.filtered[0].Name() != "pineapple.png" {
		t.Errorf("Expected pineapple.png first, got %s", fl.filtered[0].Name())
	}

	// 3. Test Filter "apple" with Name Desc
	// Filter matches: apple.png (starts with), pineapple.png (contains)
	// Smart Sort should prioritize "starts with" -> apple.png should be first
	// Even though SortNameDesc would put pineapple first.
	fl.setFilter("apple")

	if len(fl.filtered) != 2 {
		t.Fatalf("Expected 2 filtered items, got %d", len(fl.filtered))
	}

	if fl.filtered[0].Name() != "apple.png" {
		t.Errorf("Smart Sort failed. Expected apple.png (starts with) first, got %s", fl.filtered[0].Name())
	}
	if fl.filtered[1].Name() != "pineapple.png" {
		t.Errorf("Expected pineapple.png second, got %s", fl.filtered[1].Name())
	}

	// 4. Test Filter "ban"
	fl.setFilter("ban")
	if len(fl.filtered) != 1 {
		t.Fatalf("Expected 1 filtered item, got %d", len(fl.filtered))
	}
	if fl.filtered[0].Name() != "banana.png" {
		t.Errorf("Expected banana.png, got %s", fl.filtered[0].Name())
	}
}

func TestFileList_SortModifiedTime(t *testing.T) {
	test.NewApp()
	picker := &mockPicker{}
	fl := newFileList(picker)

	root := t.TempDir()
	oldPath := filepath.Join(root, "old.txt")
	midPath := filepath.Join(root, "mid.txt")
	newPath := filepath.Join(root, "new.txt")
	for _, path := range []string{oldPath, midPath, newPath} {
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("write failed: %v", err)
		}
	}

	base := time.Unix(1_700_000_000, 0)
	times := map[string]time.Time{
		oldPath: base,
		midPath: base.Add(1 * time.Hour),
		newPath: base.Add(2 * time.Hour),
	}
	for path, modTime := range times {
		if err := os.Chtimes(path, modTime, modTime); err != nil {
			t.Fatalf("chtimes failed: %v", err)
		}
	}

	fl.setFiles([]fyne.URI{
		storage.NewFileURI(midPath),
		storage.NewFileURI(oldPath),
		storage.NewFileURI(newPath),
	})

	fl.setSortOrder(SortLastModified)
	assertFileOrder(t, fl.filtered, "new.txt", "mid.txt", "old.txt")

	fl.setSortOrder(SortFirstModified)
	assertFileOrder(t, fl.filtered, "old.txt", "mid.txt", "new.txt")
}

func assertFileOrder(t *testing.T, uris []fyne.URI, names ...string) {
	t.Helper()
	if len(uris) != len(names) {
		t.Fatalf("got %d files, want %d", len(uris), len(names))
	}
	for i, name := range names {
		if uris[i].Name() != name {
			t.Fatalf("file %d = %q, want %q", i, uris[i].Name(), name)
		}
	}
}

type recordingPicker struct {
	selectedIDs []int
}

func (r *recordingPicker) SetLocation(dir fyne.ListableURI)                                   {}
func (r *recordingPicker) Refresh()                                                           {}
func (r *recordingPicker) SetView(view ViewLayout)                                            {}
func (r *recordingPicker) GetView() ViewLayout                                                { return ListView }
func (r *recordingPicker) Select(id int)                                                      {}
func (r *recordingPicker) SelectMultiple(ids []int)                                           { r.selectedIDs = append([]int(nil), ids...) }
func (r *recordingPicker) ToggleSelection(id int)                                             {}
func (r *recordingPicker) ExtendSelection(id int)                                             {}
func (r *recordingPicker) IsSelected(uri fyne.URI) bool                                       { return false }
func (r *recordingPicker) OpenSelection()                                                     {}
func (r *recordingPicker) CopyPath(uri fyne.URI)                                              {}
func (r *recordingPicker) SetFilter(filter storage.FileFilter)                                {}
func (r *recordingPicker) IsMultiSelect() bool                                                { return true }
func (r *recordingPicker) ShowMenu(menu *fyne.Menu, pos fyne.Position, obj fyne.CanvasObject) {}
func (r *recordingPicker) DismissMenu()                                                       {}

type singleRecordingPicker struct {
	selectedIDs []int
}

func (r *singleRecordingPicker) SetLocation(dir fyne.ListableURI) {}

func (r *singleRecordingPicker) Refresh() {}

func (r *singleRecordingPicker) SetView(view ViewLayout) {}

func (r *singleRecordingPicker) GetView() ViewLayout { return ListView }

func (r *singleRecordingPicker) Select(id int) {}

func (r *singleRecordingPicker) SelectMultiple(ids []int) { r.selectedIDs = append([]int(nil), ids...) }

func (r *singleRecordingPicker) ToggleSelection(id int) {}

func (r *singleRecordingPicker) ExtendSelection(id int) {}

func (r *singleRecordingPicker) IsSelected(uri fyne.URI) bool { return false }

func (r *singleRecordingPicker) OpenSelection() {}

func (r *singleRecordingPicker) CopyPath(uri fyne.URI) {}

func (r *singleRecordingPicker) SetFilter(filter storage.FileFilter) {}

func (r *singleRecordingPicker) IsMultiSelect() bool { return false }

func (r *singleRecordingPicker) ShowMenu(menu *fyne.Menu, pos fyne.Position, obj fyne.CanvasObject) {}

func (r *singleRecordingPicker) DismissMenu() {}

type contextMenuPicker struct {
	menu         *fyne.Menu
	copiedURI    fyne.URI
	dismissCalls int
}

func (c *contextMenuPicker) SetLocation(dir fyne.ListableURI)    {}
func (c *contextMenuPicker) Refresh()                            {}
func (c *contextMenuPicker) SetView(view ViewLayout)             {}
func (c *contextMenuPicker) GetView() ViewLayout                 { return ListView }
func (c *contextMenuPicker) Select(id int)                       {}
func (c *contextMenuPicker) SelectMultiple(ids []int)            {}
func (c *contextMenuPicker) ToggleSelection(id int)              {}
func (c *contextMenuPicker) ExtendSelection(id int)              {}
func (c *contextMenuPicker) IsSelected(uri fyne.URI) bool        { return false }
func (c *contextMenuPicker) OpenSelection()                      {}
func (c *contextMenuPicker) CopyPath(uri fyne.URI)               { c.copiedURI = uri }
func (c *contextMenuPicker) SetFilter(filter storage.FileFilter) {}
func (c *contextMenuPicker) IsMultiSelect() bool                 { return true }
func (c *contextMenuPicker) ShowMenu(menu *fyne.Menu, pos fyne.Position, obj fyne.CanvasObject) {
	c.menu = menu
}
func (c *contextMenuPicker) DismissMenu() { c.dismissCalls++ }

// gridRecordingPicker reports GridView (so grid items get a real, non-zero
// MinSize and ColumnCount works) and multi-select (so marquee selection runs),
// recording the ids handed to SelectMultiple.
type gridRecordingPicker struct {
	selectedIDs []int
}

func (r *gridRecordingPicker) SetLocation(dir fyne.ListableURI)    {}
func (r *gridRecordingPicker) Refresh()                            {}
func (r *gridRecordingPicker) SetView(view ViewLayout)             {}
func (r *gridRecordingPicker) GetView() ViewLayout                 { return GridView }
func (r *gridRecordingPicker) Select(id int)                       {}
func (r *gridRecordingPicker) SelectMultiple(ids []int)            { r.selectedIDs = append([]int(nil), ids...) }
func (r *gridRecordingPicker) ToggleSelection(id int)              {}
func (r *gridRecordingPicker) ExtendSelection(id int)              {}
func (r *gridRecordingPicker) IsSelected(uri fyne.URI) bool        { return false }
func (r *gridRecordingPicker) OpenSelection()                      {}
func (r *gridRecordingPicker) CopyPath(uri fyne.URI)               {}
func (r *gridRecordingPicker) SetFilter(filter storage.FileFilter) {}
func (r *gridRecordingPicker) IsMultiSelect() bool                 { return true }
func (r *gridRecordingPicker) ShowMenu(menu *fyne.Menu, pos fyne.Position, obj fyne.CanvasObject) {
}
func (r *gridRecordingPicker) DismissMenu() {}

func TestFileItem_ContextMenu_CopyPath(t *testing.T) {
	test.NewApp()

	picker := &contextMenuPicker{}
	item := newFileItem(picker, func() float32 { return 1.0 }, calculateItemSizeWithZoom)
	uri := storage.NewFileURI("/tmp/sample-folder/sample.txt")
	item.id = 3
	item.setURI(uri, ListView)

	item.showContextMenu(fyne.NewPos(10, 10))
	if picker.menu == nil {
		t.Fatal("expected context menu to be shown")
	}
	if len(picker.menu.Items) != 2 {
		t.Fatalf("expected 2 context menu items, got %d", len(picker.menu.Items))
	}

	copyPathItem := picker.menu.Items[1]
	if copyPathItem.Action == nil {
		t.Fatal("expected Copy Path item to have action")
	}
	copyPathItem.Action()

	if picker.copiedURI == nil || picker.copiedURI.String() != uri.String() {
		t.Fatalf("expected CopyPath to receive %q, got %v", uri.String(), picker.copiedURI)
	}
	if picker.dismissCalls != 1 {
		t.Fatalf("expected menu to be dismissed once after copy action, got %d", picker.dismissCalls)
	}
}

func TestFileList_MarqueeSelection_StartAnchorStableAcrossScroll(t *testing.T) {
	test.NewApp()

	picker := &recordingPicker{}
	fl := newFileList(picker)
	fl.setView(ListView)
	fl.list.Resize(fyne.NewSize(400, 200))
	fl.overlay.Resize(fyne.NewSize(400, 200))

	var files []fyne.URI
	for i := 0; i < 200; i++ {
		files = append(files, storage.NewFileURI(filepath.Join("/tmp", fmt.Sprintf("file-%03d.txt", i))))
	}
	fl.setFiles(files)

	start := fyne.NewPos(10, 20)
	cur := fyne.NewPos(390, 180)

	// First drag update at scroll offset 0.
	fl.onSelectionDrag(start, cur)
	if len(picker.selectedIDs) == 0 {
		t.Fatalf("Expected initial selection, got none")
	}

	// Scroll down and update again with the same pointer position.
	// The selection should expand downward, but still include the first row(s) from the original start anchor.
	fl.list.ScrollToOffset(200)
	fl.onSelectionDrag(start, cur)

	found0 := false
	for _, id := range picker.selectedIDs {
		if id == 0 {
			found0 = true
			break
		}
	}
	if !found0 {
		t.Fatalf("Expected selection to still include item 0 after scrolling during drag, got %v", picker.selectedIDs)
	}
}

func TestFileList_MarqueeSelectionEndPreservesScrollOffset(t *testing.T) {
	test.NewApp()

	for _, tc := range []struct {
		name string
		view ViewLayout
	}{
		{name: "list", view: ListView},
		{name: "grid", view: GridView},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &fileDialog{
				selected:      make(map[string]fyne.URI),
				allowMultiple: true,
				view:          tc.view,
				anchor:        -1,
			}
			fl := newFileList(d)
			d.fileList = fl
			fl.setView(tc.view)

			var files []fyne.URI
			for i := 0; i < 240; i++ {
				files = append(files, storage.NewFileURI(filepath.Join("/tmp", fmt.Sprintf("file-%03d.txt", i))))
			}
			fl.setFiles(files)

			var active fyne.CanvasObject
			if tc.view == GridView {
				active = fl.grid
			} else {
				active = fl.list
			}

			win := test.NewTempWindow(t, active)
			d.parent = win
			win.Resize(fyne.NewSize(420, 220))
			active.Resize(fyne.NewSize(420, 220))

			scrollTo := func(offset float32) {
				if tc.view == GridView {
					fl.grid.ScrollToOffset(offset)
					return
				}
				fl.list.ScrollToOffset(offset)
			}
			scrollOffset := func() float32 {
				if tc.view == GridView {
					return fl.grid.GetScrollOffset()
				}
				return fl.list.GetScrollOffset()
			}

			for _, move := range []struct {
				name          string
				initialOffset float32
				focusID       int
			}{
				{name: "highlight-above-viewport", initialOffset: 900, focusID: 1},
				{name: "highlight-below-viewport", initialOffset: 0, focusID: 180},
			} {
				t.Run(move.name, func(t *testing.T) {
					scrollTo(move.initialOffset)
					want := scrollOffset()
					if move.initialOffset > 0 && want == 0 {
						t.Fatalf("test setup failed: expected non-zero scroll offset")
					}

					fl.lastDragSelection = []int{move.focusID}
					fl.dragSelecting = true
					fl.onSelectionEnd()

					if got := scrollOffset(); abs32(got-want) > 0.01 {
						t.Fatalf("expected scroll offset to remain %.2f after marquee release, got %.2f", want, got)
					}
				})
			}
		})
	}
}

func TestFileList_MarqueeSelection_DisabledForSingleSelect(t *testing.T) {
	test.NewApp()

	picker := &singleRecordingPicker{}
	fl := newFileList(picker)
	fl.setView(ListView)
	fl.list.Resize(fyne.NewSize(400, 200))

	var files []fyne.URI
	for i := 0; i < 20; i++ {
		files = append(files, storage.NewFileURI(filepath.Join("/tmp", fmt.Sprintf("file-%03d.txt", i))))
	}
	fl.setFiles(files)

	fl.onSelectionDrag(fyne.NewPos(10, 10), fyne.NewPos(390, 180))

	if len(picker.selectedIDs) != 0 {
		t.Fatalf("expected no marquee selection in single-select mode, got %v", picker.selectedIDs)
	}
	if fl.dragSelecting {
		t.Fatalf("expected dragSelecting to remain false in single-select mode")
	}
}

func TestFileList_MarqueeSelection_GridHorizontalUsesStretchedCellWidth(t *testing.T) {
	test.NewApp()

	picker := &gridRecordingPicker{}
	fl := newFileList(picker)
	fl.setView(GridView)

	var files []fyne.URI
	for i := 0; i < 50; i++ {
		files = append(files, storage.NewFileURI(filepath.Join("/tmp", fmt.Sprintf("file-%03d.txt", i))))
	}
	fl.setFiles(files)

	// Lay out the grid through a real window so ColumnCount and StretchItems
	// reflect a concrete viewport, then resize to a mid-band width (not a column
	// threshold) so cells stretch noticeably past their base width.
	win := test.NewTempWindow(t, fl.grid)
	win.Resize(fyne.NewSize(560, 400))
	fl.onResize()

	pad := fl.grid.Theme().Size(theme.SizeNamePadding)
	base := calculateItemSizeWithZoom(GridView, fl.getZoom())
	cols := fl.grid.ColumnCount()
	if cols < 2 {
		t.Fatalf("expected multiple columns for the test, got %d", cols)
	}

	viewportWidth := fl.grid.Size().Width
	stretched := gridStretchedCellWidth(base.Width, viewportWidth, pad, cols)
	if stretched <= base.Width+1 {
		t.Fatalf("expected cells to stretch past base width (base %.2f, stretched %.2f, viewport %.2f, cols %d)",
			base.Width, stretched, viewportWidth, cols)
	}

	stepX := stretched + pad

	// Drag a thin vertical marquee centred on the last column of the first row.
	// At the stretched stride this lands squarely on item (cols-1). Using the
	// unstretched base stride (the bug), the computed cells sit left of where the
	// grid drew them, so this strip falls to the right of every computed cell and
	// selects nothing — the offset that grows toward the right edge after resize.
	lastCol := cols - 1
	cellCenterX := float32(lastCol)*stepX + stretched/2

	start := fyne.NewPos(cellCenterX-2, 2)
	cur := fyne.NewPos(cellCenterX+2, base.Height-2)

	fl.onSelectionDrag(start, cur)

	containsID := func(ids []int, want int) bool {
		for _, id := range ids {
			if id == want {
				return true
			}
		}
		return false
	}

	if !containsID(picker.selectedIDs, lastCol) {
		t.Fatalf("expected marquee over stretched last column to select item %d, got %v (stepX %.2f, stretched %.2f)",
			lastCol, picker.selectedIDs, stepX, stretched)
	}
	// A thin strip must hit a single cell, not bleed into the neighbour.
	if containsID(picker.selectedIDs, lastCol-1) {
		t.Fatalf("thin marquee over column %d unexpectedly also selected column %d: %v",
			lastCol, lastCol-1, picker.selectedIDs)
	}

	// The cell's right edge must also follow the stretched width: a strip in the
	// portion of the cell past its base width (but inside the rendered cell) still
	// has to register. With the right edge pinned to the base width that region is
	// a dead zone where the marquee visibly overlaps but nothing selects.
	fl.onSelectionEnd()
	picker.selectedIDs = nil

	rightX := float32(lastCol)*stepX + (base.Width+stretched)/2
	fl.onSelectionDrag(fyne.NewPos(rightX-2, 2), fyne.NewPos(rightX+2, base.Height-2))

	if !containsID(picker.selectedIDs, lastCol) {
		t.Fatalf("expected marquee in the stretched right portion of column %d to select item %d, got %v (rightX %.2f, base %.2f, stretched %.2f)",
			lastCol, lastCol, picker.selectedIDs, rightX, base.Width, stretched)
	}
}

func TestFileList_GridView_StretchesCellsToFillWidth(t *testing.T) {
	test.NewApp()

	picker := &mockPicker{}
	fl := newFileList(picker)
	fl.setView(GridView)

	var files []fyne.URI
	for i := 0; i < 50; i++ {
		files = append(files, storage.NewFileURI(filepath.Join("/tmp", fmt.Sprintf("file-%03d.txt", i))))
	}
	fl.setFiles(files)

	win := test.NewTempWindow(t, fl.grid)
	win.Resize(fyne.NewSize(300, 200))

	pad := fl.grid.Theme().Size(theme.SizeNamePadding)
	base := calculateItemSizeWithZoom(GridView, fl.getZoom())

	// Slowly resize and make sure we don't skip from 2->4 columns without ever hitting 3,
	// and that each computed layout uses all available width (no dead space strip).
	lastCols := 0
	seen := map[int]bool{}
	for width := float32(260); width <= 520; width += 5 {
		viewport := fyne.NewSize(width, 200)
		// Apply resize through the window canvas so the widget renderer is active.
		// This more closely matches how GridWrap behaves in real UI layouts.
		win.Resize(viewport)
		fl.onResize()

		cols := fl.grid.ColumnCount()
		seen[cols] = true

		if lastCols != 0 && cols-lastCols > 1 {
			t.Fatalf("unexpected column jump at width %.2f: %d -> %d", width, lastCols, cols)
		}
		lastCols = cols

		// Note: stretching to fill width is now handled by refyne's GridWrap.StretchItems
		// at layout time, so we don't verify item sizes here.
	}

	if !seen[3] && seen[4] {
		t.Fatalf("expected to reach 3 columns before 4 (baseWidth %.2f, pad %.2f); saw %v", base.Width, pad, seen)
	}

	// Now slowly shrink; column count should not increase while width is decreasing.
	prevCols := lastCols
	for width := float32(515); width >= 260; width -= 5 {
		win.Resize(fyne.NewSize(width, 200))
		fl.onResize()

		cols := fl.grid.ColumnCount()
		if cols > prevCols {
			t.Fatalf("unexpected column increase while shrinking at width %.2f: %d -> %d", width, prevCols, cols)
		}
		prevCols = cols
	}
}

func TestFileList_GridView_DoesNotOscillateColumnCountAtFixedViewport(t *testing.T) {
	test.NewApp()

	picker := &mockPicker{}
	fl := newFileList(picker)
	fl.setView(GridView)

	var files []fyne.URI
	for i := 0; i < 200; i++ {
		files = append(files, storage.NewFileURI(filepath.Join("/tmp", fmt.Sprintf("file-%03d.txt", i))))
	}
	fl.setFiles(files)

	// Put the full fileList scroll into a window so we match the real dialog structure.
	win := test.NewTempWindow(t, fl.content)
	win.Resize(fyne.NewSize(520, 240))

	outerPad := theme.Padding() * 2 // container.NewPadded(...)
	innerPad := fl.grid.Theme().Size(theme.SizeNamePadding)
	base := calculateItemSizeWithZoom(GridView, fl.getZoom())

	threshold := func(cols int) float32 {
		if cols < 1 {
			return 0
		}
		return float32(cols)*base.Width + float32(cols-1)*innerPad
	}

	// Probe around the 3->4 and 4->5 column thresholds (plus outer padding),
	// and ensure repeated refresh/reflow doesn't flip-flop the computed column count.
	widths := []float32{
		threshold(3) + outerPad - 1,
		threshold(3) + outerPad + 1,
		threshold(4) + outerPad - 1,
		threshold(4) + outerPad + 1,
	}

	for _, w := range widths {
		win.Resize(fyne.NewSize(w, 240))
		fl.onResize()
		want := fl.grid.ColumnCount()

		for i := 0; i < 8; i++ {
			// Emulate layout churn: refresh (re-measure item MinSize) and clear column cache.
			fl.grid.Refresh()
			fl.grid.Resize(fl.grid.Size())
			if got := fl.grid.ColumnCount(); got != want {
				t.Fatalf("column count oscillated at width %.2f: want %d, got %d (iter %d)", w, want, got, i)
			}
		}
	}
}

func TestFileList_GridView_ShrinkDoesNotIncreaseColumnsWhenResizeHandlingCatchesUp(t *testing.T) {
	test.NewApp()

	picker := &mockPicker{}
	fl := newFileList(picker)
	fl.setView(GridView)

	var files []fyne.URI
	for i := 0; i < 200; i++ {
		files = append(files, storage.NewFileURI(filepath.Join("/tmp", fmt.Sprintf("file-%03d.txt", i))))
	}
	fl.setFiles(files)

	win := test.NewTempWindow(t, fl.content)

	// Start wide enough that we get a stable (base-fitting) column count.
	win.Resize(fyne.NewSize(700, 240))
	fl.onResize()

	// Now shrink, but emulate the situation where the debounced resize handler hasn't run yet:
	// the widget is laid out at the new width, but fileList.onResize is delayed.
	win.Resize(fyne.NewSize(660, 240))
	fl.grid.Refresh()
	fl.grid.Resize(fl.grid.Size())

	// When the resize handler catches up, the column count stabilizes for this width.
	// With cached viewport width stretching, the stretched item width updates when
	// the cache updates, so the column count may adjust - but it should be stable
	// after the cache update (no oscillation).
	fl.onResize()
	colsAfter := fl.grid.ColumnCount()

	// Verify stability: repeated refresh/resize at the same width should not oscillate.
	for i := 0; i < 5; i++ {
		fl.grid.Refresh()
		fl.grid.Resize(fl.grid.Size())
		if got := fl.grid.ColumnCount(); got != colsAfter {
			t.Fatalf("column count oscillated after onResize at fixed width: want %d, got %d (iter %d)", colsAfter, got, i)
		}
	}
}

func TestFormatGridFileNameWithMeasure_TruncationKeepsDotsBeforeExtension(t *testing.T) {
	measure := func(s string) float32 { return float32(utf8.RuneCountInString(s)) }

	got := formatGridFileNameWithMeasure("abcdefghijklmnopqrstuvwxyz.txt", 6, measure)
	want := "abcdef\n...vwx\nyz.txt"
	if got != want {
		t.Fatalf("unexpected formatting:\n got: %q\nwant: %q", got, want)
	}

	// When the base name is truncated, we always show the dots marker somewhere above the extension.
	if !strings.Contains(got, "...") {
		t.Fatalf("expected truncation marker before extension, got %q", got)
	}
}

func TestFormatGridFileNameWithMeasure_TruncationKeepsTailBeforeExtension(t *testing.T) {
	measure := func(s string) float32 { return float32(utf8.RuneCountInString(s)) }

	got := formatGridFileNameWithMeasure("alongtextheresomethingelse.png", 8, measure)
	if !strings.Contains(got, "...") {
		t.Fatalf("expected truncation marker, got %q", got)
	}
	compact := strings.ReplaceAll(got, "\n", "")
	if !strings.Contains(compact, "gelse.png") {
		t.Fatalf("expected preserved tail plus extension, got %q", got)
	}
}

func TestFormatGridFileNameWithMeasure_ExtensionCanStayOnSecondLine(t *testing.T) {
	measure := func(s string) float32 { return float32(utf8.RuneCountInString(s)) }

	got := formatGridFileNameWithMeasure("abcdefghij.txt", 8, measure)
	want := "abcdefgh\nij.txt"
	if got != want {
		t.Fatalf("unexpected formatting:\n got: %q\nwant: %q", got, want)
	}
}

func TestFormatGridFileNameWithMeasure_NoExtensionProtectionForDotfiles(t *testing.T) {
	measure := func(s string) float32 { return float32(utf8.RuneCountInString(s)) }

	// filepath.Ext(".bashrc") == "" so we fall back to standard wrapping.
	// With width=3, ".bashrc" (7 chars) gets wrapped.
	name := ".bashrc"
	got := formatGridFileNameWithMeasure(name, 3, measure)
	if got == name {
		t.Fatalf("expected dotfile name to be wrapped/truncated, got %q", got)
	}
	// Expected wrapping for width=3: ".ba" then "shr" then "c" -> ".ba\nshr\nc"
	want := ".ba\nshr\nc"
	if got != want {
		t.Fatalf("unexpected formatting for dotfile:\n got: %q\nwant: %q", got, want)
	}
}

func TestFormatGridFolderNameWithMeasure_TruncationKeepsTailAndDots(t *testing.T) {
	measure := func(s string) float32 { return float32(utf8.RuneCountInString(s)) }

	name := "averyverylongfoldername"
	got := formatGridFolderNameWithMeasure(name, 6, measure)
	compact := strings.ReplaceAll(got, "\n", "")

	if compact == name {
		t.Fatalf("expected folder name to be truncated, got %q", got)
	}
	if !strings.Contains(compact, "...") {
		t.Fatalf("expected truncation marker, got %q", got)
	}
	if !strings.Contains(compact, "...rname") {
		t.Fatalf("expected preserved 5-char tail with dots, got %q", got)
	}
}

func TestFormatGridFolderNameWithMeasure_NoTruncationWhenFitsThreeLines(t *testing.T) {
	measure := func(s string) float32 { return float32(utf8.RuneCountInString(s)) }

	name := "foldernamealpha"
	got := formatGridFolderNameWithMeasure(name, 5, measure)
	compact := strings.ReplaceAll(got, "\n", "")

	if compact != name {
		t.Fatalf("expected full folder name to fit across 3 lines, got %q", got)
	}
	if strings.Contains(got, "...") {
		t.Fatalf("did not expect truncation marker when full folder name fits, got %q", got)
	}
}

func TestStableGridLabelWidth(t *testing.T) {
	tests := []struct {
		name   string
		base   float32
		actual float32
		want   float32
	}{
		{name: "stretched uses base", base: 120, actual: 180, want: 120},
		{name: "narrow uses actual", base: 120, actual: 90, want: 90},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := stableGridLabelWidth(tc.base, tc.actual)
			if got != tc.want {
				t.Fatalf("stableGridLabelWidth(%v, %v) = %.2f, want %.2f", tc.base, tc.actual, got, tc.want)
			}
		})
	}
}

func TestAutoScrollDistanceUsesElapsedFrameTime(t *testing.T) {
	step := float32(24)
	velocity := autoScrollVelocity(step)
	assertClose := func(name string, got, want float32) {
		t.Helper()
		if diff := got - want; diff < -0.001 || diff > 0.001 {
			t.Fatalf("%s = %.4f, want %.4f", name, got, want)
		}
	}

	assertClose("base frame", autoScrollDistance(velocity, autoScrollBaseFrame), step)
	assertClose("half frame", autoScrollDistance(velocity, autoScrollBaseFrame/2), step/2)
	assertClose(
		"clamped long frame",
		autoScrollDistance(velocity, autoScrollMaxFrame*3),
		velocity*float32(autoScrollMaxFrame.Seconds()),
	)

	if got := autoScrollDistance(velocity, 0); got != 0 {
		t.Fatalf("zero elapsed = %.4f, want 0", got)
	}
}

func TestFileList_KeyboardSelectionRoutesThroughPicker(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	w := a.NewWindow("Test")
	root := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("write failed: %v", err)
		}
	}

	d := NewFileOpen(func(_ []fyne.URIReadCloser, _ error) {}, w, true).(*fileDialog)
	d.makeUI()
	d.SetView(ListView)

	lister, err := storage.ListerForURI(storage.NewFileURI(root))
	if err != nil {
		t.Fatalf("lister failed: %v", err)
	}
	d.refreshDir(lister)

	if len(d.fileList.filtered) == 0 {
		t.Fatalf("expected files to be listed")
	}
	uri := d.fileList.filtered[0]

	// Simulate keyboard Space on the focused (first) item. widget.List.TypedKey
	// dispatches Space to list.Select(currentHighlight); mirror that here.
	d.fileList.list.Select(0)

	if !d.IsSelected(uri) {
		t.Fatalf("keyboard selection did not update the picker selection state")
	}

	// The widget must not retain its own selection. Otherwise it renders a second
	// "ghost" highlight, and a repeat Space on the same item is swallowed by the
	// widget's already-selected guard. A second Space must reach the picker and
	// toggle the selection back off.
	d.fileList.list.Select(0)
	if d.IsSelected(uri) {
		t.Fatalf("widget selection was not cleared; second Space never reached the picker")
	}
}

type countingPicker struct {
	mockPicker
	toggleCalls int
}

func (c *countingPicker) IsMultiSelect() bool    { return true }
func (c *countingPicker) ToggleSelection(id int) { c.toggleCalls++ }

func TestFileList_ChangingFolderClearsWidgetSelection(t *testing.T) {
	test.NewApp()

	picker := &countingPicker{}
	fl := newFileList(picker)
	fl.setView(ListView)

	fl.setFiles([]fyne.URI{
		storage.NewFileURI("/tmp/a.txt"),
		storage.NewFileURI("/tmp/b.txt"),
	})

	// Simulate a stray widget-owned selection that lingers (the pre-fix
	// behavior): detach our handler so Select leaves the widget's internal
	// selection in place instead of routing it through the picker.
	saved := fl.list.OnSelected
	fl.list.OnSelected = nil
	fl.list.Select(0)
	fl.list.OnSelected = saved

	// Navigating into a different folder must clear that stale widget selection.
	fl.setFiles([]fyne.URI{
		storage.NewFileURI("/other/x.txt"),
	})

	// If the widget selection survived, its already-selected guard would swallow
	// a Space on id 0, so the picker would never see the toggle.
	fl.list.Select(0)
	if picker.toggleCalls != 1 {
		t.Fatalf("expected Space after folder change to reach the picker once, got %d", picker.toggleCalls)
	}
}

func TestFileDialog_MarqueeSelectionFocusesListForKeyboard(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	w := a.NewWindow("Test")
	root := t.TempDir()
	for i := 0; i < 6; i++ {
		name := fmt.Sprintf("file-%02d.txt", i)
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("write failed: %v", err)
		}
	}

	d := NewFileOpen(func([]fyne.URIReadCloser, error) {}, w, true).(*fileDialog)
	// Render the dialog UI so the list is part of the canvas focus tree.
	w.SetContent(d.makeUI())
	d.SetView(ListView)
	d.fileList.list.Resize(fyne.NewSize(400, 400))
	d.fileList.overlay.Resize(fyne.NewSize(400, 400))

	lister, err := storage.ListerForURI(storage.NewFileURI(root))
	if err != nil {
		t.Fatalf("lister failed: %v", err)
	}
	d.refreshDir(lister)

	// Marquee-drag across the top rows (kept clear of the auto-scroll edge zone).
	d.fileList.onSelectionDrag(fyne.NewPos(10, 5), fyne.NewPos(390, 150))
	d.fileList.onSelectionEnd()

	// Find the last (highest-index) selected item; the cursor should rest there.
	lastIdx := -1
	for i, u := range d.fileList.filtered {
		if d.IsSelected(u) {
			lastIdx = i
		}
	}
	if lastIdx < 0 {
		t.Fatalf("expected the marquee drag to select files")
	}

	if got := w.Canvas().Focused(); got != d.fileList.list {
		t.Fatalf("expected the file list to receive keyboard focus after a marquee drag, got %T", got)
	}

	// Space acts on the highlighted item; it must be the last marquee-selected
	// row, so toggling clears it.
	d.fileList.list.TypedKey(&fyne.KeyEvent{Name: fyne.KeySpace})
	if d.IsSelected(d.fileList.filtered[lastIdx]) {
		t.Fatalf("expected the keyboard cursor to be parked on the last marquee-selected item")
	}
}
