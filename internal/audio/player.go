package audio

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/effects"
	"github.com/gopxl/beep/v2/speaker"
)

// ---------------------------------------------------------------------------
// Playback state enum
// ---------------------------------------------------------------------------

// State describes the current playback state of the Player.
type State int

const (
	// Stopped means no track is loaded.
	Stopped State = iota
	// Playing means a track is actively playing.
	Playing
	// Paused means playback is suspended and can be resumed.
	Paused
)

// String implements fmt.Stringer for State.
func (s State) String() string {
	switch s {
	case Stopped:
		return "stopped"
	case Playing:
		return "playing"
	case Paused:
		return "paused"
	default:
		return "unknown"
	}
}

// ---------------------------------------------------------------------------
// Player configuration
// ---------------------------------------------------------------------------

const (
	// defaultSampleRate is the target sample rate to which all audio is
	// resampled. 44100 Hz covers the vast majority of local music files.
	defaultSampleRate beep.SampleRate = 44100

	// speakerBufferDuration controls the speaker buffer size. Smaller
	// values reduce latency but increase CPU usage.
	speakerBufferDuration = time.Second / 10

	// progressTickInterval is how often PlaybackProgressMsg is sent to
	// the TUI while a track is playing.
	progressTickInterval = 500 * time.Millisecond

	// defaultVolume is the starting volume (linear 0–1).
	defaultVolume = 0.7

	// volumeStep is the amount added/subtracted per VolumeUp/VolumeDown.
	volumeStep = 0.05
)

// ---------------------------------------------------------------------------
// Player
// ---------------------------------------------------------------------------

// Player is the concurrent audio playback engine. All public methods are
// safe to call from the Bubble Tea Update loop (or any goroutine). The
// Player relays state changes back to the TUI exclusively via the Msgs
// channel — it never touches the TUI Model.
type Player struct {
	// Msgs is a channel of tea.Msg values. The TUI should drain this
	// channel in a tea.Cmd (e.g., via a WaitForActivity helper).
	Msgs chan interface{}

	mu sync.RWMutex

	state  State
	track  Track
	volume float64 // linear 0–1

	// Beep internals — guarded by speaker.Lock / mu.
	streamer   beep.StreamSeekCloser
	ctrl       *beep.Ctrl
	volumeCtrl *effects.Volume
	format     beep.Format

	// done is used to signal the progress ticker to stop.
	done chan struct{}

	// speakerInitialised tracks whether speaker.Init has been called.
	speakerInitialised bool
}

// New creates a new Player with sensible defaults. The returned Player
// is in the Stopped state with no track loaded.
func New() *Player {
	return &Player{
		Msgs:   make(chan interface{}, 64),
		volume: defaultVolume,
		state:  Stopped,
		done:   make(chan struct{}),
	}
}

// ---------------------------------------------------------------------------
// Public command methods (called from the TUI)
// ---------------------------------------------------------------------------

// Play decodes and begins playing the given track. If another track is
// already playing it is stopped first (streams are closed to prevent
// resource leaks). Errors are sent as ErrMsg on the Msgs channel.
func (p *Player) Play(t Track) {
	go p.playAsync(t)
}

// TogglePause pauses if playing, or resumes if paused.
func (p *Player) TogglePause() {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch p.state {
	case Playing:
		speaker.Lock()
		p.ctrl.Paused = true
		speaker.Unlock()
		p.state = Paused
		p.send(PlaybackPausedMsg{})

	case Paused:
		speaker.Lock()
		p.ctrl.Paused = false
		speaker.Unlock()
		p.state = Playing
		p.send(PlaybackResumedMsg{})
		go p.progressLoop()
	}
}

// Stop halts playback and releases all audio resources.
func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()
	p.send(PlaybackStoppedMsg{})
}

// SeekForward seeks forward by the given duration.
func (p *Player) SeekForward(d time.Duration) {
	p.seekRelative(d)
}

// SeekBackward seeks backward by the given duration.
func (p *Player) SeekBackward(d time.Duration) {
	p.seekRelative(-d)
}

// VolumeUp increases volume by one step.
func (p *Player) VolumeUp() {
	p.adjustVolume(volumeStep)
}

// VolumeDown decreases volume by one step.
func (p *Player) VolumeDown() {
	p.adjustVolume(-volumeStep)
}

