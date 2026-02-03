//go:build !windows && !android && !ios && !wasm && !js

package dialog

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
)

func (s *sidebar) getPlaces() []favoriteItem {
	lister, err := storage.ListerForURI(storage.NewFileURI("/"))
	if err != nil {
		fyne.LogError("could not create lister for /", err)
		return []favoriteItem{}
	}
	return []favoriteItem{{
		locName: "Computer",
		locIcon: theme.ComputerIcon(),
		loc:     lister,
	}}
}
