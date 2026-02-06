# xfilepicker

`xfilepicker` is an enhanced file selection dialog for [Fyne](https://fyne.io), designed to provide a more powerful and responsive experience than the default toolkit's file picker.

## Goals

While Fyne's default picker is excellent for general use, `xfilepicker` is an extended version designed for applications with specialized needs, particularly around high-performance media management and persistent caching.

## Key Features & Enhancements
*   **Complete Modifier Support**: Fully supports `Ctrl+Click` (additive), `Shift+Click` (range), and `Drag-Select`.
*   **Drag-and-Drop Selection**: Intuitive marquee selection. Click and drag on the background to draw a rectangle and select multiple files at once. Supports scrolling and precise intersection with grid items.
*   **Flexible API**: Easily toggle between single-file and multiple-file selection via a simple boolean flag.

### 2. Intelligent Thumbnail Management
*   **Persistent Disk Cache**: Thumbnails are cached on disk (`os.UserCacheDir()`) using SHA256 hashing of path, modification time, and partial file content. No more waiting for regeneration between app restarts.
*   **Instant Load Architecture**: Memory hits bypass the debounce timer for a "zero-delay" feel when scrolling.
*   **Background Pre-warming**: When you enter a folder, a background worker pre-loads thumbnails from disk into memory, making the first scroll feel polished and smooth.
*   **LRU Eviction**: Automatically manages disk space (soft limits of 500MB or 10,000 files), cleaning up old entries on startup.

### 3. Rich Media Support
*   **Video Previews**: Generates high-quality thumbnails for video files (`.mp4`, `.mkv`, `.avi`, `.webm`, `.mov`) using FFmpeg.
*   **Smart Aspect Ratio**: Thumbnails are resized and letterboxed to maintain their original aspect ratio within the grid.
*   **Configurable FFmpeg**: Set your FFmpeg path via the UI or programmatically.

### 4. Advanced UX & Design
*   **Type-to-Search**: Simply start typing anywhere in the dialog to instantly focus the search bar and filter results.
*   **Zoomable Thumbnails**: Use toolbar buttons or `Ctrl/Cmd + Scroll` to zoom the grid and make thumbnails more visible.
*   **Smart Truncation**: Filenames are intelligently truncated to a maximum of 3 lines in Grid View, ensuring the file extension is always visible.
*   **Search Relevance**: Search results are "Smart Sorted" to prioritize files starting with your query.
*   **Rich Folder Visuals**: Automatically uses correct icons for system folders (Desktop, Music, etc.) and supports custom folder covers (via `.background.png`) using `fancyfs`.
*   **Localized**: Fully internationalized with support for Fyne's `lang` package.
*   **Persistence**: Remembers your preferred view layout (Grid/List), zoom level, hidden file toggle, and FFmpeg path across sessions.

## Quick Start

```go
import "github.com/alexballas/xfilepicker/dialog"

// Open Multiple Files
dialog.ShowFileOpen(func(readers []fyne.URIReadCloser, err error) {
    if readers != nil {
        // Handle selection
    }
}, window, true)

// Open Single File
dialog.ShowFileOpen(func(readers []fyne.URIReadCloser, err error) {
    if readers != nil {
        // Handle selection
    }
}, window, false)

// Save File
dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
    if writer != nil {
        // Write bytes and close
        _, _ = writer.Write([]byte("hello"))
        _ = writer.Close()
    }
}, window)
```

Examples:
* `go run examples/fileselector/main.go`
* `go run examples/folderselector/main.go`
* `go run examples/filesave/main.go`

## Requirements
*   **FFmpeg** (Optional: required for video thumbnails).
