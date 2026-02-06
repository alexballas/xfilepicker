package main

import (
	"fmt"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"

	"github.com/alexballas/xfilepicker/dialog"
)

func main() {
	a := app.NewWithID("com.example.filesave")
	w := a.NewWindow("File Save Dialog Example")

	label := widget.NewLabel("No file saved yet")
	dialog.SetFFmpegPath("ffmpeg")

	saveFile := func(prefix string, writer fyne.URIWriteCloser, err error) {
		if err != nil {
			label.SetText(prefix + " Error: " + err.Error())
			return
		}
		if writer == nil {
			label.SetText(prefix + " Cancelled")
			return
		}

		content := []byte("Saved from xfilepicker example.\n")
		if _, writeErr := writer.Write(content); writeErr != nil {
			label.SetText(prefix + " Write Error: " + writeErr.Error())
			_ = writer.Close()
			return
		}

		uri := writer.URI()
		_ = writer.Close()
		label.SetText(prefix + " Saved to: " + uri.Path())
		fmt.Println(prefix, uri.Path())
	}

	btnShowSave := widget.NewButton("ShowFileSave", func() {
		dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
			saveFile("ShowFileSave:", writer, err)
		}, w)
	})

	btnNewSave := widget.NewButton("NewFileSave (suggested name + cwd)", func() {
		d := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
			saveFile("NewFileSave:", writer, err)
		}, w)

		if setter, ok := d.(interface{ SetFileName(string) }); ok {
			setter.SetFileName("output.txt")
		}
		if loc, ok := d.(interface{ SetLocation(fyne.ListableURI) }); ok {
			cwd, err := os.Getwd()
			if err == nil {
				if lister, lErr := storage.ListerForURI(storage.NewFileURI(cwd)); lErr == nil {
					loc.SetLocation(lister)
				}
			}
		}

		d.Show()
	})

	btnSaveNested := widget.NewButton("Save In Nested Path", func() {
		d := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
			saveFile("Nested Save:", writer, err)
		}, w)

		if setter, ok := d.(interface{ SetFileName(string) }); ok {
			setter.SetFileName(filepath.Join("exports", "report.txt"))
		}

		d.Show()
	})

	w.SetContent(container.NewVBox(
		widget.NewLabel("File save dialog examples:"),
		btnShowSave,
		btnNewSave,
		btnSaveNested,
		label,
	))

	w.Resize(fyne.NewSize(640, 280))
	w.ShowAndRun()
}
