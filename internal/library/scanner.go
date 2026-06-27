// Package library provides concurrent directory scanning and metadata
// extraction for local music files. It produces a slice of audio.Track
// values ready for the TUI to display and the Player to consume.
package library

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/ohhmkar/um/internal/audio"
)

// ScanResult is the outcome of scanning a root directory.
type ScanResult struct {
	Tracks []audio.Track
	Errors []error
}

// Scan recursively walks root and collects every supported audio file.
// Metadata extraction is parallelised across numWorkers goroutines.
// If numWorkers <= 0, it defaults to runtime.NumCPU().
//
// Scan never panics. Per-file errors (permission denied, corrupt tags)
// are accumulated in ScanResult.Errors while all successfully parsed
// tracks appear in ScanResult.Tracks.
func Scan(root string, numWorkers int) ScanResult {
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU()
	}

	// Phase 1 — fast walk to collect paths (I/O-bound, single goroutine).
	paths, walkErrs := collectPaths(root)

	// Phase 2 — fan-out metadata extraction across workers.
	tracks, parseErrs := parseTracksParallel(paths, numWorkers)

	return ScanResult{
		Tracks: tracks,
		Errors: append(walkErrs, parseErrs...),
	}
}

// collectPaths walks the directory tree rooted at root and returns the
// absolute paths of every supported audio file it finds.
func collectPaths(root string) ([]string, []error) {
	var paths []string
	var errs []error

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// Record the error but keep walking.
			errs = append(errs, fmt.Errorf("walk %q: %w", path, err))
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if audio.IsSupportedFile(path) {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		errs = append(errs, fmt.Errorf("walk root %q: %w", root, err))
	}
	return paths, errs
}

// parseTracksParallel distributes paths across numWorkers goroutines,
// each of which reads metadata and produces audio.Track values.
func parseTracksParallel(paths []string, numWorkers int) ([]audio.Track, []error) {
	type result struct {
		track audio.Track
		err   error
	}

	// Buffered channels to feed workers and collect results.
	jobs := make(chan string, len(paths))
	results := make(chan result, len(paths))

	// Start workers.
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				t, err := parseTrack(path)
				results <- result{track: t, err: err}
			}
		}()
	}

	// Enqueue all paths.
	for _, p := range paths {
		jobs <- p
	}
	close(jobs)

	// Wait for workers then close results.
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect.
	var tracks []audio.Track
	var errs []error
	for r := range results {
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		tracks = append(tracks, r.track)
	}
	return tracks, errs
}
