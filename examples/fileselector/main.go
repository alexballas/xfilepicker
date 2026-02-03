package main

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"

	"github.com/alexballas/xfilepicker/dialog"
)

func main() {
	a := app.NewWithID("com.example.multiselect")
	w := a.NewWindow("Multi-Select File Picker Example")

	// Example: Set custom FFmpeg path programmatically
	// This will also be saved to Fyne preferences automatically
	dialog.GetThumbnailManager().SetFFmpegPath("ffmpeg")

	label := widget.NewLabel("No files selected")

	btnMulti := widget.NewButton("Open Multiple Files", func() {
		dialog.ShowFileOpen(func(readers []fyne.URIReadCloser, err error) {
			if err != nil {
				label.SetText("Error: " + err.Error())
				return
			}
			if readers == nil {
				label.SetText("Cancelled")
				return
			}

			var names []string
			for _, r := range readers {
				names = append(names, r.URI().Name())
				r.Close()
			}
			label.SetText("Selected (Multi): " + strings.Join(names, ", "))
			fmt.Println("Selected:", names)
		}, w, true)
	})

	btnSingle := widget.NewButton("Open Single File", func() {
		dialog.ShowFileOpen(func(readers []fyne.URIReadCloser, err error) {
			if err != nil {
				label.SetText("Error: " + err.Error())
				return
			}
			if readers == nil {
				label.SetText("Cancelled")
				return
			}

			if len(readers) > 0 {
				label.SetText("Selected (Single): " + readers[0].URI().Name())
				readers[0].Close()
			}
		}, w, false)
	})

	btnSRT := widget.NewButton("Open SRT Files Only", func() {
		// NewFileOpen gives us the object, so we can call SetFilter before Show
		d := dialog.NewFileOpen(func(readers []fyne.URIReadCloser, err error) {
			if err != nil {
				label.SetText("Error: " + err.Error())
				return
			}
			if readers == nil {
				label.SetText("Cancelled")
				return
			}

			if len(readers) > 0 {
				label.SetText("Selected (SRT): " + readers[0].URI().Name())
				readers[0].Close()
			}
		}, w, false) // Single select

		// Since NewFileOpen returns dyne.Dialog, we need to cast to access SetFilter
		if f, ok := d.(interface{ SetFilter(storage.FileFilter) }); ok {
			f.SetFilter(storage.NewExtensionFileFilter([]string{".srt"}))
		}
		d.Show()
	})

	w.SetContent(container.NewVBox(
		widget.NewLabel("Click the buttons below to test different modes."),
		btnMulti,
		btnSingle,
		btnSRT,
		label,
	))

	w.Resize(fyne.NewSize(400, 300))
	w.ShowAndRun()
}
