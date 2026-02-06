//go:build android || ios

package dialog

import (
	"fyne.io/fyne/v2"
	fynedialog "fyne.io/fyne/v2/dialog"
)

func fileOpenOSOverride(f *fileDialog) bool {
	if f.isFolderMode() {
		d := fynedialog.NewFolderOpen(func(dir fyne.ListableURI, err error) {
			fyne.Do(func() {
				if f.folderCallback != nil {
					f.folderCallback(dir, err)
				}
			})
		}, f.parent)
		d.Show()
		return true
	}

	d := fynedialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		var readers []fyne.URIReadCloser
		if reader != nil {
			readers = []fyne.URIReadCloser{reader}
		}
		fyne.Do(func() {
			if f.callback != nil {
				f.callback(readers, err)
			}
		})
	}, f.parent)
	if f.extensionFilter != nil {
		d.SetFilter(f.extensionFilter)
	}
	d.Show()
	return true
}
