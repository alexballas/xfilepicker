package main

import (
	"fmt"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"

	"github.com/alexballas/xfilepicker/dialog"
)

func main() {
	a := app.NewWithID("com.example.folderselector")
	w := a.NewWindow("Folder Picker Example")

	label := widget.NewLabel("No folder selected")

	updateLabel := func(prefix string, dir fyne.ListableURI, err error) {
		if err != nil {
			label.SetText(prefix + " Error: " + err.Error())
			return
		}
		if dir == nil {
			label.SetText(prefix + " Cancelled")
			return
		}

		label.SetText(prefix + " " + dir.Path())
		fmt.Println(prefix, dir.Path())
	}

	btnShowFolderOpen := widget.NewButton("ShowFolderOpen", func() {
		dialog.ShowFolderOpen(func(dir fyne.ListableURI, err error) {
			updateLabel("ShowFolderOpen:", dir, err)
		}, w)
	})

	btnNewFolderOpen := widget.NewButton("NewFolderOpen (start at cwd)", func() {
		d := dialog.NewFolderOpen(func(dir fyne.ListableURI, err error) {
			updateLabel("NewFolderOpen:", dir, err)
		}, w)

		// Optional: set initial location for this picker.
		if loc, ok := d.(interface{ SetLocation(fyne.ListableURI) }); ok {
			cwd, err := os.Getwd()
			if err == nil {
				if lister, listerErr := storage.ListerForURI(storage.NewFileURI(cwd)); listerErr == nil {
					loc.SetLocation(lister)
				}
			}
		}

		d.Show()
	})

	w.SetContent(container.NewVBox(
		widget.NewLabel("Folder picker examples:"),
		btnShowFolderOpen,
		btnNewFolderOpen,
		label,
	))

	w.Resize(fyne.NewSize(560, 240))
	w.ShowAndRun()
}