// SetVolume sets the volume to a specific value in [0, 1].
func (p *Player) SetVolume(v float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.volume = clamp(v, 0, 1)
	if p.volumeCtrl != nil {
		speaker.Lock()
		p.volumeCtrl.Volume = linearToVolume(p.volume)
		p.volumeCtrl.Silent = p.volume == 0
		speaker.Unlock()
	}
	p.send(VolumeChangedMsg{Volume: p.volume})
}

// ---------------------------------------------------------------------------
// Read-only state accessors (safe for concurrent reads)
// ---------------------------------------------------------------------------

// CurrentState returns the current playback state.
func (p *Player) CurrentState() State {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state
}

// CurrentTrack returns the currently loaded track (zero value if stopped).
func (p *Player) CurrentTrack() Track {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.track
}

// CurrentPosition returns the playback position of the current track.
func (p *Player) CurrentPosition() time.Duration {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.streamer == nil {
		return 0
	}
	speaker.Lock()
	pos := p.format.SampleRate.D(p.streamer.Position())
	speaker.Unlock()
	return pos
}

// CurrentVolume returns the current volume as a linear value in [0, 1].
func (p *Player) CurrentVolume() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.volume
}

// TrackDuration returns the total duration of the current track.
func (p *Player) TrackDuration() time.Duration {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.streamer == nil {
		return 0
	}
	return p.format.SampleRate.D(p.streamer.Len())
}

// ---------------------------------------------------------------------------
// WaitForActivity returns a tea.Cmd-friendly function that blocks until
// the next message is available on the Msgs channel. Usage:
//
//	func waitForPlayerMsg(p *audio.Player) tea.Cmd {
//	    return func() tea.Msg {
//	        return p.WaitForActivity()
//	    }
//	}
// ---------------------------------------------------------------------------

// WaitForActivity blocks until the Player emits its next message. The
// returned value is always one of the exported Msg types in messages.go.
func (p *Player) WaitForActivity() interface{} {
	return <-p.Msgs
}

// ---------------------------------------------------------------------------
// Close shuts down the player and releases all resources. It should be
// called when the application exits.
// ---------------------------------------------------------------------------

// Close stops playback and drains the message channel.
func (p *Player) Close() {
	p.mu.Lock()
	p.stopLocked()
	p.mu.Unlock()
	close(p.Msgs)
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// playAsync runs on its own goroutine so it never blocks the TUI event
// loop (per AGENTS.md concurrency rule).
func (p *Player) playAsync(t Track) {
	result, err := decodeFile(t.Path)
	if err != nil {
		p.send(ErrMsg{Err: fmt.Errorf("play %q: %w", t.Path, err)})
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Stop any previous track and close its resources.
	p.stopLocked()

	// Initialise the speaker once.
	if !p.speakerInitialised {
		err = speaker.Init(
			defaultSampleRate,
			defaultSampleRate.N(speakerBufferDuration),
		)
		if err != nil {
			result.streamer.Close()
			p.send(ErrMsg{Err: fmt.Errorf("speaker init: %w", err)})
			return
		}
		p.speakerInitialised = true
	}

	// Resample if the file's sample rate differs from our target.
	var streamer beep.Streamer = result.streamer
	if result.format.SampleRate != defaultSampleRate {
		streamer = beep.Resample(4, result.format.SampleRate, defaultSampleRate, result.streamer)
	}

	// Wrap in control and volume streamers.
	p.ctrl = &beep.Ctrl{Streamer: streamer, Paused: false}
	p.volumeCtrl = &effects.Volume{
		Streamer: p.ctrl,
		Base:     2,
		Volume:   linearToVolume(p.volume),
		Silent:   p.volume == 0,
	}
	p.streamer = result.streamer
	p.format = result.format
	p.track = t
	p.state = Playing
	p.done = make(chan struct{})

	// Compute track duration from the decoded stream length.
	dur := p.format.SampleRate.D(p.streamer.Len())
	t.Duration = dur

	// Play through the speaker. When the stream ends, the callback
	// fires on the audio goroutine — we relay via message channel.
	done := p.done
	speaker.Play(beep.Seq(p.volumeCtrl, beep.Callback(func() {
		p.mu.Lock()
		// Only send TrackFinishedMsg if we haven't been stopped/replaced.
		if p.done == done {
			p.state = Stopped
		}
		p.mu.Unlock()
		select {
		case <-done:
		default:
			p.send(TrackFinishedMsg{})
		}
	})))

	p.send(PlaybackStartedMsg{Track: t, Duration: dur})

	// Start the progress ticker in a new goroutine.
	go p.progressLoop()
}

// progressLoop periodically sends PlaybackProgressMsg while the player is
// in the Playing state. It exits when the done channel is closed or the
// player is no longer playing.
func (p *Player) progressLoop() {
	ticker := time.NewTicker(progressTickInterval)
	defer ticker.Stop()

	// Capture the done channel under the lock so we watch the right
	// generation of playback.
	p.mu.RLock()
	done := p.done
	p.mu.RUnlock()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			p.mu.RLock()
			state := p.state
			if state != Playing || p.streamer == nil {
				p.mu.RUnlock()
				return
			}
			speaker.Lock()
			pos := p.format.SampleRate.D(p.streamer.Position())
			dur := p.format.SampleRate.D(p.streamer.Len())
			speaker.Unlock()
			p.mu.RUnlock()

			p.send(PlaybackProgressMsg{Position: pos, Duration: dur})
		}
	}
}

