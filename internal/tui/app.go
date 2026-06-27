// Package tui implements the Bubble Tea TUI for µm. It wires the
// audio.Player, library scanner, and queue together into a keyboard-
// driven terminal interface.
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ohhmkar/um/internal/audio"
	"github.com/ohhmkar/um/internal/config"
	"github.com/ohhmkar/um/internal/library"
	"github.com/ohhmkar/um/internal/queue"
)

// ---------------------------------------------------------------------------
// Styles
// ---------------------------------------------------------------------------

var (
	accent     = lipgloss.Color("#7C3AED")
	accentDim  = lipgloss.Color("#A78BFA")
	subtle     = lipgloss.Color("#6B7280")
	textNormal = lipgloss.Color("#E5E7EB")
	textBright = lipgloss.Color("#F9FAFB")
	errColor   = lipgloss.Color("#EF4444")

	headerStyle = lipgloss.NewStyle().
			Foreground(accent).
			Bold(true)

	tabActiveStyle = lipgloss.NewStyle().
			Foreground(textBright).
			Bold(true)

	tabInactiveStyle = lipgloss.NewStyle().
				Foreground(subtle)

	selectedStyle = lipgloss.NewStyle().
			Foreground(textBright).
			Background(accent).
			Bold(true).
			Padding(0, 1)

	normalStyle = lipgloss.NewStyle().
			Foreground(textNormal).
			Padding(0, 1)

	dimStyle = lipgloss.NewStyle().
			Foreground(subtle)

	nowPlayingStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accent).
			Padding(0, 1).
			Width(60)

	statusStyle = lipgloss.NewStyle().
			Foreground(accentDim).
			Italic(true)

	searchStyle = lipgloss.NewStyle().
			Foreground(textBright).
			Bold(true)

	errStyle = lipgloss.NewStyle().
			Foreground(errColor)

	helpStyle = lipgloss.NewStyle().
			Foreground(subtle).
			Italic(true)

	progressFull  = lipgloss.NewStyle().Foreground(accent)
	progressEmpty = lipgloss.NewStyle().Foreground(subtle)
)

// ---------------------------------------------------------------------------
// Pane
// ---------------------------------------------------------------------------

type pane int

const (
	paneLibrary pane = iota
	paneQueue
)

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

type scanCompleteMsg struct{ result library.ScanResult }
type playerMsg struct{ inner any }

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

type Model struct {
	player *audio.Player
	queue  *queue.Queue
	cfg    config.Settings

	// Library data
	tracks     []audio.Track
	viewTracks []audio.Track // filtered when searching, otherwise == tracks

	// Library pane state
	cursor  int
	offset  int
	maxRows int

	// Queue pane state
	queueCursor int
	queueOffset int

	// Search state
	searching   bool
	searchQuery string

	// Active pane
	activePane pane

	// Now-playing state
	nowPlaying audio.Track
	position   time.Duration
	duration   time.Duration
	state      audio.State
	volume     float64

	// UI state
	width   int
	height  int
	err     string
	status  string
	loading bool
	rootDir string
}

// NewModel creates the initial model. rootDir is the music directory to scan.
func NewModel(rootDir string) Model {
	cfg := config.Load()
	p := audio.New()
	p.SetVolume(cfg.Volume)
	return Model{
		player:     p,
		queue:      queue.New(),
		cfg:        cfg,
		volume:     cfg.Volume,
		maxRows:    20,
		loading:    true,
		rootDir:    rootDir,
		activePane: paneLibrary,
	}
}

