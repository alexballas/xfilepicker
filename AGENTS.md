# AGENTS.md

## Project Overview
`xfilepicker` is an enhanced file selection dialog for the [Fyne](https://fyne.io) toolkit. It acts as a drop-in replacement or upgrade for the standard Fyne file dialog, focusing on performance, media handling (thumbnails), and advanced user interaction (multi-select, keyboard navigation).

## Key Architecture

### Core Logic (`dialog/`)
The primary logic resides in the `dialog` package.
- **`file_list.go`**: Manages the grid and list views, handling user input, selection logic (including Shift/Ctrl modifiers), and rendering of file items.
- **`thumbnail_manager.go`**: Handles asynchronous generation and caching of thumbnails. It uses a 2-layer cache: in-memory (LRU) and disk-based.
- **`file.go` / `base.go`**: (Verify exact filename) Base structures for the file dialog.

### Caching Strategy
- **Disk Cache**: Located in `os.UserCacheDir()`. Uses SHA256 hashing of file path + mod time + partal content.
- **Memory Cache**: "Zero-delay" rendering for recently viewed items.

### Features to Maintain
- **Multi-select**: Critical feature. Logic tracked in `dialog/multiselect.go`.
- **Right-Click Context Menu**: Primary interaction for advanced actions. The "3 dots" button has been removed in favor of native right-click behavior.
- **Resize Handling**: Popups are automatically dismissed on resize. This is handled by a custom `resizeLayout` in `multiselect.go` which uses `fyne.Do` to safely defer UI updates.
- **FFmpeg Integration**: Optional dependency for video thumbnails. Code checks for availability.
- **Responsiveness**: Heavy operations (IO, thumb generation) *must* be on background goroutines. UI updates *must* happen on the main thread (`container.Refresh()`, `widget.Refresh()`).
- **Safety**: When updating UI from layout callbacks (like `Resize`), always defer execution using `fyne.Do()` to avoid re-entrant layout panics.

## Development Guidelines

### 1. UI/UX
- **Fyne Idioms**: Use standard Fyne widgets (`widget.Label`, `widget.Icon`) where possible, but `xfilepicker` often uses custom rendering for performance in lists/grids.
- **Theme**: Respect `theme.Current()` changes.

### 2. Testing
- Run tests in `dialog/` package.
  ```bash
  go test ./dialog/...
  ```
- Ensure no regressions in thumbnail generation or selection state.

### 3. Common Pitfalls
- **Deadlocks**: Watch out for mutexes in the thumbnail manager.
- **Race Conditions**: UI updates from background threads must use `driver.RunOnUIThread(...)` if not triggered by an event handler.

## Command Reference
- **Run Example**:
  ```bash
  go run examples/fileselector/main.go
  ```
