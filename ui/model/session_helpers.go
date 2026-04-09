package model

import (
	"net/url"

	tea "charm.land/bubbletea/v2"

	"cliamp/internal/session"
	"cliamp/internal/source"
	"cliamp/playlist"
)

func (m *Model) clearSourceTracking() {
	m.source = source.Ref{}
	m.pendingSource = source.Ref{}
	m.loadedPlaylist = ""
}

func (m *Model) currentSessionOwnerKey() string {
	if m.source.Valid() {
		return m.source.ProviderKey
	}
	return m.sessionOwnerKey
}

func (m *Model) resumeMetaKey() string {
	key := m.sessionPlanner.ResumeMetaKey(m.source.ProviderKey)
	if key != "" || !m.source.Valid() {
		return key
	}
	return session.InferResumeMetaKey(m.playlist.Tracks())
}

func (m *Model) cancelPendingRestore() {
	if !m.restore.pending() {
		return
	}
	m.restore.token++
	m.restore.plan = session.RestorePlan{}
}

func (m *Model) setPlaylistSource(ref source.Ref) {
	m.source = ref
	if ref.Valid() {
		m.sessionOwnerKey = ref.ProviderKey
	} else {
		m.sessionOwnerKey = ""
	}
	m.pendingSource = source.Ref{}
	if ref.ProviderKey == "local" && ref.Kind == source.Playlist && ref.ID != "" {
		m.loadedPlaylist = ref.ID
		return
	}
	m.loadedPlaylist = ""
}

func (m *Model) replacePlaylistWithSource(tracks []playlist.Track, sourceRef source.Ref) {
	m.cancelPendingRestore()
	m.playlist.Replace(tracks)
	m.plCursor = 0
	m.plScroll = 0
	m.focus = focusPlaylist
	m.setPlaylistSource(sourceRef)
}

func (m *Model) replacePlaylistFromPendingSource(tracks []playlist.Track) {
	m.replacePlaylistWithSource(tracks, m.pendingSource)
}

func (m *Model) replacePlaylistTransient(tracks []playlist.Track) {
	m.replacePlaylistWithSource(tracks, source.Ref{})
}

func (m *Model) appendTransientTracks(tracks []playlist.Track) int {
	start := m.playlist.Len()
	m.cancelPendingRestore()
	m.clearSourceTracking()
	m.playlist.Add(tracks...)
	return start
}

func localPlaylistSource(name string) source.Ref {
	return source.Ref{
		ProviderKey: "local",
		Kind:        source.Playlist,
		ID:          name,
	}
}

func (m *Model) appendTransientTracksToQueue(tracks []playlist.Track) int {
	start := m.appendTransientTracks(tracks)
	for i := start; i < m.playlist.Len(); i++ {
		m.playlist.Queue(i)
	}
	return start
}

func (m *Model) appendTransientTrackToQueue(track playlist.Track) int {
	return m.appendTransientTracksToQueue([]playlist.Track{track})
}

func synthesizeBuiltinRadioSource(providerKey string, track playlist.Track) source.Ref {
	if providerKey != "radio" || !track.Stream || track.Path == "" {
		return source.Ref{}
	}
	u, err := url.Parse(track.Path)
	if err != nil || u.Hostname() != "radio.cliamp.stream" {
		return source.Ref{}
	}
	return source.Ref{
		ProviderKey: providerKey,
		Kind:        source.Playlist,
		ID:          "u:" + url.QueryEscape(track.Path),
	}
}

func (m *Model) currentSessionState() session.State {
	track, idx := m.playlist.Current()
	if idx < 0 || track.Path == "" {
		return session.State{}
	}
	if !m.player.IsPlaying() {
		return session.State{}
	}
	playback := session.CapturePlaylistState(m.resumeMetaKey(), m.playlist)
	source := m.source
	if !source.Valid() {
		source = synthesizeBuiltinRadioSource(m.currentProviderKey(), track)
	}
	state := session.State{
		OwnerKey: m.currentSessionOwnerKey(),
		Playlist: playback,
	}
	if !track.IsLive() {
		state.PositionSec = int(m.player.Position().Seconds())
	}

	if source.Valid() && m.sessionPlanner.CanRestoreSource(source.ProviderKey) {
		state.Source = source
		return state
	}

	state.Tracks = session.CloneTracks(m.playlist.Tracks())
	return state
}

func (m *Model) storeCapturedSessionState(state session.State, tracks []playlist.Track) {
	if !state.HasPersistableState() {
		return
	}
	ownerKey := state.SessionOwnerKey()
	if ownerKey == "" {
		return
	}
	if m.providerSessions == nil {
		m.providerSessions = make(map[string]session.HydratedState)
	}
	m.providerSessions[ownerKey] = session.HydratedState{
		State:  state,
		Tracks: session.CloneTracks(tracks),
	}
}

func (m *Model) captureCurrentProviderState() {
	m.storeCapturedSessionState(m.currentSessionState(), m.playlist.Tracks())
}

func (m *Model) applySavedCurrentTitle(state session.PlaylistState, trackIdx int) {
	if trackIdx < 0 || state.Current.Title == "" {
		return
	}
	track, idx := m.playlist.Current()
	if idx != trackIdx || track.Title == state.Current.Title {
		return
	}
	track.Title = state.Current.Title
	m.playlist.SetTrack(trackIdx, track)
}

func (m *Model) primeRestoredSession(rs session.State, tracks []playlist.Track) error {
	m.player.Stop()
	m.player.ClearPreload()
	m.resetYTDLBatch()
	m.focus = focusPlaylist

	if rs.IsSourceSession() {
		m.setPlaylistSource(rs.Source)
	} else {
		m.clearSourceTracking()
		m.sessionOwnerKey = rs.SessionOwnerKey()
	}

	trackIdx, err := session.RestorePlaylistState(m.playlist, rs.Playlist, tracks)
	if err != nil {
		m.plCursor = 0
		m.plScroll = 0
		return err
	}

	m.plCursor = trackIdx
	m.adjustScroll()
	m.applySavedCurrentTitle(rs.Playlist, trackIdx)

	m.resume.path = tracks[trackIdx].Path
	m.resume.secs = rs.PositionSec
	return nil
}

func (m *Model) applyRestoredTracks(rs session.State, tracks []playlist.Track) tea.Cmd {
	if err := m.primeRestoredSession(rs, tracks); err != nil {
		m.status.Show(err.Error(), statusTTLDefault)
		return nil
	}
	cmd := m.playCurrentTrack()
	m.notifyAll()
	return cmd
}
