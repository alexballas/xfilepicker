package dialog

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestFileDialog_CopyPath(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	d := &fileDialog{}
	uri := storage.NewFileURI("/tmp/demo-folder/demo-file.txt")
	d.CopyPath(uri)

	if got, want := a.Clipboard().Content(), "/tmp/demo-folder/demo-file.txt"; got != want {
		t.Fatalf("expected clipboard content %q, got %q", want, got)
	}
}

func TestResizeLayout_OnResizeWhenExternalSizeChanges(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	callbacks := 0
	external := fyne.NewSize(1200, 800)
	r := &resizeLayout{
		internal: layout.NewStackLayout(),
		onResize: func() {
			callbacks++
		},
		externalSize: func() fyne.Size {
			return external
		},
	}

	contentSize := fyne.NewSize(700, 500)
	r.Layout(nil, contentSize)
	fyne.DoAndWait(func() {})
	if callbacks != 1 {
		t.Fatalf("expected 1 resize callback after initial layout, got %d", callbacks)
	}

	// No internal or external size change should not trigger callback.
	r.lastFired = time.Now().Add(-time.Second)
	r.Layout(nil, contentSize)
	fyne.DoAndWait(func() {})
	if callbacks != 1 {
		t.Fatalf("expected callback count to stay at 1, got %d", callbacks)
	}

	// External size change should trigger callback even when content size is unchanged.
	external = fyne.NewSize(1300, 800)
	r.lastFired = time.Now().Add(-time.Second)
	r.Layout(nil, contentSize)
	fyne.DoAndWait(func() {})
	if callbacks != 2 {
		t.Fatalf("expected callback count to be 2 after external resize, got %d", callbacks)
	}
}

