package session

import (
	"reflect"
	"testing"

	"cliamp/internal/source"
	"cliamp/playlist"
)

const (
	navidromeIDKey = "navidrome.id"
)

func TestStateHasPersistableState(t *testing.T) {
	source := State{
		Source: source.Ref{
			ProviderKey: "navidrome",
			Kind:        source.Playlist,
			ID:          "saved-id",
		},
	}
	if !source.HasPersistableState() {
		t.Fatal("source session should be persistable")
	}

	playlistState := State{
		Tracks: []playlist.Track{
			{Path: "/saved.mp3", Title: "Saved"},
		},
	}
	if !playlistState.HasPersistableState() {
		t.Fatal("playlist session should be persistable")
	}

	if (State{}).HasPersistableState() {
		t.Fatal("empty state should not be persistable")
	}
}

func TestCloneTracks(t *testing.T) {
	tracks := []playlist.Track{{
		Path:         "/saved.mp3",
		Title:        "Saved",
		ProviderMeta: map[string]string{"navidrome.id": "abc"},
	}}

	cloned := CloneTracks(tracks)
	if !reflect.DeepEqual(cloned, tracks) {
		t.Fatalf("CloneTracks() = %+v, want %+v", cloned, tracks)
	}

	cloned[0].ProviderMeta["navidrome.id"] = "mutated"
	if tracks[0].ProviderMeta["navidrome.id"] != "abc" {
		t.Fatalf("CloneTracks() mutated original provider meta: %+v", tracks[0].ProviderMeta)
	}
}

