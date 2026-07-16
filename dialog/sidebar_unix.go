//go:build !windows && !android && !ios && !wasm && !js

package dialog

import (
	"path/filepath"
	"sort"

	"github.com/alexballas/refyne/v2"
	"github.com/alexballas/refyne/v2/storage"
	"github.com/alexballas/refyne/v2/theme"
)

func (s *sidebar) getPlaces() []favoriteItem {
	var places []favoriteItem

	lister, err := storage.ListerForURI(storage.NewFileURI("/"))
	if err != nil {
		fyne.LogError("could not create lister for /", err)
	} else {
		places = append(places, favoriteItem{
			locName: "Computer",
			locIcon: theme.ComputerIcon(),
			loc:     lister,
		})
	}

	return append(places, volumePlaces(externalVolumePaths())...)
}

func volumePlaces(paths []string) []favoriteItem {
	cleanPaths := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		if path == "." {
			continue
		}
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		cleanPaths = append(cleanPaths, path)
	}
	sort.Strings(cleanPaths)

	places := make([]favoriteItem, 0, len(cleanPaths))
	for _, path := range cleanPaths {
		lister, err := storage.ListerForURI(storage.NewFileURI(path))
		if err != nil {
			continue
		}

		name := filepath.Base(path)
		if name == "." || name == string(filepath.Separator) {
			name = path
		}
		places = append(places, favoriteItem{
			locName: name,
			locIcon: theme.StorageIcon(),
			loc:     lister,
		})
	}
	return places
}
