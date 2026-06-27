package audio

import "time"

// ---------------------------------------------------------------------------
// Messages (tea.Msg types)
//
// These are sent from the audio goroutine back to the Bubble Tea Update loop
// via the Player's message channel. The TUI must never modify the Player
// directly; it issues commands, and the Player responds with messages.
// ---------------------------------------------------------------------------

// PlaybackStartedMsg is emitted when a track begins playing.
type PlaybackStartedMsg struct {
	Track    Track
	Duration time.Duration
}

// PlaybackPausedMsg is emitted when the player pauses.
type PlaybackPausedMsg struct{}

// PlaybackResumedMsg is emitted when the player resumes from a paused state.
type PlaybackResumedMsg struct{}

// PlaybackStoppedMsg is emitted when the player stops (no track loaded).
type PlaybackStoppedMsg struct{}

// PlaybackProgressMsg is emitted periodically while a track is playing.
// It carries the current position so the TUI can update the progress bar.
type PlaybackProgressMsg struct {
	Position time.Duration
	Duration time.Duration
}

// TrackFinishedMsg is emitted when the current track reaches its end
// naturally (i.e. not stopped by the user). The TUI should use this to
// advance to the next track in the queue.
type TrackFinishedMsg struct{}

// VolumeChangedMsg is emitted after a volume adjustment.
type VolumeChangedMsg struct {
	// Volume in the range [0.0, 1.0].
	Volume float64
}

// SeekCompleteMsg is emitted after a seek operation finishes.
type SeekCompleteMsg struct {
	Position time.Duration
}

// ErrMsg wraps any error originating from the audio subsystem. The TUI
// should display the error non-intrusively and continue operating.
type ErrMsg struct {
	Err error
}

// Error implements the error interface so ErrMsg can be used ergonomically.
func (e ErrMsg) Error() string { return e.Err.Error() }
