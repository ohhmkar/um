package audio

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/flac"
	"github.com/gopxl/beep/v2/mp3"
	"github.com/gopxl/beep/v2/vorbis"
	"github.com/gopxl/beep/v2/wav"
)

// supportedExtensions lists the file extensions µm can decode.
var supportedExtensions = map[string]bool{
	".mp3":  true,
	".flac": true,
	".wav":  true,
	".ogg":  true,
}

// IsSupportedFile reports whether the given path has a supported audio
// file extension.
func IsSupportedFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return supportedExtensions[ext]
}

// decodeResult bundles everything returned by a successful decode so it
// can be shuttled around as a single value.
type decodeResult struct {
	streamer beep.StreamSeekCloser
	format   beep.Format
}

// decodeFile opens and decodes an audio file, selecting the decoder based
// on the file extension. On success the caller owns the returned streamer
// and must call streamer.Close() when done (which also closes the
// underlying os.File).
//
// Errors are returned as values — this function never panics.
func decodeFile(path string) (decodeResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return decodeResult{}, fmt.Errorf("open %q: %w", path, err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	var (
		streamer beep.StreamSeekCloser
		format   beep.Format
		decErr   error
	)

	switch ext {
	case ".mp3":
		streamer, format, decErr = mp3.Decode(f)
	case ".flac":
		streamer, format, decErr = flac.Decode(f)
	case ".wav":
		streamer, format, decErr = wav.Decode(f)
	case ".ogg":
		streamer, format, decErr = vorbis.Decode(f)
	default:
		f.Close()
		return decodeResult{}, fmt.Errorf("unsupported audio format: %s", ext)
	}

	if decErr != nil {
		// Close the file on decode failure to avoid leaking the fd.
		f.Close()
		return decodeResult{}, fmt.Errorf("decode %q: %w", path, decErr)
	}

	return decodeResult{streamer: streamer, format: format}, nil
}
