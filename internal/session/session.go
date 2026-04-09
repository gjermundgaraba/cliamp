package session

import (
	"errors"

	"cliamp/internal/source"
	"cliamp/playlist"
)

type TrackRef struct {
	Index     int    `json:"index"`
	Path      string `json:"path,omitempty"`
	Title     string `json:"title,omitempty"`
	MetaKey   string `json:"meta_key,omitempty"`
	MetaValue string `json:"meta_value,omitempty"`
}

type PlaylistState struct {
	Current       TrackRef   `json:"current,omitzero"`
	Cursor        TrackRef   `json:"cursor,omitzero"`
	CurrentQueued bool       `json:"current_queued,omitempty"`
	Queue         []TrackRef `json:"queue,omitempty"`
	Order         []TrackRef `json:"order,omitempty"`
}

type State struct {
	OwnerKey    string           `json:"owner_key,omitempty"`
	PositionSec int              `json:"position_sec,omitempty"`
	Source      source.Ref       `json:"source,omitzero"`
	Tracks      []playlist.Track `json:"tracks,omitempty"`
	Playlist    PlaylistState    `json:"playlist,omitzero"`
}

func (s State) SessionOwnerKey() string {
	if s.Source.Valid() {
		return s.Source.ProviderKey
	}
	return s.OwnerKey
}

func (s State) IsSourceSession() bool {
	return s.Source.Valid()
}

func (s State) IsPlaylistSession() bool {
	return !s.Source.Valid() && len(s.Tracks) > 0
}

func (s State) HasPersistableState() bool {
	switch {
	case s.IsSourceSession():
		return true
	case s.IsPlaylistSession():
		return true
	}
	return false
}

var (
	ErrSavedTrackNotFound        = errors.New("saved track not found in restored source")
	ErrSavedQueueContextNotFound = errors.New("saved queue context not found in restored source")
)

func TrackRefFromTrack(metaKey string, t playlist.Track, idx int) TrackRef {
	ref := TrackRef{
		Index: idx,
		Path:  t.Path,
		Title: t.Title,
	}
	if metaKey != "" {
		if v := t.Meta(metaKey); v != "" {
			ref.MetaKey = metaKey
			ref.MetaValue = v
		}
	}
	return ref
}

func InferResumeMetaKey(tracks []playlist.Track) string {
	key := ""
	for _, track := range tracks {
		for candidate, value := range track.ProviderMeta {
			if candidate == "" || value == "" {
				continue
			}
			if key == "" {
				key = candidate
				continue
			}
			if key != candidate {
				return ""
			}
		}
	}
	return key
}

func isDefaultOrder(order []int) bool {
	for i, idx := range order {
		if idx != i {
			return false
		}
	}
	return true
}

func CapturePlaylistState(metaKey string, pl *playlist.Playlist) PlaylistState {
	playback := pl.CapturePlaybackState()
	tracks := pl.Tracks()

	state := PlaylistState{
		CurrentQueued: playback.CurrentQueued,
		Queue:         make([]TrackRef, 0, len(playback.QueueIndices)),
	}
	if playback.CurrentIndex >= 0 && playback.CurrentIndex < len(tracks) {
		state.Current = TrackRefFromTrack(metaKey, tracks[playback.CurrentIndex], playback.CurrentIndex)
	}
	if playback.CursorIndex >= 0 && playback.CursorIndex < len(tracks) {
		state.Cursor = TrackRefFromTrack(metaKey, tracks[playback.CursorIndex], playback.CursorIndex)
	}
	for _, idx := range playback.QueueIndices {
		if idx >= 0 && idx < len(tracks) {
			state.Queue = append(state.Queue, TrackRefFromTrack(metaKey, tracks[idx], idx))
		}
	}
	if !isDefaultOrder(playback.OrderIndices) {
		state.Order = make([]TrackRef, 0, len(playback.OrderIndices))
		for _, idx := range playback.OrderIndices {
			if idx >= 0 && idx < len(tracks) {
				state.Order = append(state.Order, TrackRefFromTrack(metaKey, tracks[idx], idx))
			}
		}
	}
	return state
}

type trackMatcher struct {
	tracks []playlist.Track
	byMeta map[string][]int
	byPath map[string][]int
}