// Init kicks off the library scan and starts listening for player messages.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.scanLibrary(), m.waitForPlayer())
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.maxRows = max(m.height-14, 5)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case scanCompleteMsg:
		m.loading = false
		m.tracks = msg.result.Tracks
		library.SortByPath(m.tracks)
		m.viewTracks = m.tracks
		m.queue.Set(m.tracks)
		if len(msg.result.Errors) > 0 {
			m.err = fmt.Sprintf("%d scan errors (first: %s)",
				len(msg.result.Errors), msg.result.Errors[0])
		}
		m.status = fmt.Sprintf("Loaded %d tracks from %s", len(m.tracks), m.rootDir)
		return m, nil

	case playerMsg:
		return m.handlePlayerMsg(msg.inner)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Search mode intercepts most keys.
	if m.searching {
		return m.handleSearchKey(msg)
	}

	switch msg.String() {

	// Quit — persist settings.
	case "q", "ctrl+c":
		m.cfg.Volume = m.volume
		m.cfg.LastDir = m.rootDir
		config.Save(m.cfg)
		m.player.Close()
		return m, tea.Quit

	// Switch pane.
	case "tab":
		if m.activePane == paneLibrary {
			m.activePane = paneQueue
			m.queueCursor = m.queue.CursorIndex()
			m.queueOffset = 0
			if m.queueCursor >= m.maxRows {
				m.queueOffset = m.queueCursor - m.maxRows/2
			}
		} else {
			m.activePane = paneLibrary
		}

	// Playback controls (work in both panes).
	case " ":
		m.player.TogglePause()
	case "s":
		m.player.Stop()
	case "n":
		if t, ok := m.queue.Next(); ok {
			m.player.Play(t)
		}
	case "p":
		if t, ok := m.queue.Previous(); ok {
			m.player.Play(t)
		}
	case "l", "right":
		m.player.SeekForward(5 * time.Second)
	case "h", "left":
		m.player.SeekBackward(5 * time.Second)
	case "+", "=":
		m.player.VolumeUp()
	case "-", "_":
		m.player.VolumeDown()
	case "z":
		m.queue.ToggleShuffle()
		if m.queue.Shuffle() {
			m.status = "Shuffle: ON"
		} else {
			m.status = "Shuffle: OFF"
		}
	case "r":
		mode := m.queue.CycleRepeat()
		m.status = fmt.Sprintf("Repeat: %s", mode)

	// Navigation — pane-specific.
	case "j", "down":
		if m.activePane == paneLibrary {
			if m.cursor < len(m.viewTracks)-1 {
				m.cursor++
				m.ensureVisible()
			}
		} else {
			if m.queueCursor < m.queue.Len()-1 {
				m.queueCursor++
				m.queueEnsureVisible()
			}
		}

	case "k", "up":
		if m.activePane == paneLibrary {
			if m.cursor > 0 {
				m.cursor--
				m.ensureVisible()
			}
		} else {
			if m.queueCursor > 0 {
				m.queueCursor--
				m.queueEnsureVisible()
			}
		}

	case "g":
		if m.activePane == paneLibrary {
			m.cursor = 0
			m.offset = 0
		} else {
			m.queueCursor = 0
			m.queueOffset = 0
		}

	case "G":
		if m.activePane == paneLibrary {
			m.cursor = max(0, len(m.viewTracks)-1)
			m.ensureVisible()
		} else {
			m.queueCursor = max(0, m.queue.Len()-1)
			m.queueEnsureVisible()
		}

	// Page half-screen up/down (library pane only).
	case "ctrl+u":
		if m.activePane == paneLibrary {
			m.cursor -= m.maxRows / 2
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.ensureVisible()
		}
	case "ctrl+d":
		if m.activePane == paneLibrary {
			m.cursor += m.maxRows / 2
			if m.cursor >= len(m.viewTracks) {
				m.cursor = max(0, len(m.viewTracks)-1)
			}
			m.ensureVisible()
		}

	// Enter — play selected track.
	case "enter":
		if m.activePane == paneLibrary {
			return m.playLibrarySelected()
		}
		return m.playQueueSelected()

	// Search (library pane only).
	case "/":
		if m.activePane == paneLibrary {
			m.searching = true
			m.searchQuery = ""
		}

	// Jump cursor to currently playing track (library pane only).
	case "c":
		if m.activePane == paneLibrary && m.nowPlaying.Path != "" {
			for i, t := range m.viewTracks {
				if t.Path == m.nowPlaying.Path {
					m.cursor = i
					m.ensureVisible()
					break
				}
			}
		}
	}

	return m, nil
}