// stopLocked releases all playback resources. Caller must hold p.mu.
func (p *Player) stopLocked() {
	if p.state == Stopped && p.streamer == nil {
		return
	}

	// Signal the progress ticker and any pending callback to stop.
	select {
	case <-p.done:
		// Already closed.
	default:
		close(p.done)
	}

	// Clear the speaker to halt audio output immediately.
	speaker.Clear()

	// Close the streamer to release the underlying os.File and decoder
	// resources, preventing memory/fd leaks on track switch.
	if p.streamer != nil {
		p.streamer.Close()
		p.streamer = nil
	}
	p.ctrl = nil
	p.volumeCtrl = nil
	p.state = Stopped
	p.track = Track{}
}

// seekRelative seeks the current track by the given delta (positive =
// forward, negative = backward). Clamped to [0, length].
func (p *Player) seekRelative(delta time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.streamer == nil {
		return
	}

	speaker.Lock()
	cur := p.streamer.Position()
	total := p.streamer.Len()
	samples := p.format.SampleRate.N(delta)
	target := cur + samples
	if target < 0 {
		target = 0
	}
	if target > total {
		target = total
	}
	err := p.streamer.Seek(target)
	speaker.Unlock()

	if err != nil {
		p.send(ErrMsg{Err: fmt.Errorf("seek: %w", err)})
		return
	}

	pos := p.format.SampleRate.D(target)
	p.send(SeekCompleteMsg{Position: pos})
}

// adjustVolume changes volume by the given delta and clamps to [0, 1].
func (p *Player) adjustVolume(delta float64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.volume = clamp(p.volume+delta, 0, 1)

	if p.volumeCtrl != nil {
		speaker.Lock()
		p.volumeCtrl.Volume = linearToVolume(p.volume)
		p.volumeCtrl.Silent = p.volume == 0
		speaker.Unlock()
	}
	p.send(VolumeChangedMsg{Volume: p.volume})
}

// send pushes a message to the Msgs channel without blocking. If the
// channel is full the message is dropped to prevent deadlocking the
// audio goroutine.
func (p *Player) send(msg interface{}) {
	select {
	case p.Msgs <- msg:
	default:
		// Channel full — drop the message. This is intentional: we must
		// never block the audio goroutine, and a dropped progress tick
		// is harmless.
	}
}

// ---------------------------------------------------------------------------
// Volume math helpers
// ---------------------------------------------------------------------------

// linearToVolume converts a linear [0, 1] volume to the exponential
// base-2 scale that effects.Volume expects. A linear value of 0.5
// should sound roughly "half volume" to the listener.
func linearToVolume(linear float64) float64 {
	if linear <= 0 {
		return -10 // effectively silent
	}
	// volume = log2(linear); base=2 so effects.Volume multiplies by
	// 2^volume. log2(0.5) = -1, log2(1) = 0.
	return math.Log2(linear)
}

// clamp restricts v to the range [lo, hi].
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
