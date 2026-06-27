# µm

![Go](https://img.shields.io/badge/go-1.25%2B-00ADD8?style=flat-square&logo=go)
![License](https://img.shields.io/badge/license-MIT-green?style=flat-square)

**A keyboard-driven TUI music player for local libraries.**

µm scans a directory of audio files, extracts metadata in parallel, and plays them through a minimal terminal interface with vim-style navigation.

---

## Screenshot

> _Screenshot / demo GIF coming soon. Run `um ~/Music` to see it live._

---

## Features

- Plays **MP3, FLAC, WAV, and OGG** files
- Parallel metadata extraction — large libraries load fast
- Vim-style navigation (`j`/`k`, `g`/`G`, `ctrl+u`/`ctrl+d`)
- Real-time **search** filtered by title, artist, or album
- **Queue view** showing the current playback order, including after shuffle
- Shuffle and three-mode repeat (off / all / one)
- Seek forward and back in 5-second increments
- **Persistent volume** — restored automatically between sessions
- Jump to the currently playing track from anywhere in the list
- Progress bar and volume indicator in the now-playing panel
- Graceful fallback: filenames used when ID3 tags are absent

---

## Requirements

- Go 1.21 or later (module declares `go 1.25.6`)
- A terminal with 256-color support
- Audio output device recognized by the OS

> macOS, Linux, and Windows are supported by the underlying libraries. macOS and Linux are tested.

---

## Installation

**Build from source:**

```sh
git clone https://github.com/yourusername/um.git
cd um
go build -o um ./cmd/um
```

**Install directly with `go install`:**

```sh
go install um/cmd/um@latest
```

---

## Usage

```sh
um <music-directory>

# Examples
um ~/Music
um /mnt/nas/audio
um .
```

µm scans the given directory recursively on startup. Once loaded, the full library is available for navigation and playback.

---

## Keybindings

### Navigation

| Key       | Action                       |
| --------- | ---------------------------- |
| `j` / `↓` | Move down                    |
| `k` / `↑` | Move up                      |
| `g`       | Jump to top                  |
| `G`       | Jump to bottom               |
| `ctrl+u`  | Half-page up                 |
| `ctrl+d`  | Half-page down               |
| `c`       | Jump cursor to current track |

### Playback

| Key       | Action                 |
| --------- | ---------------------- |
| `enter`   | Play selected track    |
| `space`   | Pause / resume         |
| `s`       | Stop                   |
| `n`       | Next track             |
| `p`       | Previous track         |
| `h` / `←` | Seek back 5 seconds    |
| `l` / `→` | Seek forward 5 seconds |
| `+` / `=` | Volume up              |
| `-` / `_` | Volume down            |

### Queue & Modes

| Key   | Action                                 |
| ----- | -------------------------------------- |
| `z`   | Toggle shuffle                         |
| `r`   | Cycle repeat: off → all → one          |
| `tab` | Switch between Library and Queue views |

### Search

| Key      | Action                            |
| -------- | --------------------------------- |
| `/`      | Enter search mode                 |
| _(type)_ | Filter by title, artist, or album |
| `esc`    | Clear search and exit search mode |
| `enter`  | Play selected result              |

### Other

| Key            | Action                               |
| -------------- | ------------------------------------ |
| `q` / `ctrl+c` | Quit (volume is saved automatically) |

---

## Project Structure

```
.
├── cmd/
│   └── um/
│       └── main.go           # Entry point — validates args, starts the TUI
├── internal/
│   ├── audio/
│   │   ├── decoder.go        # Format dispatch: MP3, FLAC, WAV, OGG
│   │   ├── messages.go       # tea.Msg types for player events
│   │   ├── player.go         # Concurrent playback engine (beep wrapper)
│   │   └── track.go          # Track struct and metadata helpers
│   ├── config/
│   │   └── settings.go       # JSON persistence (~/.config/um/settings.json)
│   ├── library/
│   │   ├── metadata.go       # ID3/Vorbis tag extraction with filename fallback
│   │   ├── scanner.go        # Parallel recursive directory scan
│   │   └── sort.go           # Sort by path or artist/album/track
│   ├── queue/
│   │   └── queue.go          # Playback order — shuffle, repeat, navigation
│   └── tui/
│       └── app.go            # Bubble Tea model, view, update, and key handling
└── pkg/                      # Reserved for future public utilities
```

---

## Architecture

µm follows the [Elm Architecture](https://guide.elm-lang.org/architecture/) as implemented by [Bubble Tea](https://github.com/charmbracelet/bubbletea):

**Model** (`tui.Model`) is the single source of truth. It holds the track list, playback state, queue position, search query, and UI dimensions.

**Update** handles all incoming messages — key presses, window resize, scan completion, and player events — and returns a new model plus any commands to run.

**View** is a pure function that renders the model to a string. It never executes side effects.

**Concurrency** is handled through `tea.Cmd`:

- Library scanning runs in a goroutine and sends `scanCompleteMsg` when done
- The audio player runs on its own goroutine and sends typed messages (`PlaybackStartedMsg`, `PlaybackProgressMsg`, `TrackFinishedMsg`, etc.) back to the TUI via a buffered channel
- The TUI drains this channel with `waitForPlayer`, which returns a `tea.Cmd` that blocks until the next player event — keeping the event loop responsive without polling

---

## Dependencies

| Package                                                                 | Purpose                          |
| ----------------------------------------------------------------------- | -------------------------------- |
| [`charmbracelet/bubbletea`](https://github.com/charmbracelet/bubbletea) | TUI framework (Elm Architecture) |
| [`charmbracelet/lipgloss`](https://github.com/charmbracelet/lipgloss)   | Terminal styling                 |
| [`gopxl/beep`](https://github.com/gopxl/beep)                           | Audio decoding and playback      |
| [`dhowden/tag`](https://github.com/dhowden/tag)                         | ID3 and Vorbis tag extraction    |

---

## Current Limitations

- Track duration is unknown until a track starts playing — `dhowden/tag` does not expose duration, so the progress bar initializes to zero on the first tick
- No gapless playback between tracks
- Library is always sorted by file path on load (artist/album sort exists in `library/sort.go` but is not yet exposed via a keybinding)
- No way to add individual tracks to the queue without immediately jumping to them
- No playlist import or export (M3U, etc.)
- No lyrics display
- Seeking is not supported on all OGG files depending on how they were encoded

---

## Roadmap

- [ ] Sort toggle (path / artist-album-track)
- [ ] Enqueue without playing (`a` key)
- [ ] M3U playlist export
- [ ] `go install` binary release via GitHub Actions
- [ ] `--version` flag

---

## Contributing

1. Fork the repository and create a feature branch
2. Run `go test ./...` before submitting — all tests must pass
3. Run `gofmt -w .` — the CI will reject unformatted code
4. Open a pull request with a clear description of the change

Bug reports are welcome as GitHub issues. Please include the OS, terminal emulator, and audio format that triggered the issue.

---

## Suggested GitHub Topics

`go` `tui` `terminal` `music-player` `bubbletea` `charmbracelet` `cli` `audio`

---

## License

MIT — see [LICENSE](LICENSE).