func (m Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searching = false
		m.searchQuery = ""
		m.viewTracks = m.tracks
		m.cursor = 0
		m.offset = 0

	case "enter":
		m.searching = false
		return m.playLibrarySelected()

	case "backspace":
		if len(m.searchQuery) > 0 {
			runes := []rune(m.searchQuery)
			m.searchQuery = string(runes[:len(runes)-1])
			m.viewTracks = filterTracks(m.tracks, m.searchQuery)
			m.cursor = 0
			m.offset = 0
		}

	case "j", "down":
		if m.cursor < len(m.viewTracks)-1 {
			m.cursor++
			m.ensureVisible()
		}

	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
			m.ensureVisible()
		}

	default:
		if msg.Type == tea.KeyRunes {
			m.searchQuery += string(msg.Runes)
			m.viewTracks = filterTracks(m.tracks, m.searchQuery)
			m.cursor = 0
			m.offset = 0
		}
	}
	return m, nil
}

func (m Model) handlePlayerMsg(msg any) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case audio.PlaybackStartedMsg:
		m.nowPlaying = msg.Track
		m.duration = msg.Duration
		m.position = 0
		m.state = audio.Playing
		m.err = ""
		m.status = fmt.Sprintf("Playing: %s", msg.Track.DisplayTitle())

	case audio.PlaybackProgressMsg:
		m.position = msg.Position
		m.duration = msg.Duration

	case audio.PlaybackPausedMsg:
		m.state = audio.Paused
		m.status = "Paused"

	case audio.PlaybackResumedMsg:
		m.state = audio.Playing
		m.status = "Resumed"

	case audio.PlaybackStoppedMsg:
		m.state = audio.Stopped
		m.position = 0
		m.status = "Stopped"

	case audio.TrackFinishedMsg:
		if t, ok := m.queue.Next(); ok {
			m.player.Play(t)
		} else {
			m.state = audio.Stopped
			m.status = "Queue finished"
		}

	case audio.VolumeChangedMsg:
		m.volume = msg.Volume

	case audio.SeekCompleteMsg:
		m.position = msg.Position

	case audio.ErrMsg:
		m.err = msg.Error()
	}

	return m, m.waitForPlayer()
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (m Model) View() string {
	var b strings.Builder

	// Tab bar.
	libLabel := tabActiveStyle.Render("  Library  ")
	queueLabel := tabInactiveStyle.Render("  Queue  ")
	if m.activePane == paneQueue {
		libLabel = tabInactiveStyle.Render("  Library  ")
		queueLabel = tabActiveStyle.Render("  Queue  ")
	}
	b.WriteString(libLabel)
	b.WriteString(dimStyle.Render("│"))
	b.WriteString(queueLabel)
	b.WriteString("\n")

	if m.loading {
		b.WriteString(statusStyle.Render("  Scanning " + m.rootDir + " ..."))
		b.WriteString("\n")
		return b.String()
	}

	// Subtitle / search bar.
	if m.activePane == paneLibrary {
		if m.searching {
			b.WriteString("  ")
			b.WriteString(headerStyle.Render("/"))
			b.WriteString(searchStyle.Render(m.searchQuery))
			b.WriteString(headerStyle.Render("█"))
		} else if m.searchQuery != "" {
			b.WriteString(dimStyle.Render(fmt.Sprintf("  /%s  (%d results)", m.searchQuery, len(m.viewTracks))))
		} else {
			b.WriteString(dimStyle.Render(fmt.Sprintf("  %d tracks", len(m.tracks))))
		}
	} else {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  %d in queue", m.queue.Len())))
	}
	b.WriteString("\n\n")

	// Pane content.
	if m.activePane == paneLibrary {
		b.WriteString(m.renderLibraryPane())
	} else {
		b.WriteString(m.renderQueuePane())
	}

	// Now Playing panel.
	b.WriteString("\n")
	b.WriteString(m.renderNowPlaying())
	b.WriteString("\n")

	// Error / status line.
	if m.err != "" {
		b.WriteString(errStyle.Render("  ⚠ " + m.err))
		b.WriteString("\n")
	} else if m.status != "" {
		b.WriteString(statusStyle.Render("  " + m.status))
		b.WriteString("\n")
	}

	// Help bar.
	b.WriteString("\n")
	if m.searching {
		b.WriteString(helpStyle.Render("  type to filter • j/k navigate results • enter play • esc cancel"))
	} else if m.activePane == paneQueue {
		b.WriteString(helpStyle.Render("  j/k navigate • enter play • space pause • n/p next/prev • tab library • q quit"))
	} else {
		b.WriteString(helpStyle.Render(
			"  j/k ^u/^d navigate • enter play • space pause • n/p next/prev • h/l seek • +/- vol • / search • c jump-here • z shuffle • r repeat • tab queue • q quit",
		))
	}
	b.WriteString("\n")

	return b.String()
}

