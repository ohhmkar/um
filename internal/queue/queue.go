// Package queue implements a playback queue with shuffle and repeat
// modes. It sits between the library (which provides tracks) and the
// audio.Player (which plays them one at a time). The TUI drives the
// queue; the queue itself is a pure data structure with no goroutines.
package queue

import (
	"math/rand"

	"github.com/ohhmkar/um/internal/audio"
)

// RepeatMode controls what happens after the last track finishes.
type RepeatMode int

const (
	// RepeatOff stops after the last track.
	RepeatOff RepeatMode = iota
	// RepeatAll loops back to the first track.
	RepeatAll
	// RepeatOne replays the current track indefinitely.
	RepeatOne
)

// String implements fmt.Stringer for RepeatMode.
func (r RepeatMode) String() string {
	switch r {
	case RepeatOff:
		return "off"
	case RepeatAll:
		return "all"
	case RepeatOne:
		return "one"
	default:
		return "unknown"
	}
}

// Queue manages an ordered list of tracks and a cursor pointing to the
// current track. It supports shuffle and repeat modes.
type Queue struct {
	tracks  []audio.Track
	order   []int // playback order indices (identity when not shuffled)
	cursor  int   // position within `order`
	shuffle bool
	repeat  RepeatMode
}

// New creates an empty Queue.
func New() *Queue {
	return &Queue{
		repeat: RepeatOff,
	}
}

// ---------------------------------------------------------------------------
// Loading tracks
// ---------------------------------------------------------------------------

// Set replaces the queue contents with the given tracks and resets the
// cursor to the beginning. If shuffle is active, the order is randomised.
func (q *Queue) Set(tracks []audio.Track) {
	q.tracks = make([]audio.Track, len(tracks))
	copy(q.tracks, tracks)
	q.cursor = 0
	q.rebuildOrder()
}

// Append adds tracks to the end of the queue without resetting the
// cursor position.
func (q *Queue) Append(tracks ...audio.Track) {
	start := len(q.tracks)
	q.tracks = append(q.tracks, tracks...)
	for i := start; i < len(q.tracks); i++ {
		q.order = append(q.order, i)
	}
	// If shuffled, shuffle only the appended portion to avoid
	// disturbing already-played ordering.
	if q.shuffle && len(q.order) > start {
		tail := q.order[q.cursor+1:]
		rand.Shuffle(len(tail), func(i, j int) {
			tail[i], tail[j] = tail[j], tail[i]
		})
	}
}

// Clear empties the queue.
func (q *Queue) Clear() {
	q.tracks = nil
	q.order = nil
	q.cursor = 0
}

// ---------------------------------------------------------------------------
// Navigation
// ---------------------------------------------------------------------------

// Current returns the track at the cursor, or (Track{}, false) if the
// queue is empty.
func (q *Queue) Current() (audio.Track, bool) {
	if len(q.order) == 0 || q.cursor < 0 || q.cursor >= len(q.order) {
		return audio.Track{}, false
	}
	return q.tracks[q.order[q.cursor]], true
}

// Next advances the cursor and returns the next track. Returns false
// when there is no next track (respects RepeatMode).
func (q *Queue) Next() (audio.Track, bool) {
	if len(q.order) == 0 {
		return audio.Track{}, false
	}

	switch q.repeat {
	case RepeatOne:
		// Stay on the same track.
		return q.tracks[q.order[q.cursor]], true

	case RepeatAll:
		q.cursor++
		if q.cursor >= len(q.order) {
			q.cursor = 0
			if q.shuffle {
				q.rebuildOrder()
			}
		}
		return q.tracks[q.order[q.cursor]], true

	default: // RepeatOff
		q.cursor++
		if q.cursor >= len(q.order) {
			q.cursor = len(q.order) - 1 // clamp
			return audio.Track{}, false
		}
		return q.tracks[q.order[q.cursor]], true
	}
}

