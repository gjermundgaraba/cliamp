package model

import (
	"cliamp/internal/artwork"
	"cliamp/playlist"

	tea "charm.land/bubbletea/v2"
)

type artworkSession struct {
	index int
	gen   uint64
	path  string
}

func (s *artworkSession) Activate(index int) uint64 {
	s.index = index
	s.gen++
	s.path = ""
	return s.gen
}

func (s *artworkSession) Clear() {
	s.index = -1
	s.gen++
	s.path = ""
}

func (s *artworkSession) Rebind(index int) uint64 {
	s.index = index
	s.gen++
	return s.gen
}

func (s *artworkSession) Apply(msg artworkResolvedMsg) bool {
	if msg.index != s.index || msg.gen != s.gen {
		return false
	}
	s.path = msg.path
	return true
}

func (m *Model) replacePlaylist(tracks []playlist.Track) {
	m.playlist.Replace(tracks)
	m.artwork.session.Clear()
}

func (m *Model) moveTrack(from, to int) (bool, tea.Cmd) {
	current := m.playlist.Index()
	trackedCurrent := m.artwork.session.gen > 0 && m.artwork.session.index == current

	if !m.playlist.Move(from, to) {
		return false, nil
	}

	nextCurrent := m.playlist.Index()
	if !trackedCurrent || current == nextCurrent {
		return true, nil
	}

	gen := m.artwork.session.Rebind(nextCurrent)
	if m.artwork.session.path != "" || m.artwork.materializer == nil {
		return true, nil
	}

	track, _ := m.playlist.Current()
	return true, resolveArtworkCmd(m.artwork.materializer, m.resolveTrackArtworkRef, nextCurrent, gen, track)
}

func (m *Model) ensureArtworkMaterializer() {
	if m.artwork.materializer == nil {
		m.artwork.materializer = artwork.NewMaterializer()
	}
}

func (m *Model) refreshCurrentArtwork() tea.Cmd {
	if m.artwork.materializer == nil {
		return nil
	}
	track, idx := m.playlist.Current()
	if idx < 0 {
		m.artwork.session.Clear()
		return nil
	}

	gen := m.artwork.session.Activate(idx)
	return resolveArtworkCmd(m.artwork.materializer, m.resolveTrackArtworkRef, idx, gen, track)
}

func (m *Model) applyArtworkResolved(msg artworkResolvedMsg) {
	if !m.artwork.session.Apply(msg) {
		return
	}
	if msg.err == nil && !msg.ref.IsNone() {
		tracks := m.playlist.Tracks()
		if msg.index >= 0 && msg.index < len(tracks) && tracks[msg.index].Artwork.IsNone() {
			track := tracks[msg.index]
			track.Artwork = msg.ref
			m.playlist.SetTrack(msg.index, track)
		}
	}
	if msg.err != nil {
		return
	}
	if msg.path != "" {
		m.notifyPlayback()
	}
}
