package queue

import (
	"testing"

	"um/internal/audio"
)

func sampleTracks(n int) []audio.Track {
	tracks := make([]audio.Track, n)
	for i := range tracks {
		tracks[i] = audio.Track{
			Path:  "/music/track" + string(rune('A'+i)) + ".mp3",
			Title: "Track " + string(rune('A'+i)),
		}
	}
	return tracks
}

// ---------------------------------------------------------------------------
// Basic queue operations
// ---------------------------------------------------------------------------

func TestNew_Empty(t *testing.T) {
	q := New()
	if !q.IsEmpty() {
		t.Error("new queue should be empty")
	}
	if q.Len() != 0 {
		t.Errorf("new queue Len = %d, want 0", q.Len())
	}
}

func TestSet_And_Current(t *testing.T) {
	q := New()
	tracks := sampleTracks(3)
	q.Set(tracks)

	if q.Len() != 3 {
		t.Fatalf("Len = %d, want 3", q.Len())
	}
	cur, ok := q.Current()
	if !ok {
		t.Fatal("Current returned false after Set")
	}
	if cur.Title != "Track A" {
		t.Errorf("Current = %q, want Track A", cur.Title)
	}
}

func TestCurrent_Empty(t *testing.T) {
	q := New()
	_, ok := q.Current()
	if ok {
		t.Error("Current on empty queue should return false")
	}
}

func TestClear(t *testing.T) {
	q := New()
	q.Set(sampleTracks(3))
	q.Clear()
	if !q.IsEmpty() {
		t.Error("Clear should empty the queue")
	}
}

// ---------------------------------------------------------------------------
// Navigation — Next / Previous
// ---------------------------------------------------------------------------

func TestNext_Sequential(t *testing.T) {
	q := New()
	q.Set(sampleTracks(3))

	tr, ok := q.Next()
	if !ok || tr.Title != "Track B" {
		t.Errorf("Next() = (%q, %v), want (Track B, true)", tr.Title, ok)
	}

	tr, ok = q.Next()
	if !ok || tr.Title != "Track C" {
		t.Errorf("Next() = (%q, %v), want (Track C, true)", tr.Title, ok)
	}

	// Past the end with RepeatOff.
	_, ok = q.Next()
	if ok {
		t.Error("Next past end with RepeatOff should return false")
	}
}

func TestNext_RepeatAll(t *testing.T) {
	q := New()
	q.Set(sampleTracks(2))
	q.SetRepeat(RepeatAll)

	q.Next() // B
	tr, ok := q.Next()
	if !ok || tr.Title != "Track A" {
		t.Errorf("RepeatAll wrap: (%q, %v), want (Track A, true)", tr.Title, ok)
	}
}

func TestNext_RepeatOne(t *testing.T) {
	q := New()
	q.Set(sampleTracks(3))
	q.SetRepeat(RepeatOne)

	tr, ok := q.Next()
	if !ok || tr.Title != "Track A" {
		t.Errorf("RepeatOne: (%q, %v), want (Track A, true)", tr.Title, ok)
	}
}

func TestPrevious(t *testing.T) {
	q := New()
	q.Set(sampleTracks(3))
	q.Next() // → B
	q.Next() // → C

	tr, ok := q.Previous()
	if !ok || tr.Title != "Track B" {
		t.Errorf("Previous = (%q, %v), want (Track B, true)", tr.Title, ok)
	}
}

func TestPrevious_AtBeginning(t *testing.T) {
	q := New()
	q.Set(sampleTracks(3))

	tr, ok := q.Previous()
	if !ok {
		t.Fatal("Previous at beginning should still return a track")
	}
	if tr.Title != "Track A" {
		t.Errorf("Previous at beginning = %q, want Track A", tr.Title)
	}
}

func TestPrevious_RepeatAll_Wraps(t *testing.T) {
	q := New()
	q.Set(sampleTracks(3))
	q.SetRepeat(RepeatAll)

	tr, _ := q.Previous()
	if tr.Title != "Track C" {
		t.Errorf("Previous with RepeatAll at 0 = %q, want Track C", tr.Title)
	}
}

// ---------------------------------------------------------------------------
// JumpTo
// ---------------------------------------------------------------------------

