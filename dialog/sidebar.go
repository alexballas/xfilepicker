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

func (s *sidebar) loadFavorites() {
	s.items = []favoriteItem{}

	// Home
	homeDir, _ := os.UserHomeDir()
	homeURI := storage.NewFileURI(homeDir)
	if l, err := storage.ListerForURI(homeURI); err == nil {
		icon := theme.HomeIcon()
		if details, err := fancyfs.DetailsForFolder(homeURI); err == nil && details != nil && details.BackgroundResource != nil {
			icon = details.BackgroundResource
		}
		s.items = append(s.items, favoriteItem{
			locName: "Home",
			locIcon: icon,
			loc:     l,
		})
	}

	// XDG Folders
	order := []string{"Desktop", "Documents", "Downloads", "Music", "Pictures", "Videos"}
	if runtime.GOOS == "darwin" {
		order = []string{"Desktop", "Documents", "Downloads", "Music", "Pictures", "Movies"}
	}

	for _, name := range order {
		uri, err := getFavoriteLocation(homeURI, name)
		if err == nil {
			if l, err := storage.ListerForURI(uri); err == nil {
				icon := theme.FolderIcon()
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

	// Drives / Root
	// Iterate through root items if possible, or just Show Root
	roots := storage.NewFileURI("/")
	if l, err := storage.ListerForURI(roots); err == nil {
		s.items = append(s.items, favoriteItem{
			locName: "Computer",
			locIcon: theme.ComputerIcon(),
			loc:     l,
		})
	}

	// Add Volumes if possible (TODO for later)
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
