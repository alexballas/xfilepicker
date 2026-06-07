package dialog

import (
	"github.com/alexballas/refyne/v2"
	"github.com/alexballas/refyne/v2/widget"
)

// typeSearchEntry is the dialog's type-to-search box. It behaves like a normal
// Entry except that Escape cancels an in-progress search: it clears the text
// (resetting the file filter) and then runs onEscape so the dialog can release
// focus, mirroring file managers like Nautilus. Without this, Escape falls
// through to Entry's no-op default, leaving the filter applied and the box still
// focused.
type typeSearchEntry struct {
	widget.Entry

	// onEscape is invoked after the text has been cleared in response to Escape,
	// letting the dialog move focus away from the search box.
	onEscape func()
}

func newTypeSearchEntry() *typeSearchEntry {
	e := &typeSearchEntry{}
	e.ExtendBaseWidget(e)
	return e
}

// TypedKey intercepts Escape to cancel the search and otherwise defers to the
// embedded Entry. SetText("") fires OnChanged, which resets the filter back to
// the full listing.
func (e *typeSearchEntry) TypedKey(key *fyne.KeyEvent) {
	if key.Name == fyne.KeyEscape {
		e.SetText("")
		if e.onEscape != nil {
			e.onEscape()
		}
		return
	}
	e.Entry.TypedKey(key)
}
