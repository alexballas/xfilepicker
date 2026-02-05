package dialog

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
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
func (r *recordingPicker) SetFilter(filter storage.FileFilter)                                {}
func (r *recordingPicker) IsMultiSelect() bool                                                { return true }
func (r *recordingPicker) ShowMenu(menu *fyne.Menu, pos fyne.Position, obj fyne.CanvasObject) {}
func (r *recordingPicker) DismissMenu()                                                       {}

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

		itemSize := fl.getItemSize()
		used := float32(cols)*itemSize.Width + float32(cols-1)*pad
		targetWidth := fl.grid.Size().Width
		if diff := abs32(used - targetWidth); diff > 0.6 {
			t.Fatalf("expected grid to fill width; used %.2f vs grid %.2f (diff %.2f, cols %d, pad %.2f, item %.2f)", used, targetWidth, diff, cols, pad, itemSize.Width)
		}
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
	colsBefore := fl.grid.ColumnCount()

	// When the resize handler catches up, columns must not increase at the same viewport width.
	fl.onResize()
	colsAfter := fl.grid.ColumnCount()
	if colsAfter > colsBefore {
		t.Fatalf("unexpected column increase after onResize catch-up at fixed width: %d -> %d", colsBefore, colsAfter)
	}
}

func TestFormatGridFileNameWithMeasure_TruncationKeepsDotsBeforeExtension(t *testing.T) {
	measure := func(s string) float32 { return float32(utf8.RuneCountInString(s)) }

	got := formatGridFileNameWithMeasure("abcdefghijklmnopqrstuvwxyz.txt", 6, measure)
	want := "abcdef\nghijkl\n...txt"
	if got != want {
		t.Fatalf("unexpected formatting:\n got: %q\nwant: %q", got, want)
	}

	// When the base name is truncated, we always show the dots marker somewhere above the extension.
	if !strings.Contains(got, "...") {
		t.Fatalf("expected truncation marker before extension, got %q", got)
	}
}

func TestFormatGridFileNameWithMeasure_NoExtensionProtectionForDotfiles(t *testing.T) {
	measure := func(s string) float32 { return float32(utf8.RuneCountInString(s)) }

	// filepath.Ext(".bashrc") == "" so we should not split/protect.
	name := ".bashrc"
	if got := formatGridFileNameWithMeasure(name, 3, measure); got != name {
		t.Fatalf("expected dotfile name unchanged, got %q", got)
	}
}
