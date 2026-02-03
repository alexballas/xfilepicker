package dialog

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/FyshOS/fancyfs"
)

type sidebar struct {
	picker FilePicker
	list   *widget.List
	items  []favoriteItem
}

func newSidebar(p FilePicker) *sidebar {
	s := &sidebar{
		picker: p,
		items:  []favoriteItem{},
	}
	s.loadFavorites()

	s.list = widget.NewList(
		func() int { return len(s.items) },
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewIcon(theme.DocumentIcon()),
				widget.NewLabel(lang.L("Template")),
			)
		},
		func(id widget.ListItemID, o fyne.CanvasObject) {
			if id >= len(s.items) {
				return
			}
			item := s.items[id]
			box := o.(*fyne.Container)
			box.Objects[0].(*widget.Icon).SetResource(item.locIcon)
			box.Objects[1].(*widget.Label).SetText(lang.L(item.locName))
		},
	)
	s.list.OnSelected = func(id widget.ListItemID) {
		if id < len(s.items) {
			s.picker.SetLocation(s.items[id].loc)

			// Unfocus to allow Type-to-Search to capture keys immediately
			// (Sidebar list might capture keys if focused, preventing global hook fallback)
			if fd, ok := s.picker.(*fileDialog); ok {
				var c fyne.Canvas
				if fd.win != nil {
					c = fd.win.Canvas
				} else if fd.parent != nil {
					c = fd.parent.Canvas()
				}
				if c != nil {
					c.Unfocus()
				}
			}
		}
	}

	return s
}

func (s *sidebar) getFavoritesOrder() []string {
	order := []string{
		"Desktop",
		"Documents",
		"Downloads",
		"Music",
		"Pictures",
		"Videos",
	}

	if runtime.GOOS == "darwin" {
		order[5] = "Movies"
	}

	return order
}

func (s *sidebar) getFavoritesIcon(location string) fyne.Resource {
	switch location {
	case "Documents":
		return theme.DocumentIcon()
	case "Desktop":
		return theme.DesktopIcon()
	case "Downloads":
		return theme.DownloadIcon()
	case "Music":
		return theme.MediaMusicIcon()
	case "Pictures":
		return theme.MediaPhotoIcon()
	case "Videos", "Movies":
		return theme.MediaVideoIcon()
	}

	return nil
}

func (s *sidebar) hasAppFiles(a fyne.App) bool {
	if a.UniqueID() == "testApp" || a.Storage() == nil {
		return false
	}

	return len(a.Storage().List()) > 0
}

func (s *sidebar) storageURI(a fyne.App) fyne.URI {
	dir, _ := storage.Child(a.Storage().RootURI(), "Documents")
	return dir
}

func (s *sidebar) SyncSelection(uri fyne.URI) {
	if s.list == nil {
		return
	}

	for i, item := range s.items {
		if storage.EqualURI(item.loc, uri) {
			s.list.Select(i)
			return
		}
	}
	s.list.UnselectAll()
}

func (s *sidebar) loadFavorites() {
	s.items = []favoriteItem{}

	// Home
	homeDir, _ := os.UserHomeDir()
	homeURI := storage.NewFileURI(homeDir)
	if l, err := storage.ListerForURI(homeURI); err == nil {
		s.items = append(s.items, favoriteItem{
			locName: "Home",
			locIcon: theme.HomeIcon(),
			loc:     l,
		})
	}

	// App Files
	app := fyne.CurrentApp()
	if s.hasAppFiles(app) {
		uri := s.storageURI(app)
		if l, err := storage.ListerForURI(uri); err == nil {
			s.items = append(s.items, favoriteItem{
				locName: "App Files",
				locIcon: theme.FileIcon(),
				loc:     l,
			})
		}
	}

	// Places (Platform specific)
	s.items = append(s.items, s.getPlaces()...)

	// XDG / Standard Folders
	for _, name := range s.getFavoritesOrder() {
		uri, err := getFavoriteLocation(homeURI, name)
		if err == nil {
			if l, err := storage.ListerForURI(uri); err == nil {
				icon := s.getFavoritesIcon(name)
				// Override with fancyfs if possible (matches Fyne's theme support but better)
				if details, err := fancyfs.DetailsForFolder(uri); err == nil && details != nil && details.BackgroundResource != nil {
					icon = details.BackgroundResource
				}

				s.items = append(s.items, favoriteItem{
					locName: name,
					locIcon: icon,
					loc:     l,
				})
			}
		}
	}
}

func getFavoriteLocation(homeURI fyne.URI, name string) (fyne.URI, error) {
	if runtime.GOOS != "linux" && runtime.GOOS != "openbsd" && runtime.GOOS != "freebsd" && runtime.GOOS != "netbsd" {
		return storage.Child(homeURI, name)
	}

	// Linux XDG check
	const cmdName = "xdg-user-dir"
	if _, err := exec.LookPath(cmdName); err != nil {
		return storage.Child(homeURI, name)
	}

	lookupName := strings.ToUpper(name)
	cmd := exec.Command(cmdName, lookupName)
	loc, err := cmd.Output()
	if err != nil {
		return storage.Child(homeURI, name)
	}

	// Remove whitespace/newlines
	cleanPath := filepath.Clean(strings.TrimSpace(string(loc)))
	locURI := storage.NewFileURI(cleanPath)

	// Check if it points to home (after cleaning)
	if locURI.String() == homeURI.String() {
		// Fallback: try to construct path manually and resolve symlinks
		childPath := filepath.Join(homeURI.Path(), name)
		if resolved, err := filepath.EvalSymlinks(childPath); err == nil {
			return storage.NewFileURI(resolved), nil
		}
		return storage.NewFileURI(childPath), nil
	}

	return locURI, nil
}
