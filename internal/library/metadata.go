package library

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dhowden/tag"

	"github.com/ohhmkar/um/internal/audio"
)

// parseTrack opens a file, reads its ID3/Vorbis metadata, and returns
// a populated audio.Track. If tags are missing it gracefully falls back
// to deriving title from the filename.
func parseTrack(path string) (audio.Track, error) {
	f, err := os.Open(path)
	if err != nil {
		return audio.Track{}, fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()

	t := audio.Track{Path: path}

	// Attempt tag extraction — failure is non-fatal.
	m, tagErr := tag.ReadFrom(f)
	if tagErr != nil {
		// Fallback: derive a title from the filename.
		t.Title = titleFromFilename(path)
		return t, nil
	}

	t.Title = stringOrFallback(m.Title(), titleFromFilename(path))
	t.Artist = m.Artist()
	t.Album = m.Album()

	trackNum, _ := m.Track()
	t.TrackNumber = trackNum

	// dhowden/tag does not expose duration — that is obtained at decode
	// time by the audio.Player. Leave as zero.
	t.Duration = 0

	return t, nil
}

// durationFromSeconds is a helper to convert raw seconds to
// time.Duration (used by callers that may receive duration hints from
// external sources).
func durationFromSeconds(s int) time.Duration {
	return time.Duration(s) * time.Second
}

// titleFromFilename strips the directory and extension from path to
// produce a human-readable fallback title.
//
//	"/home/user/music/01 - Intro.mp3" → "01 - Intro"
func titleFromFilename(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

// stringOrFallback returns s if non-empty, otherwise fallback.
func stringOrFallback(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}