func newTrackMatcher(tracks []playlist.Track) trackMatcher {
	m := trackMatcher{
		tracks: tracks,
		byMeta: make(map[string][]int),
		byPath: make(map[string][]int, len(tracks)),
	}
	for i, track := range tracks {
		if track.Path != "" {
			m.byPath[track.Path] = append(m.byPath[track.Path], i)
		}
		for key, value := range track.ProviderMeta {
			if key == "" || value == "" {
				continue
			}
			m.byMeta[key+"\x00"+value] = append(m.byMeta[key+"\x00"+value], i)
		}
	}
	return m
}

func firstUnused(candidates []int, used []bool) int {
	for _, idx := range candidates {
		if used == nil || !used[idx] {
			return idx
		}
	}
	return -1
}

func (m trackMatcher) match(ref TrackRef, used []bool) int {
	if ref.Index >= 0 && ref.Index < len(m.tracks) && (used == nil || !used[ref.Index]) {
		if ref.matches(m.tracks[ref.Index]) {
			return ref.Index
		}
	}
	if ref.MetaKey != "" && ref.MetaValue != "" {
		if idx := firstUnused(m.byMeta[ref.MetaKey+"\x00"+ref.MetaValue], used); idx >= 0 {
			return idx
		}
	}
	if ref.Path != "" {
		return firstUnused(m.byPath[ref.Path], used)
	}
	return -1
}

func RestorePlaylistState(pl *playlist.Playlist, state PlaylistState, tracks []playlist.Track) (int, error) {
	pl.Replace(tracks)
	matcher := newTrackMatcher(tracks)

	currentIdx := matcher.match(state.Current, nil)
	if currentIdx < 0 {
		return -1, ErrSavedTrackNotFound
	}

	cursorIdx := currentIdx
	if state.CurrentQueued {
		cursorIdx = matcher.match(state.Cursor, nil)
		if cursorIdx < 0 {
			return -1, ErrSavedQueueContextNotFound
		}
	}

	playback := playlist.PlaybackState{
		CurrentIndex:  currentIdx,
		CursorIndex:   cursorIdx,
		CurrentQueued: state.CurrentQueued,
		QueueIndices:  make([]int, 0, len(state.Queue)),
	}
	playback.QueueIndices = matchQueueTrackRefs(state.Queue, matcher)
	if len(state.Order) > 0 {
		playback.OrderIndices = matchTrackRefsUniq(state.Order, matcher)
	}
	if !pl.RestorePlaybackState(playback) {
		return -1, ErrSavedTrackNotFound
	}
	return currentIdx, nil
}

func (r TrackRef) matches(t playlist.Track) bool {
	if r.MetaKey != "" && r.MetaValue != "" && t.Meta(r.MetaKey) == r.MetaValue {
		return true
	}
	return r.Path != "" && t.Path == r.Path
}

func matchTrackRefsUniq(refs []TrackRef, matcher trackMatcher) []int {
	order := make([]int, 0, len(matcher.tracks))
	used := make([]bool, len(matcher.tracks))
	for _, ref := range refs {
		if idx := matcher.match(ref, used); idx >= 0 {
			order = append(order, idx)
			used[idx] = true
		}
	}
	for i := range matcher.tracks {
		if !used[i] {
			order = append(order, i)
		}
	}
	return order
}

func matchQueueTrackRefs(refs []TrackRef, matcher trackMatcher) []int {
	queue := make([]int, 0, len(refs))
	used := make([]bool, len(matcher.tracks))
	repeated := make(map[TrackRef]int, len(refs))
	for _, ref := range refs {
		if idx, ok := repeated[ref]; ok {
			queue = append(queue, idx)
			continue
		}
		idx := matcher.match(ref, used)
		if idx < 0 {
			idx = matcher.match(ref, nil)
		}
		if idx < 0 {
			continue
		}
		queue = append(queue, idx)
		repeated[ref] = idx
		used[idx] = true
	}
	return queue
}

func CloneTracks(tracks []playlist.Track) []playlist.Track {
	if len(tracks) == 0 {
		return nil
	}
	out := make([]playlist.Track, len(tracks))
	for i, track := range tracks {
		out[i] = track
		if len(track.ProviderMeta) > 0 {
			out[i].ProviderMeta = make(map[string]string, len(track.ProviderMeta))
			for key, value := range track.ProviderMeta {
				out[i].ProviderMeta[key] = value
			}
		}
	}
	return out
}
