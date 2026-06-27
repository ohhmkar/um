package audio

import (
	"fmt"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Track tests
// ---------------------------------------------------------------------------

func TestTrack_DisplayTitle_WithTitle(t *testing.T) {
	tr := Track{Path: "/music/song.mp3", Title: "Bohemian Rhapsody"}
	got := tr.DisplayTitle()
	want := "Bohemian Rhapsody"
	if got != want {
		t.Errorf("DisplayTitle() = %q, want %q", got, want)
	}
}

func TestTrack_DisplayTitle_FallbackToFilename(t *testing.T) {
	tr := Track{Path: "/home/user/music/song.mp3"}
	got := tr.DisplayTitle()
	want := "song.mp3"
	if got != want {
		t.Errorf("DisplayTitle() = %q, want %q", got, want)
	}
}

func TestTrack_DisplayTitle_NoSlash(t *testing.T) {
	tr := Track{Path: "song.mp3"}
	got := tr.DisplayTitle()
	want := "song.mp3"
	if got != want {
		t.Errorf("DisplayTitle() = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// IsSupportedFile tests
// ---------------------------------------------------------------------------

func TestIsSupportedFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"track.mp3", true},
		{"track.MP3", true},
		{"album/song.flac", true},
		{"track.wav", true},
		{"track.ogg", true},
		{"readme.txt", false},
		{"image.png", false},
		{"noext", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := IsSupportedFile(tt.path); got != tt.want {
				t.Errorf("IsSupportedFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Player unit tests (state machine, no actual audio)
// ---------------------------------------------------------------------------

func TestNew_DefaultState(t *testing.T) {
	p := New()
	defer p.Close()

	if p.CurrentState() != Stopped {
		t.Errorf("New player state = %v, want Stopped", p.CurrentState())
	}
	if p.CurrentVolume() != defaultVolume {
		t.Errorf("New player volume = %v, want %v", p.CurrentVolume(), defaultVolume)
	}
}

func TestPlayer_SetVolume_Clamp(t *testing.T) {
	p := New()
	defer p.Close()

	p.SetVolume(1.5)
	if p.CurrentVolume() != 1.0 {
		t.Errorf("SetVolume(1.5) clamped to %v, want 1.0", p.CurrentVolume())
	}

	// Drain message.
	select {
	case <-p.Msgs:
	case <-time.After(100 * time.Millisecond):
	}

	p.SetVolume(-0.3)
	if p.CurrentVolume() != 0.0 {
		t.Errorf("SetVolume(-0.3) clamped to %v, want 0.0", p.CurrentVolume())
	}
}

func TestPlayer_VolumeUpDown(t *testing.T) {
	p := New()
	defer p.Close()

	initial := p.CurrentVolume()

	p.VolumeUp()
	// Drain message.
	select {
	case <-p.Msgs:
	case <-time.After(100 * time.Millisecond):
	}

	afterUp := p.CurrentVolume()
	if afterUp <= initial {
		t.Errorf("VolumeUp: %v should be > %v", afterUp, initial)
	}

	p.VolumeDown()
	select {
	case <-p.Msgs:
	case <-time.After(100 * time.Millisecond):
	}

	afterDown := p.CurrentVolume()
	diff := afterDown - initial
	if diff < -0.001 || diff > 0.001 {
		t.Errorf("VolumeDown after VolumeUp: %v should be ≈ %v", afterDown, initial)
	}
}

func TestPlayer_PlayInvalidFile(t *testing.T) {
	p := New()
	defer p.Close()

	// Play a non-existent file — should produce an ErrMsg, never panic.
	p.Play(Track{Path: "/nonexistent/track.mp3"})

	select {
	case msg := <-p.Msgs:
		if _, ok := msg.(ErrMsg); !ok {
			t.Errorf("Expected ErrMsg, got %T", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for ErrMsg from Play on invalid file")
	}
}

func TestPlayer_StopWhenAlreadyStopped(t *testing.T) {
	p := New()
	defer p.Close()

	// Should not panic.
	p.Stop()

	select {
	case msg := <-p.Msgs:
		if _, ok := msg.(PlaybackStoppedMsg); !ok {
			t.Errorf("Expected PlaybackStoppedMsg, got %T", msg)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timed out waiting for PlaybackStoppedMsg")
	}
}

func TestState_String(t *testing.T) {
	tests := []struct {
		s    State
		want string
	}{
		{Stopped, "stopped"},
		{Playing, "playing"},
		{Paused, "paused"},
		{State(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.s.String(); got != tt.want {
			t.Errorf("State(%d).String() = %q, want %q", tt.s, got, tt.want)
		}
	}
}

func TestErrMsg_Error(t *testing.T) {
	e := ErrMsg{Err: fmt.Errorf("test error")}
	if e.Error() != "test error" {
		t.Errorf("ErrMsg.Error() = %q, want %q", e.Error(), "test error")
	}
}

// ---------------------------------------------------------------------------
// Volume math tests
// ---------------------------------------------------------------------------

func TestLinearToVolume(t *testing.T) {
	// linear=1.0 → volume=0 (unity gain)
	if v := linearToVolume(1.0); v != 0 {
		t.Errorf("linearToVolume(1.0) = %v, want 0", v)
	}

	// linear=0.5 → volume=-1 (half amplitude)
	if v := linearToVolume(0.5); v != -1 {
		t.Errorf("linearToVolume(0.5) = %v, want -1", v)
	}

	// linear=0 → very negative (silent)
	if v := linearToVolume(0); v > -5 {
		t.Errorf("linearToVolume(0) = %v, want <= -5", v)
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		v, lo, hi, want float64
	}{
		{0.5, 0, 1, 0.5},
		{-1, 0, 1, 0},
		{2, 0, 1, 1},
		{0, 0, 0, 0},
	}
	for _, tt := range tests {
		if got := clamp(tt.v, tt.lo, tt.hi); got != tt.want {
			t.Errorf("clamp(%v, %v, %v) = %v, want %v", tt.v, tt.lo, tt.hi, got, tt.want)
		}
	}
}
