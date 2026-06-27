package library

import (
	"os"
	"path/filepath"
	"testing"

	"um/internal/audio"
)

// ---------------------------------------------------------------------------
// Scanner tests
// ---------------------------------------------------------------------------

// createTestTree builds a temporary directory tree with fake audio files
// and non-audio files for testing the scanner.
func createTestTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	// Create subdirectory structure.
	sub := filepath.Join(root, "artist", "album")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	// Supported files (content doesn't matter for the walk phase).
	for _, name := range []string{"track1.mp3", "track2.flac", "track3.wav", "track4.ogg"} {
		if err := os.WriteFile(filepath.Join(sub, name), []byte("fake"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Unsupported files should be ignored.
	for _, name := range []string{"cover.jpg", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(sub, name), []byte("nope"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Root-level supported file.
	if err := os.WriteFile(filepath.Join(root, "root.mp3"), []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	return root
}

func TestCollectPaths_FindsAllSupportedFiles(t *testing.T) {
	root := createTestTree(t)
	paths, errs := collectPaths(root)

	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}

	// 4 in sub + 1 at root = 5.
	if got := len(paths); got != 5 {
		t.Errorf("collectPaths found %d files, want 5", got)
	}
}

func TestCollectPaths_IgnoresUnsupportedFiles(t *testing.T) {
	root := createTestTree(t)
	paths, _ := collectPaths(root)
	for _, p := range paths {
		if !audio.IsSupportedFile(p) {
			t.Errorf("collectPaths returned unsupported file: %s", p)
		}
	}
}

func TestCollectPaths_NonExistentRoot(t *testing.T) {
	paths, errs := collectPaths("/nonexistent/dir/42")
	if len(paths) != 0 {
		t.Errorf("expected no paths for non-existent root, got %d", len(paths))
	}
	if len(errs) == 0 {
		t.Error("expected errors for non-existent root")
	}
}

func TestScan_IntegrationWithWorkers(t *testing.T) {
	root := createTestTree(t)

	// Tags will fail on our fake files, but we should still get tracks
	// with fallback titles derived from filenames.
	result := Scan(root, 2)

	if got := len(result.Tracks); got != 5 {
		t.Errorf("Scan found %d tracks, want 5", got)
	}

	// Verify fallback titles are set (no real tags in fake files).
	for _, tr := range result.Tracks {
		if tr.Title == "" {
			t.Errorf("Track at %q has empty title after fallback", tr.Path)
		}
	}
}

func TestScan_DefaultWorkers(t *testing.T) {
	root := createTestTree(t)

	// numWorkers=0 should default to runtime.NumCPU() and not panic.
	result := Scan(root, 0)
	if len(result.Tracks) != 5 {
		t.Errorf("Scan(workers=0) found %d tracks, want 5", len(result.Tracks))
	}
}

// ---------------------------------------------------------------------------
// Metadata tests
// ---------------------------------------------------------------------------

func TestTitleFromFilename(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/home/user/music/01 - Intro.mp3", "01 - Intro"},
		{"song.flac", "song"},
		{"/deep/dir/Track.WAV", "Track"},
		{"noext", "noext"},
	}
	for _, tt := range tests {
		got := titleFromFilename(tt.path)
		if got != tt.want {
			t.Errorf("titleFromFilename(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestStringOrFallback(t *testing.T) {
	if got := stringOrFallback("hello", "world"); got != "hello" {
		t.Errorf("expected %q, got %q", "hello", got)
	}
	if got := stringOrFallback("", "world"); got != "world" {
		t.Errorf("expected %q, got %q", "world", got)
	}
}

// ---------------------------------------------------------------------------
// Sort tests
// ---------------------------------------------------------------------------

func TestSortByPath(t *testing.T) {
	tracks := []audio.Track{
		{Path: "/z/track.mp3"},
		{Path: "/a/track.mp3"},
		{Path: "/m/track.mp3"},
	}
	SortByPath(tracks)
	if tracks[0].Path != "/a/track.mp3" || tracks[2].Path != "/z/track.mp3" {
		t.Errorf("SortByPath incorrect order: %v", tracks)
	}
}

func TestSortByAlbumTrack(t *testing.T) {
	tracks := []audio.Track{
		{Artist: "Zephyr", Album: "Alpha", TrackNumber: 2},
		{Artist: "Zephyr", Album: "Alpha", TrackNumber: 1},
		{Artist: "Apex", Album: "Beta", TrackNumber: 1},
	}
	SortByAlbumTrack(tracks)

	if tracks[0].Artist != "Apex" {
		t.Errorf("expected Apex first, got %s", tracks[0].Artist)
	}
	if tracks[1].TrackNumber != 1 || tracks[2].TrackNumber != 2 {
		t.Errorf("Zephyr tracks not sorted by track number: %v", tracks[1:])
	}
}

func TestDurationFromSeconds(t *testing.T) {
	got := durationFromSeconds(90)
	if got.Seconds() != 90 {
		t.Errorf("durationFromSeconds(90) = %v, want 90s", got)
	}
}
