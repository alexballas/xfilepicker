//go:build android || ios

package dialog

func (s *sidebar) getPlaces() []favoriteItem {
	return []favoriteItem{}
}