func TestFolderDialog_RefreshDirOnlyShowsDirectories(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	w := a.NewWindow("Test")
	root := t.TempDir()
	dirChild := filepath.Join(root, "child")
	if err := os.MkdirAll(dirChild, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	d := NewFolderOpen(func(_ fyne.ListableURI, _ error) {}, w).(*fileDialog)
	d.fileList = newFileList(d)
	lister, err := storage.ListerForURI(storage.NewFileURI(root))
	if err != nil {
		t.Fatalf("lister failed: %v", err)
	}

	d.refreshDir(lister)
	if len(d.fileList.filtered) != 1 {
		t.Fatalf("expected exactly 1 visible item in folder mode, got %d", len(d.fileList.filtered))
	}
	if got := d.fileList.filtered[0].Path(); got != dirChild {
		t.Fatalf("expected only directory %q, got %q", dirChild, got)
	}
}

func TestFolderDialog_OpenUsesSelectionOrCurrentDirectory(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	w := a.NewWindow("Test")
	root := t.TempDir()
	dirChild := filepath.Join(root, "child")
	if err := os.MkdirAll(dirChild, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	var opened []string
	d := NewFolderOpen(func(dir fyne.ListableURI, err error) {
		if err != nil {
			t.Fatalf("unexpected callback error: %v", err)
		}
		if dir == nil {
			opened = append(opened, "")
			return
		}
		opened = append(opened, dir.Path())
	}, w).(*fileDialog)

	d.makeUI()
	lister, err := storage.ListerForURI(storage.NewFileURI(root))
	if err != nil {
		t.Fatalf("lister failed: %v", err)
	}
	d.refreshDir(lister)

	if d.open.Disabled() {
		t.Fatalf("expected Open button to be enabled in folder mode with no selection")
	}

	d.open.OnTapped()
	if len(opened) != 1 || opened[0] != root {
		t.Fatalf("expected Open with no selection to return current dir %q, got %v", root, opened)
	}

	selectedIdx := -1
	for i, u := range d.fileList.filtered {
		if u.Path() == dirChild {
			selectedIdx = i
			break
		}
	}
	if selectedIdx == -1 {
		t.Fatalf("expected to find directory %q in filtered list", dirChild)
	}

	d.Select(selectedIdx)
	d.open.OnTapped()
	if len(opened) != 2 || opened[1] != dirChild {
		t.Fatalf("expected Open with selection to return %q, got %v", dirChild, opened)
	}
}

func TestFolderDialog_EnterNavigatesSelectedDirectory(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	w := a.NewWindow("Test")
	w.SetContent(container.NewVBox(widget.NewLabel("x")))

	root := t.TempDir()
	dirChild := filepath.Join(root, "child")
	if err := os.MkdirAll(dirChild, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	callbacks := 0
	d := NewFolderOpen(func(_ fyne.ListableURI, _ error) {
		callbacks++
	}, w).(*fileDialog)

	d.makeUI()
	d.win = widget.NewModalPopUp(container.NewVBox(), w.Canvas())
	lister, err := storage.ListerForURI(storage.NewFileURI(root))
	if err != nil {
		t.Fatalf("lister failed: %v", err)
	}
	d.refreshDir(lister)

	selectedIdx := -1
	for i, u := range d.fileList.filtered {
		if u.Path() == dirChild {
			selectedIdx = i
			break
		}
	}
	if selectedIdx == -1 {
		t.Fatalf("expected to find directory %q in filtered list", dirChild)
	}

	d.Select(selectedIdx)
	w.Canvas().Unfocus()
	d.typedKeyHook(&fyne.KeyEvent{Name: fyne.KeyReturn})

	if d.dir == nil || d.dir.Path() != dirChild {
		t.Fatalf("expected Enter to navigate to %q, got %v", dirChild, d.dir)
	}
	if callbacks != 0 {
		t.Fatalf("expected Enter navigation to not invoke callback, got %d calls", callbacks)
	}
}

func TestMergeSaveSelectionName(t *testing.T) {
	tests := []struct {
		name     string
		current  string
		selected string
		want     string
	}{
		{name: "empty current replaces", current: "", selected: "file.txt", want: "file.txt"},
		{name: "path suffix appends", current: "subdir/", selected: "file.txt", want: "subdir/file.txt"},
		{name: "non-path current replaces", current: "old.txt", selected: "new.txt", want: "new.txt"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := mergeSaveSelectionName(tc.current, tc.selected); got != tc.want {
				t.Fatalf("mergeSaveSelectionName(%q, %q) = %q, want %q", tc.current, tc.selected, got, tc.want)
			}
		})
	}
}

func TestFileSaveDialog_SaveButtonEnabledByNameEntry(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	w := a.NewWindow("Test")
	d := NewFileSave(func(_ fyne.URIWriteCloser, _ error) {}, w).(*fileDialog)
	d.makeUI()

	if !d.open.Disabled() {
		t.Fatal("expected Save button to be disabled when filename is empty")
	}

	d.saveName.SetText("output.txt")
	if d.open.Disabled() {
		t.Fatal("expected Save button to be enabled when filename is set")
	}

	d.saveName.SetText("   ")
	if !d.open.Disabled() {
		t.Fatal("expected Save button to be disabled when filename is blank")
	}
}

func TestFileSaveDialog_SelectUpdatesNameEntry(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	w := a.NewWindow("Test")
	root := t.TempDir()
	dirChild := filepath.Join(root, "child")
	fileChild := filepath.Join(root, "file.txt")
	if err := os.MkdirAll(dirChild, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(fileChild, []byte("x"), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	d := NewFileSave(func(_ fyne.URIWriteCloser, _ error) {}, w).(*fileDialog)
	d.makeUI()

	lister, err := storage.ListerForURI(storage.NewFileURI(root))
	if err != nil {
		t.Fatalf("lister failed: %v", err)
	}
	d.refreshDir(lister)

	fileIdx := -1
	dirIdx := -1
	for i, u := range d.fileList.filtered {
		if u.Path() == fileChild {
			fileIdx = i
		}
		if u.Path() == dirChild {
			dirIdx = i
		}
	}
	if fileIdx == -1 || dirIdx == -1 {
		t.Fatalf("expected to find both file and dir in filtered entries, got file=%d dir=%d", fileIdx, dirIdx)
	}

	d.Select(fileIdx)
	if got, want := d.saveName.Text, "file.txt"; got != want {
		t.Fatalf("expected file selection to set save name %q, got %q", want, got)
	}

	d.saveName.SetText("nested/")
	d.Select(dirIdx)
	if got, want := d.saveName.Text, "nested/child"; got != want {
		t.Fatalf("expected directory selection to append to path prefix %q, got %q", want, got)
	}
}

func TestFileSaveDialog_SaveNewFileReturnsWriter(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	w := a.NewWindow("Test")
	root := t.TempDir()

	var (
		gotWriter fyne.URIWriteCloser
		gotErr    error
	)
	d := NewFileSave(func(writer fyne.URIWriteCloser, err error) {
		gotWriter = writer
		gotErr = err
	}, w).(*fileDialog)
	d.makeUI()

	lister, err := storage.ListerForURI(storage.NewFileURI(root))
	if err != nil {
		t.Fatalf("lister failed: %v", err)
	}
	d.refreshDir(lister)
	d.saveName.SetText("new.txt")

	d.open.OnTapped()

	if gotErr != nil {
		t.Fatalf("unexpected save callback error: %v", gotErr)
	}
	if gotWriter == nil {
		t.Fatal("expected save callback to return a writer")
	}
	if got, want := gotWriter.URI().Path(), filepath.Join(root, "new.txt"); got != want {
		t.Fatalf("expected writer URI %q, got %q", want, got)
	}
	_ = gotWriter.Close()
}

func TestFileSaveDialog_OverwriteUsesConfirmation(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	w := a.NewWindow("Test")
	root := t.TempDir()
	existing := filepath.Join(root, "existing.txt")
	if err := os.WriteFile(existing, []byte("old"), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	callbackCalls := 0
	var gotWriter fyne.URIWriteCloser
	d := NewFileSave(func(writer fyne.URIWriteCloser, err error) {
		callbackCalls++
		if err != nil {
			t.Fatalf("unexpected callback error: %v", err)
		}
		gotWriter = writer
	}, w).(*fileDialog)
	d.makeUI()

	lister, err := storage.ListerForURI(storage.NewFileURI(root))
	if err != nil {
		t.Fatalf("lister failed: %v", err)
	}
	d.refreshDir(lister)
	d.saveName.SetText("existing.txt")

	confirmCalls := 0
	d.confirmOverwrite = func(target fyne.URI, confirm func(bool)) {
		confirmCalls++
		if target == nil || target.Path() != existing {
			t.Fatalf("expected overwrite target %q, got %v", existing, target)
		}
		confirm(false)
	}

	d.open.OnTapped()
	if confirmCalls != 1 {
		t.Fatalf("expected one overwrite confirm call, got %d", confirmCalls)
	}
	if callbackCalls != 0 {
		t.Fatalf("expected callback to not run when overwrite is rejected, got %d calls", callbackCalls)
	}

	d.confirmOverwrite = func(_ fyne.URI, confirm func(bool)) {
		confirmCalls++
		confirm(true)
	}
	d.open.OnTapped()
	if confirmCalls != 2 {
		t.Fatalf("expected second overwrite confirm call, got %d", confirmCalls)
	}
	if callbackCalls != 1 || gotWriter == nil {
		t.Fatalf("expected callback with writer after confirmed overwrite, calls=%d writer=%v", callbackCalls, gotWriter)
	}
	_ = gotWriter.Close()
}
