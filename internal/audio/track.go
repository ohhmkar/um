// Package audio provides the concurrent audio playback engine for µm.
// It uses gopxl/beep for decoding and speaker output, and communicates
// state changes back to the TUI via tea.Msg values on a channel.
package audio

import "time"

// Track represents a single audio file with its associated metadata.
// The audio backend operates on Track values; it does not concern itself
// with how metadata was obtained (ID3 tags, Vorbis comments, filename
// fallback, etc.).
type Track struct {
	// Path is the absolute filesystem path to the audio file.
	Path string

	// Title of the track. May be empty if metadata is unavailable.
	Title string

	// Artist name. May be empty.
	Artist string

	// Album name. May be empty.
	Album string

	// TrackNumber within the album (0 if unknown).
	TrackNumber int

	// Duration of the track. Zero value means the duration is unknown
	// until decoding begins.
	Duration time.Duration
}

// DisplayTitle returns a human-friendly title, falling back to the file
// path basename when no Title metadata is set.
func (t Track) DisplayTitle() string {
	if t.Title != "" {
		return t.Title
	}
	// Fallback: extract filename from path.
	for i := len(t.Path) - 1; i >= 0; i-- {
		if t.Path[i] == '/' || t.Path[i] == '\\' {
			return t.Path[i+1:]
		}
	}
	return t.Path
}
