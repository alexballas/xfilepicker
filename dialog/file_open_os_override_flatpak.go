//go:build flatpak && !windows && !android && !ios && !wasm && !js

package dialog

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/storage"

	"github.com/rymdport/portal"
	"github.com/rymdport/portal/filechooser"
)

func fileOpenOSOverride(f *fileDialog) bool {
	if f.isSaveMode() {
		options := &filechooser.SaveFileOptions{
			AcceptLabel: lang.L("Save"),
		}
		if f.dir != nil {
			options.CurrentFolder = f.dir.Path()
		}
		if f.defaultSaveName != "" {
			options.CurrentName = f.defaultSaveName
		}
		options.Filters, options.CurrentFilter = convertFilterForPortal(f.extensionFilter)
		windowHandle := windowHandleForPortal(f.parent)

		go func() {
			uris, err := filechooser.SaveFile(windowHandle, lang.L("Save File"), options)
			if err != nil {
				fyne.Do(func() {
					if f.saveCallback != nil {
						f.saveCallback(nil, err)
					}
				})
				return
			}
			if len(uris) == 0 {
				fyne.Do(func() {
					if f.saveCallback != nil {
						f.saveCallback(nil, nil)
					}
				})
				return
			}

			uri, err := storage.ParseURI(uris[0])
			if err != nil {
				fyne.Do(func() {
					if f.saveCallback != nil {
						f.saveCallback(nil, err)
					}
				})
				return
			}

			writer, err := storage.Writer(uri)
			fyne.Do(func() {
				if f.saveCallback != nil {
					f.saveCallback(writer, err)
				}
			})
		}()
		return true
	}

	options := &filechooser.OpenFileOptions{
		AcceptLabel: lang.L("Open"),
		Multiple:    f.allowMultiple,
		Directory:   f.isFolderMode(),
	}
	if f.dir != nil {
		options.CurrentFolder = f.dir.Path()
	}

	options.Filters, options.CurrentFilter = convertFilterForPortal(f.extensionFilter)
	windowHandle := windowHandleForPortal(f.parent)

	go func() {
		titleNoun := lang.L("File")
		if f.isFolderMode() {
			titleNoun = lang.L("Folder")
		}
		title := lang.L("Open") + " " + titleNoun
		uris, err := filechooser.OpenFile(windowHandle, title, options)
		if err != nil {
			fyne.Do(func() {
				if f.isFolderMode() {
					if f.folderCallback != nil {
						f.folderCallback(nil, err)
					}
					return
				}
				if f.callback != nil {
					f.callback(nil, err)
				}
			})
			return
		}
		if len(uris) == 0 {
			fyne.Do(func() {
				if f.isFolderMode() {
					if f.folderCallback != nil {
						f.folderCallback(nil, nil)
					}
					return
				}
				if f.callback != nil {
					f.callback(nil, nil)
				}
			})
			return
		}

		if f.isFolderMode() {
			uri, parseErr := storage.ParseURI(uris[0])
			if parseErr != nil {
				err = parseErr
			}
			var dir fyne.ListableURI
			if err == nil {
				dir, err = storage.ListerForURI(uri)
			}
			fyne.Do(func() {
				if f.folderCallback != nil {
					f.folderCallback(dir, err)
				}
			})
			return
		}

		readers := make([]fyne.URIReadCloser, 0, len(uris))
		for _, raw := range uris {
			uri, parseErr := storage.ParseURI(raw)
			if parseErr != nil {
				err = parseErr
				break
			}
			r, openErr := storage.Reader(uri)
			if openErr != nil {
				err = openErr
				break
			}
			readers = append(readers, r)
		}

		if err != nil {
			for _, r := range readers {
				_ = r.Close()
			}
			readers = nil
		}

		fyne.Do(func() {
			if f.callback != nil {
				f.callback(readers, err)
			}
		})
	}()

	return true
}

func windowHandleForPortal(window fyne.Window) string {
	native, ok := window.(driver.NativeWindow)
	if !ok {
		return ""
	}

	windowHandle := ""
	native.RunNative(func(context any) {
		if x11, ok := context.(driver.X11WindowContext); ok {
			windowHandle = portal.FormatX11WindowHandle(x11.WindowHandle)
		}
	})
	return windowHandle
}

func convertFilterForPortal(fyneFilter storage.FileFilter) (list []*filechooser.Filter, current *filechooser.Filter) {
	if fyneFilter == nil {
		return nil, nil
	}

	if filter, ok := fyneFilter.(*storage.ExtensionFileFilter); ok {
		rules := make([]filechooser.Rule, 0, 2*len(filter.Extensions))
		for _, ext := range filter.Extensions {
			lowercase := filechooser.Rule{Type: filechooser.GlobPattern, Pattern: "*" + strings.ToLower(ext)}
			uppercase := filechooser.Rule{Type: filechooser.GlobPattern, Pattern: "*" + strings.ToUpper(ext)}
			rules = append(rules, lowercase, uppercase)
		}
		name := formatFilterName(filter.Extensions, 3)
		converted := &filechooser.Filter{Name: name, Rules: rules}
		return []*filechooser.Filter{converted}, converted
	}

	if filter, ok := fyneFilter.(*storage.MimeTypeFileFilter); ok {
		rules := make([]filechooser.Rule, len(filter.MimeTypes))
		for i, mime := range filter.MimeTypes {
			rules[i] = filechooser.Rule{Type: filechooser.MIMEType, Pattern: mime}
		}
		name := formatFilterName(filter.MimeTypes, 3)
		converted := &filechooser.Filter{Name: name, Rules: rules}
		return []*filechooser.Filter{converted}, converted
	}

	return nil, nil
}

func formatFilterName(patterns []string, count int) string {
	if len(patterns) < count {
		count = len(patterns)
	}

	name := strings.Join(patterns[:count], ", ")
	if len(patterns) > count {
		name += "…"
	}
	return name
}