func (m Model) renderLibraryPane() string {
	var b strings.Builder

	if len(m.viewTracks) == 0 {
		if m.searching {
			b.WriteString(dimStyle.Render(fmt.Sprintf("  No results for %q", m.searchQuery)))
			b.WriteString("\n")
		}
		for i := 1; i < m.maxRows; i++ {
			b.WriteString("\n")
		}
		return b.String()
	}

	end := min(m.offset+m.maxRows, len(m.viewTracks))

	for i := m.offset; i < end; i++ {
		t := m.viewTracks[i]
		label := formatTrackLabel(t, i)
		switch {
		case i == m.cursor:
			b.WriteString(selectedStyle.Render(label))
		case t.Path == m.nowPlaying.Path && m.nowPlaying.Path != "":
			b.WriteString(lipgloss.NewStyle().Foreground(accent).Padding(0, 1).Render(label))
		default:
			b.WriteString(normalStyle.Render(label))
		}
		b.WriteString("\n")
	}

	// Pad to keep layout stable.
	for i := end - m.offset; i < m.maxRows; i++ {
		b.WriteString("\n")
	}
	return b.String()
}

func (m Model) renderQueuePane() string {
	var b strings.Builder

	playOrder := m.queue.PlayOrder()
	currentIdx := m.queue.CursorIndex()

	end := min(m.queueOffset+m.maxRows, len(playOrder))

	for i := m.queueOffset; i < end; i++ {
		t := playOrder[i]
		label := formatTrackLabel(t, i)
		isCurrentTrack := i == currentIdx && m.nowPlaying.Path != ""
		isCursor := i == m.queueCursor

		switch {
		case isCursor && isCurrentTrack:
			b.WriteString(selectedStyle.Render("▶ " + label))
		case isCursor:
			b.WriteString(selectedStyle.Render(label))
		case isCurrentTrack:
			b.WriteString(lipgloss.NewStyle().Foreground(accent).Padding(0, 1).Render("▶ " + label))
		default:
			b.WriteString(normalStyle.Render(label))
		}
		b.WriteString("\n")
	}

	for i := end - m.queueOffset; i < m.maxRows; i++ {
		b.WriteString("\n")
	}
	return b.String()
}