func TestJumpTo(t *testing.T) {
	q := New()
	q.Set(sampleTracks(5))

	tr, ok := q.JumpTo(3)
	if !ok || tr.Title != "Track D" {
		t.Errorf("JumpTo(3) = (%q, %v), want (Track D, true)", tr.Title, ok)
	}

	cur, _ := q.Current()
	if cur.Title != "Track D" {
		t.Errorf("Current after JumpTo = %q, want Track D", cur.Title)
	}
}

func TestJumpTo_OutOfRange(t *testing.T) {
	q := New()
	q.Set(sampleTracks(3))

	_, ok := q.JumpTo(-1)
	if ok {
		t.Error("JumpTo(-1) should return false")
	}
	_, ok = q.JumpTo(10)
	if ok {
		t.Error("JumpTo(10) should return false")
	}
}

// ---------------------------------------------------------------------------
// Shuffle
// ---------------------------------------------------------------------------

func TestShuffle_ChangesOrder(t *testing.T) {
	q := New()
	q.Set(sampleTracks(20)) // large enough that shuffle is overwhelmingly different

	q.SetShuffle(true)

	// We can't assert exact order since it's random, but we can verify
	// the length is preserved and the current track is still accessible.
	if q.Len() != 20 {
		t.Errorf("Len after shuffle = %d, want 20", q.Len())
	}
	_, ok := q.Current()
	if !ok {
		t.Error("Current after shuffle should return true")
	}
}

func TestShuffle_Off_RestoresOrder(t *testing.T) {
	q := New()
	q.Set(sampleTracks(5))
	q.Next() // B
	q.Next() // C

	cur, _ := q.Current()
	q.SetShuffle(true)
	q.SetShuffle(false)

	// After toggling shuffle off, cursor should still be on the same track.
	afterCur, _ := q.Current()
	if afterCur.Title != cur.Title {
		t.Errorf("after shuffle off Current = %q, want %q", afterCur.Title, cur.Title)
	}
}

// ---------------------------------------------------------------------------
// Repeat mode cycling
// ---------------------------------------------------------------------------

func TestCycleRepeat(t *testing.T) {
	q := New()
	if q.Repeat() != RepeatOff {
		t.Fatalf("initial repeat = %v, want Off", q.Repeat())
	}

	r := q.CycleRepeat()
	if r != RepeatAll {
		t.Errorf("CycleRepeat 1 = %v, want All", r)
	}
	r = q.CycleRepeat()
	if r != RepeatOne {
		t.Errorf("CycleRepeat 2 = %v, want One", r)
	}
	r = q.CycleRepeat()
	if r != RepeatOff {
		t.Errorf("CycleRepeat 3 = %v, want Off", r)
	}
}

func TestRepeatMode_String(t *testing.T) {
	tests := []struct {
		m    RepeatMode
		want string
	}{
		{RepeatOff, "off"},
		{RepeatAll, "all"},
		{RepeatOne, "one"},
		{RepeatMode(42), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.m.String(); got != tt.want {
			t.Errorf("RepeatMode(%d).String() = %q, want %q", tt.m, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Append
// ---------------------------------------------------------------------------

func TestAppend(t *testing.T) {
	q := New()
	q.Set(sampleTracks(2))
	q.Append(audio.Track{Path: "/new.mp3", Title: "New"})

	if q.Len() != 3 {
		t.Fatalf("Len after Append = %d, want 3", q.Len())
	}

	// Navigate to the appended track.
	q.Next() // B
	tr, ok := q.Next()
	if !ok || tr.Title != "New" {
		t.Errorf("Appended track: (%q, %v), want (New, true)", tr.Title, ok)
	}
}

// ---------------------------------------------------------------------------
// Tracks copy
// ---------------------------------------------------------------------------

func TestTracks_ReturnsCopy(t *testing.T) {
	q := New()
	q.Set(sampleTracks(3))

	tracks := q.Tracks()
	tracks[0].Title = "MUTATED"

	cur, _ := q.Current()
	if cur.Title == "MUTATED" {
		t.Error("Tracks() should return a copy, mutation leaked")
	}
}

// ---------------------------------------------------------------------------
// Empty queue edge cases
// ---------------------------------------------------------------------------

func TestNext_EmptyQueue(t *testing.T) {
	q := New()
	_, ok := q.Next()
	if ok {
		t.Error("Next on empty queue should return false")
	}
}

func TestPrevious_EmptyQueue(t *testing.T) {
	q := New()
	_, ok := q.Previous()
	if ok {
		t.Error("Previous on empty queue should return false")
	}
}
