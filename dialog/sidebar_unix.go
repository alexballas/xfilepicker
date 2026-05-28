//go:build !windows && !android && !ios && !wasm && !js

package dialog

import (
	"github.com/alexballas/refyne/v2"
	"github.com/alexballas/refyne/v2/storage"
	"github.com/alexballas/refyne/v2/theme"
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
