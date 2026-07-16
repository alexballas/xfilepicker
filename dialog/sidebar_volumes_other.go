//go:build !windows && !android && !ios && !wasm && !js && !darwin && !linux

package dialog

func externalVolumePaths() []string {
	return nil
}
