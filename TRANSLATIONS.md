# xfilepicker Translation Keys

This extension uses standard Fyne localization keys where possible, along with custom keys for specific functionality. Use `fyne.io/fyne/v2/lang` to provide translations for these strings.

## Standard Fyne Keys (Internal)
These keys are typically handled by the Fyne toolkit if provided in your `translations/` directory.

- `Open`: The confirm button text.
- `Cancel`: The dismiss button text.
- `Search...`: Placeholder for the search input.
- `New Folder`: Title for the folder creation dialog.
- `Create Folder`: Confirm button for folder creation.
- `Name`: Label for the folder name input.
- `Show Hidden Files`: Checkbox label.
- `FFmpeg Path`: Settings section label.
- `ffmpeg path (default: ffmpeg)`: Placeholder for FFmpeg path input.

## Custom Extension Keys
These are specifically used by the xfilepicker extension.

- `Sort By`: Placeholder for the sort selection menu.
- `Name (A-Z)`: Sort order option.
- `Name (Z-A)`: Sort order option.
- `Size`: Sort order option.
- `Date`: Sort order option.
- `Home`: Sidebar location.
- `Computer`: Sidebar root location.
- `Desktop`: Sidebar location.
- `Documents`: Sidebar location.
- `Downloads`: Sidebar location.
- `Music`: Sidebar location.
- `Pictures`: Sidebar location.
- `Videos`: Sidebar location.
- `Movies`: Sidebar location (macOS).

## Implementation Details
Strings in the code are wrapped in `lang.L(string)` or `lang.X(id, string)`.
Example: `lang.L("Sort By")`
