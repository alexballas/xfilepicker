package dialog

import (
	"bytes"
	"errors"
	"image"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cabbagekobe/tunetag"
)

const (
	maxAudioArtworkInputSize = 20 << 20
	maxAudioArtworkPixels    = 40_000_000
)

var (
	audioArtworkExtensions = []string{".jpg", ".jpeg", ".png"}
	audioArtworkSidecars   = []string{
		"cover",
		"folder",
		"front",
		"albumart",
		"album",
		"artwork",
		"albumartlarge",
		"albumartsmall",
		"thumb",
	}
)

type audioArtworkCandidate struct {
	name      string
	path      string
	base      string
	extension string
}

func loadAudioArtwork(path string) (image.Image, error) {
	candidates, err := scanAudioArtwork(filepath.Dir(path))
	if err != nil {
		return nil, err
	}

	trackStem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if artwork := bestAudioArtworkSidecar(candidates, trackStem); artwork != nil {
		return artwork, nil
	}

	if supportsEmbeddedAudioArtwork(path) {
		if artwork := loadEmbeddedAudioArtwork(path); artwork != nil {
			return artwork, nil
		}
	}

	for _, base := range audioArtworkSidecars {
		if artwork := bestAudioArtworkSidecar(candidates, base); artwork != nil {
			return artwork, nil
		}
	}

	for _, size := range []string{"large", "small"} {
		for _, base := range windowsAudioArtworkBases(candidates, size) {
			if artwork := bestAudioArtworkSidecar(candidates, base); artwork != nil {
				return artwork, nil
			}
		}
	}

	return nil, errors.New("audio artwork not found")
}

func scanAudioArtwork(directory string) ([]audioArtworkCandidate, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}

	candidates := make([]audioArtworkCandidate, 0, len(entries))
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		name := entry.Name()
		extension := strings.ToLower(filepath.Ext(name))
		if !isAudioArtworkExtension(extension) {
			continue
		}
		candidates = append(candidates, audioArtworkCandidate{
			name:      name,
			path:      filepath.Join(directory, name),
			base:      strings.TrimSuffix(name, filepath.Ext(name)),
			extension: extension,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		left := strings.ToLower(candidates[i].name)
		right := strings.ToLower(candidates[j].name)
		if left == right {
			return candidates[i].name < candidates[j].name
		}
		return left < right
	})
	return candidates, nil
}

func bestAudioArtworkSidecar(candidates []audioArtworkCandidate, base string) image.Image {
	var (
		best          image.Image
		bestArea      uint64
		bestExtension = len(audioArtworkExtensions)
	)

	for _, candidate := range candidates {
		if !strings.EqualFold(candidate.base, base) {
			continue
		}
		artwork, area, err := decodeAudioArtworkFile(candidate.path)
		if err != nil {
			continue
		}
		extension := audioArtworkExtensionRank(candidate.extension)
		if best == nil || area > bestArea || area == bestArea && extension < bestExtension {
			best = artwork
			bestArea = area
			bestExtension = extension
		}
	}
	return best
}

func decodeAudioArtworkFile(path string) (image.Image, uint64, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxAudioArtworkInputSize+1))
	if err != nil {
		return nil, 0, err
	}
	if len(data) > maxAudioArtworkInputSize {
		return nil, 0, errors.New("audio artwork exceeds 20 MiB")
	}
	return decodeAudioArtwork(data)
}

func decodeAudioArtwork(data []byte) (image.Image, uint64, error) {
	if len(data) == 0 {
		return nil, 0, errors.New("audio artwork is empty")
	}
	if len(data) > maxAudioArtworkInputSize {
		return nil, 0, errors.New("audio artwork exceeds 20 MiB")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, 0, err
	}
	if format != "jpeg" && format != "png" {
		return nil, 0, errors.New("unsupported audio artwork")
	}
	if config.Width <= 0 || config.Height <= 0 {
		return nil, 0, errors.New("invalid audio artwork dimensions")
	}
	area := uint64(config.Width) * uint64(config.Height)
	if area > maxAudioArtworkPixels {
		return nil, 0, errors.New("audio artwork exceeds 40 million pixels")
	}

	artwork, decodedFormat, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, err
	}
	if decodedFormat != format {
		return nil, 0, errors.New("audio artwork format changed while decoding")
	}
	return artwork, area, nil
}

func loadEmbeddedAudioArtwork(path string) image.Image {
	tag, err := tunetag.Open(path)
	if err != nil {
		return nil
	}
	pictures := tag.Pictures()
	for _, picture := range pictures {
		if picture.Type != tunetag.PictureCoverFront {
			continue
		}
		if artwork, _, err := decodeAudioArtwork(picture.Data); err == nil {
			return artwork
		}
	}
	for _, picture := range pictures {
		if picture.Type == tunetag.PictureCoverFront {
			continue
		}
		if artwork, _, err := decodeAudioArtwork(picture.Data); err == nil {
			return artwork
		}
	}
	return nil
}

func isAudioArtworkExtension(extension string) bool {
	for _, supported := range audioArtworkExtensions {
		if extension == supported {
			return true
		}
	}
	return false
}

func audioArtworkExtensionRank(extension string) int {
	for i, supported := range audioArtworkExtensions {
		if extension == supported {
			return i
		}
	}
	return len(audioArtworkExtensions)
}

func supportsEmbeddedAudioArtwork(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3", ".m4a", ".mp4", ".flac", ".ogg", ".oga", ".opus":
		return true
	default:
		return false
	}
}

func windowsAudioArtworkBases(candidates []audioArtworkCandidate, size string) []string {
	var bases []string
	for _, candidate := range candidates {
		lower := strings.ToLower(candidate.base)
		prefix := "albumart_"
		suffix := "_" + size
		if !strings.HasPrefix(lower, prefix) || !strings.HasSuffix(lower, suffix) || len(lower) == len(prefix)+len(suffix) {
			continue
		}
		if !containsAudioArtworkBase(bases, candidate.base) {
			bases = append(bases, candidate.base)
		}
	}
	return bases
}

func containsAudioArtworkBase(bases []string, candidate string) bool {
	for _, base := range bases {
		if strings.EqualFold(base, candidate) {
			return true
		}
	}
	return false
}
