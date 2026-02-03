package dialog

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/test"
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
