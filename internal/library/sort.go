package library

import (
	"sort"
	"strings"

	"github.com/ohhmkar/um/internal/audio"
)

// SortByPath sorts tracks lexicographically by their file path. This
// produces a natural directory-grouped ordering.
func SortByPath(tracks []audio.Track) {
	sort.Slice(tracks, func(i, j int) bool {
		return tracks[i].Path < tracks[j].Path
	})
}

// SortByAlbumTrack sorts tracks first by artist (case-insensitive),
// then by album, then by track number. Tracks without metadata sort
// to the end within each group.
func SortByAlbumTrack(tracks []audio.Track) {
	sort.SliceStable(tracks, func(i, j int) bool {
		ai, aj := strings.ToLower(tracks[i].Artist), strings.ToLower(tracks[j].Artist)
		if ai != aj {
			return ai < aj
		}
		bi, bj := strings.ToLower(tracks[i].Album), strings.ToLower(tracks[j].Album)
		if bi != bj {
			return bi < bj
		}
		return tracks[i].TrackNumber < tracks[j].TrackNumber
	})
}