func TestTrackRefFromTrack(t *testing.T) {
	tests := []struct {
		name    string
		metaKey string
		track   playlist.Track
		idx     int
		want    TrackRef
	}{
		{
			name:    "captures provider metadata",
			metaKey: navidromeIDKey,
			track: playlist.Track{
				Path:         "/music/song.mp3",
				ProviderMeta: map[string]string{navidromeIDKey: "nd-42"},
			},
			idx:  7,
			want: TrackRef{Index: 7, Path: "/music/song.mp3", MetaKey: navidromeIDKey, MetaValue: "nd-42"},
		},
		{
			name:  "omits metadata when metaKey is empty",
			track: playlist.Track{Path: "/local/file.mp3"},
			idx:   3,
			want:  TrackRef{Index: 3, Path: "/local/file.mp3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TrackRefFromTrack(tt.metaKey, tt.track, tt.idx); got != tt.want {
				t.Fatalf("TrackRefFromTrack() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRestorePlaylistStatePreservesUnderlyingCursorForQueuedCurrent(t *testing.T) {
	tracks := []playlist.Track{
		{Title: "A", Path: "/a.mp3"},
		{Title: "B", Path: "/b.mp3"},
		{Title: "C", Path: "/c.mp3"},
		{Title: "D", Path: "/d.mp3"},
	}

	pl := playlist.New()
	currentIdx, err := RestorePlaylistState(pl, PlaylistState{
		Current:       TrackRefFromTrack("", tracks[3], 3),
		Cursor:        TrackRefFromTrack("", tracks[0], 0),
		CurrentQueued: true,
	}, tracks)
	if err != nil {
		t.Fatalf("RestorePlaylistState() error = %v", err)
	}
	if currentIdx != 3 {
		t.Fatalf("currentIdx = %d, want 3", currentIdx)
	}

	current, idx := pl.Current()
	if current.Title != "D" || idx != 3 {
		t.Fatalf("Current() = (%q, %d), want (D, 3)", current.Title, idx)
	}

	next, ok := pl.Next()
	if !ok || next.Title != "B" {
		t.Fatalf("Next() after queued-current restore = (%q, %v), want (B, true)", next.Title, ok)
	}
}

func TestCapturePlaylistStateUsesPlaylistIndices(t *testing.T) {
	tracks := []playlist.Track{
		{Title: "dup-1", Path: "/dup.mp3"},
		{Title: "x", Path: "/x.mp3"},
		{Title: "dup-2", Path: "/dup.mp3"},
	}

	pl := playlist.New()
	pl.Add(tracks...)
	pl.Queue(2)

	state := CapturePlaylistState("", pl)
	if len(state.Queue) != 1 {
		t.Fatalf("CapturePlaylistState() queue len = %d, want 1", len(state.Queue))
	}
	if state.Queue[0].Index != 2 {
		t.Fatalf("CapturePlaylistState() queue[0].Index = %d, want 2", state.Queue[0].Index)
	}
}

func TestRestorePlaylistStatePreservesPlaybackOrder(t *testing.T) {
	tracks := []playlist.Track{
		{Title: "A", Path: "/a.mp3"},
		{Title: "B", Path: "/b.mp3"},
		{Title: "C", Path: "/c.mp3"},
		{Title: "D", Path: "/d.mp3"},
	}

	pl := playlist.New()
	pl.ToggleShuffle()

	currentIdx, err := RestorePlaylistState(pl, PlaylistState{
		Current: TrackRefFromTrack("", tracks[2], 2),
		Cursor:  TrackRefFromTrack("", tracks[2], 2),
		Order: []TrackRef{
			TrackRefFromTrack("", tracks[2], 2),
			TrackRefFromTrack("", tracks[0], 0),
			TrackRefFromTrack("", tracks[3], 3),
			TrackRefFromTrack("", tracks[1], 1),
		},
	}, tracks)
	if err != nil {
		t.Fatalf("RestorePlaylistState() error = %v", err)
	}
	if currentIdx != 2 {
		t.Fatalf("currentIdx = %d, want 2", currentIdx)
	}

	for _, want := range []string{"A", "D", "B"} {
		next, ok := pl.Next()
		if !ok || next.Title != want {
			t.Fatalf("Next() = (%q, %v), want (%q, true)", next.Title, ok, want)
		}
	}
}

func TestRestorePlaylistStateMatchesChangedPathsByMeta(t *testing.T) {
	oldRef := func(idx int, title, id string) TrackRef {
		return TrackRef{
			Index:     idx,
			Path:      "https://nav.example/rest/stream?id=" + id + "&s=old&t=old",
			Title:     title,
			MetaKey:   navidromeIDKey,
			MetaValue: id,
		}
	}

	restoredTracks := []playlist.Track{
		{
			Title: "A",
			Path:  "https://nav.example/rest/stream?id=song-1&s=new&t=new",
			ProviderMeta: map[string]string{
				navidromeIDKey: "song-1",
			},
		},
		{
			Title: "B",
			Path:  "https://nav.example/rest/stream?id=song-2&s=new&t=new",
			ProviderMeta: map[string]string{
				navidromeIDKey: "song-2",
			},
		},
		{
			Title: "C",
			Path:  "https://nav.example/rest/stream?id=song-3&s=new&t=new",
			ProviderMeta: map[string]string{
				navidromeIDKey: "song-3",
			},
		},
	}

	pl := playlist.New()
	pl.ToggleShuffle()

	currentIdx, err := RestorePlaylistState(pl, PlaylistState{
		Current: oldRef(1, "B", "song-2"),
		Cursor:  oldRef(1, "B", "song-2"),
		Queue: []TrackRef{
			oldRef(0, "A", "song-1"),
		},
		Order: []TrackRef{
			oldRef(1, "B", "song-2"),
			oldRef(2, "C", "song-3"),
			oldRef(0, "A", "song-1"),
		},
	}, restoredTracks)
	if err != nil {
		t.Fatalf("RestorePlaylistState() error = %v", err)
	}
	if currentIdx != 1 {
		t.Fatalf("currentIdx = %d, want 1", currentIdx)
	}

	current, idx := pl.Current()
	if idx != 1 || current.Title != "B" {
		t.Fatalf("Current() = (%q, %d), want (B, 1)", current.Title, idx)
	}

	queue := pl.QueueTracks()
	if len(queue) != 1 || queue[0].Title != "A" {
		t.Fatalf("QueueTracks() = %+v, want [A]", queue)
	}

	next, ok := pl.Next()
	if !ok || next.Title != "A" {
		t.Fatalf("first Next() = (%q, %v), want (A, true)", next.Title, ok)
	}

	next, ok = pl.Next()
	if !ok || next.Title != "C" {
		t.Fatalf("second Next() = (%q, %v), want (C, true)", next.Title, ok)
	}
}

func TestRestorePlaylistStateMatchesDuplicateOrderRefsUniquely(t *testing.T) {
	savedOrder := PlaylistState{
		Current: TrackRef{Index: 2, Path: "/dup.mp3"},
		Cursor:  TrackRef{Index: 2, Path: "/dup.mp3"},
		Order: []TrackRef{
			{Index: 2, Path: "/dup.mp3"},
			{Index: 0, Path: "/dup.mp3"},
			{Index: 1, Path: "/x.mp3"},
		},
	}
	restoredTracks := []playlist.Track{
		{Title: "dup-1", Path: "/dup.mp3"},
		{Title: "dup-2", Path: "/dup.mp3"},
		{Title: "x", Path: "/x.mp3"},
	}

	pl := playlist.New()
	currentIdx, err := RestorePlaylistState(pl, savedOrder, restoredTracks)
	if err != nil {
		t.Fatalf("RestorePlaylistState() error = %v", err)
	}
	if currentIdx != 0 {
		t.Fatalf("currentIdx = %d, want 0", currentIdx)
	}

	current, idx := pl.Current()
	if idx != 0 || current.Title != "dup-1" {
		t.Fatalf("Current() = (%q, %d), want (dup-1, 0)", current.Title, idx)
	}

	next, ok := pl.Next()
	if !ok || next.Title != "dup-2" {
		t.Fatalf("first Next() = (%q, %v), want (dup-2, true)", next.Title, ok)
	}

	next, ok = pl.Next()
	if !ok || next.Title != "x" {
		t.Fatalf("second Next() = (%q, %v), want (x, true)", next.Title, ok)
	}
}

func TestRestorePlaylistStateMatchesDuplicateQueueRefsUniquely(t *testing.T) {
	savedState := PlaylistState{
		Current: TrackRef{Index: 1, Path: "/x.mp3"},
		Cursor:  TrackRef{Index: 1, Path: "/x.mp3"},
		Queue: []TrackRef{
			{Index: 2, Path: "/dup.mp3"},
			{Index: 0, Path: "/dup.mp3"},
		},
	}
	restoredTracks := []playlist.Track{
		{Title: "dup-1", Path: "/dup.mp3"},
		{Title: "dup-2", Path: "/dup.mp3"},
		{Title: "x", Path: "/x.mp3"},
	}

	pl := playlist.New()
	currentIdx, err := RestorePlaylistState(pl, savedState, restoredTracks)
	if err != nil {
		t.Fatalf("RestorePlaylistState() error = %v", err)
	}
	if currentIdx != 2 {
		t.Fatalf("currentIdx = %d, want 2", currentIdx)
	}

	queue := pl.QueueTracks()
	if len(queue) != 2 {
		t.Fatalf("QueueTracks() len = %d, want 2", len(queue))
	}
	if queue[0].Title != "dup-1" || queue[1].Title != "dup-2" {
		t.Fatalf("QueueTracks() = [%s %s], want [dup-1 dup-2]", queue[0].Title, queue[1].Title)
	}
}

func TestRestorePlaylistStatePreservesRepeatedIdenticalQueueRefs(t *testing.T) {
	savedState := PlaylistState{
		Current: TrackRef{Index: 0, Path: "/a.mp3"},
		Cursor:  TrackRef{Index: 0, Path: "/a.mp3"},
		Queue: []TrackRef{
			{Index: 1, Path: "/b.mp3"},
			{Index: 1, Path: "/b.mp3"},
		},
	}
	tracks := []playlist.Track{
		{Title: "A", Path: "/a.mp3"},
		{Title: "B", Path: "/b.mp3"},
		{Title: "C", Path: "/c.mp3"},
	}

	pl := playlist.New()
	if _, err := RestorePlaylistState(pl, savedState, tracks); err != nil {
		t.Fatalf("RestorePlaylistState() error = %v", err)
	}

	queue := pl.QueueTracks()
	if len(queue) != 2 {
		t.Fatalf("QueueTracks() len = %d, want 2", len(queue))
	}
	if queue[0].Title != "B" || queue[1].Title != "B" {
		t.Fatalf("QueueTracks() = [%s %s], want [B B]", queue[0].Title, queue[1].Title)
	}
}