func (m Model) renderNowPlaying() string {
	if m.nowPlaying.Path == "" && m.state == audio.Stopped {
		return nowPlayingStyle.Render(dimStyle.Render("  No track playing"))
	}

	title := m.nowPlaying.DisplayTitle()
	artist := m.nowPlaying.Artist
	if artist == "" {
		artist = "Unknown Artist"
	}

	stateIcon := "■"
	switch m.state {
	case audio.Playing:
		stateIcon = "▶"
	case audio.Paused:
		stateIcon = "⏸"
	}

	bar := renderProgressBar(m.position, m.duration, 40)
	posStr := formatDuration(m.position)
	durStr := formatDuration(m.duration)
	volBar := renderVolumeBar(m.volume, 10)

	modes := ""
	if m.queue.Shuffle() {
		modes += " 🔀"
	}
	switch m.queue.Repeat() {
	case queue.RepeatAll:
		modes += " 🔁"
	case queue.RepeatOne:
		modes += " 🔂"
	}

	content := fmt.Sprintf(
		"  %s %s\n  %s\n  %s  %s / %s\n  Vol: %s%s",
		stateIcon,
		lipgloss.NewStyle().Foreground(textBright).Bold(true).Render(title),
		dimStyle.Render(artist),
		bar, posStr, durStr,
		volBar, modes,
	)

	return nowPlayingStyle.Render(content)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (m Model) playLibrarySelected() (tea.Model, tea.Cmd) {
	if len(m.viewTracks) == 0 || m.cursor >= len(m.viewTracks) {
		return m, nil
	}
	selected := m.viewTracks[m.cursor]
	if idx := m.originalIdx(selected.Path); idx >= 0 {
		m.queue.JumpTo(idx)
		if t, ok := m.queue.Current(); ok {
			m.player.Play(t)
		}
	}
	return m, nil
}

func (m Model) playQueueSelected() (tea.Model, tea.Cmd) {
	playOrder := m.queue.PlayOrder()
	if len(playOrder) == 0 || m.queueCursor >= len(playOrder) {
		return m, nil
	}
	selected := playOrder[m.queueCursor]
	if idx := m.originalIdx(selected.Path); idx >= 0 {
		m.queue.JumpTo(idx)
		if t, ok := m.queue.Current(); ok {
			m.player.Play(t)
		}
	}
	return m, nil
}

func (m Model) originalIdx(path string) int {
	for i, t := range m.tracks {
		if t.Path == path {
			return i
		}
	}
	return -1
}

func filterTracks(tracks []audio.Track, query string) []audio.Track {
	if query == "" {
		return tracks
	}
	lower := strings.ToLower(query)
	var out []audio.Track
	for _, t := range tracks {
		if strings.Contains(strings.ToLower(t.DisplayTitle()), lower) ||
			strings.Contains(strings.ToLower(t.Artist), lower) ||
			strings.Contains(strings.ToLower(t.Album), lower) {
			out = append(out, t)
		}
	}
	return out
}

func formatTrackLabel(t audio.Track, idx int) string {
	title := t.DisplayTitle()
	if t.Artist != "" {
		return fmt.Sprintf("%3d  %s — %s", idx+1, t.Artist, title)
	}
	return fmt.Sprintf("%3d  %s", idx+1, title)
}

func renderProgressBar(pos, dur time.Duration, width int) string {
	if dur <= 0 {
		return progressEmpty.Render(strings.Repeat("─", width))
	}
	ratio := float64(pos) / float64(dur)
	if ratio > 1 {
		ratio = 1
	}
	filled := int(ratio * float64(width))
	return progressFull.Render(strings.Repeat("━", filled)) +
		progressEmpty.Render(strings.Repeat("─", width-filled))
}

func renderVolumeBar(vol float64, width int) string {
	filled := int(vol * float64(width))
	return progressFull.Render(strings.Repeat("█", filled)) +
		progressEmpty.Render(strings.Repeat("░", width-filled))
}

func formatDuration(d time.Duration) string {
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%d:%02d", m, s)
}

func (m *Model) ensureVisible() {
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+m.maxRows {
		m.offset = m.cursor - m.maxRows + 1
	}
}

func (m *Model) queueEnsureVisible() {
	if m.queueCursor < m.queueOffset {
		m.queueOffset = m.queueCursor
	}
	if m.queueCursor >= m.queueOffset+m.maxRows {
		m.queueOffset = m.queueCursor - m.maxRows + 1
	}
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

func (m Model) scanLibrary() tea.Cmd {
	root := m.rootDir
	return func() tea.Msg {
		return scanCompleteMsg{result: library.Scan(root, 0)}
	}
}

func (m Model) waitForPlayer() tea.Cmd {
	p := m.player
	return func() tea.Msg {
		return playerMsg{inner: p.WaitForActivity()}
	}
}