// Previous moves the cursor back and returns the previous track.
// Returns false if already at the beginning (and repeat is off/one).
func (q *Queue) Previous() (audio.Track, bool) {
	if len(q.order) == 0 {
		return audio.Track{}, false
	}

	q.cursor--
	if q.cursor < 0 {
		if q.repeat == RepeatAll {
			q.cursor = len(q.order) - 1
		} else {
			q.cursor = 0
			return q.tracks[q.order[0]], true
		}
	}
	return q.tracks[q.order[q.cursor]], true
}

// JumpTo sets the cursor to the given index within the original track
// list (not the shuffled order). Returns false if idx is out of range.
func (q *Queue) JumpTo(idx int) (audio.Track, bool) {
	if idx < 0 || idx >= len(q.tracks) {
		return audio.Track{}, false
	}
	// Find the position in the current order.
	for i, oi := range q.order {
		if oi == idx {
			q.cursor = i
			return q.tracks[idx], true
		}
	}
	return audio.Track{}, false
}

// ---------------------------------------------------------------------------
// Modes
// ---------------------------------------------------------------------------

// SetShuffle toggles shuffle on or off. Turning shuffle on randomises
// the remaining (unplayed) portion of the queue. Turning it off restores
// sequential order from the current position.
func (q *Queue) SetShuffle(on bool) {
	if q.shuffle == on {
		return
	}
	q.shuffle = on

	if len(q.order) == 0 {
		return
	}

	currentTrackIdx := q.order[q.cursor]

	if on {
		q.rebuildOrder()
	} else {
		// Restore sequential order.
		q.order = makeIdentityOrder(len(q.tracks))
	}

	// Maintain cursor on the same track.
	for i, oi := range q.order {
		if oi == currentTrackIdx {
			q.cursor = i
			break
		}
	}
}

// ToggleShuffle flips the shuffle state.
func (q *Queue) ToggleShuffle() {
	q.SetShuffle(!q.shuffle)
}

// Shuffle reports whether shuffle is active.
func (q *Queue) Shuffle() bool {
	return q.shuffle
}

// CycleRepeat cycles through RepeatOff → RepeatAll → RepeatOne → RepeatOff.
func (q *Queue) CycleRepeat() RepeatMode {
	q.repeat = (q.repeat + 1) % 3
	return q.repeat
}

// Repeat returns the current repeat mode.
func (q *Queue) Repeat() RepeatMode {
	return q.repeat
}

// SetRepeat sets the repeat mode directly.
func (q *Queue) SetRepeat(mode RepeatMode) {
	q.repeat = mode
}

// ---------------------------------------------------------------------------
// Query
// ---------------------------------------------------------------------------

// Len returns the total number of tracks in the queue.
func (q *Queue) Len() int {
	return len(q.tracks)
}

// IsEmpty reports whether the queue has no tracks.
func (q *Queue) IsEmpty() bool {
	return len(q.tracks) == 0
}

// Tracks returns a copy of the underlying track list in its original
// (insertion) order.
func (q *Queue) Tracks() []audio.Track {
	out := make([]audio.Track, len(q.tracks))
	copy(out, q.tracks)
	return out
}

// CursorIndex returns the current cursor position within the playback
// order.
func (q *Queue) CursorIndex() int {
	return q.cursor
}

// PlayOrder returns all tracks in their current playback order
// (shuffled or sequential). The slice is a fresh copy.
func (q *Queue) PlayOrder() []audio.Track {
	out := make([]audio.Track, len(q.order))
	for i, oi := range q.order {
		out[i] = q.tracks[oi]
	}
	return out
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// rebuildOrder creates a fresh playback order. If shuffle is on, the
// order is randomised; otherwise it is sequential.
func (q *Queue) rebuildOrder() {
	q.order = makeIdentityOrder(len(q.tracks))
	if q.shuffle {
		rand.Shuffle(len(q.order), func(i, j int) {
			q.order[i], q.order[j] = q.order[j], q.order[i]
		})
	}
}

// makeIdentityOrder returns [0, 1, 2, ..., n-1].
func makeIdentityOrder(n int) []int {
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	return order
}
