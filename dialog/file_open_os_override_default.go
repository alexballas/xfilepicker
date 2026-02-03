//go:build !flatpak && !android && !ios

package dialog

func fileOpenOSOverride(_ *fileDialog) bool {
	return false
}
