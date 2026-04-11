package model

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"cliamp/playlist"
	"cliamp/provider"
)

// resetProviderNav resets provider navigation and search state to the top.
func (m *Model) resetProviderNav() {
	m.providers.cursor = 0
	m.providers.scroll = 0
	m.providers.loading = true
	m.providers.search.active = false
	m.providers.search.query = ""
	m.providers.search.results = nil
	m.providers.search.cursor = 0
}

// StartInProvider configures the model to begin in the provider browse view.
// Call this from main when no CLI tracks or pending URLs were given.
func (m *Model) StartInProvider() {
	if m.providers.active != nil {
		m.focus = focusProvider
		m.resetProviderNav()
	}
}

// switchProvider sets the active provider by pill index and fetches its playlists.
func (m *Model) switchProvider(idx int) tea.Cmd {
	if idx < 0 || idx >= len(m.providers.entries) {
		return nil
	}
	m.providers.pillIdx = idx
	m.providers.active = m.providers.entries[idx].Provider
	m.providers.lists = nil
	m.providers.signIn = false
	m.providers.catalog = catalogBatchState{}
	m.resetProviderNav()
	m.focus = focusProvider
	return fetchPlaylistsCmd(m.providers.active)
}

// switchToProvider finds a provider by config key and switches to it.
// Returns nil if the provider is not configured.
func (m *Model) switchToProvider(key string) tea.Cmd {
	for i, pe := range m.providers.entries {
		if pe.Key == key {
			return m.switchProvider(i)
		}
	}
	return nil
}

// SetPendingURLs stores remote URLs (feeds, M3U) for async resolution after Init.
func (m *Model) SetPendingURLs(urls []string) {
	m.pendingURLs = urls
	m.feedLoading = len(urls) > 0
}

// findBrowseProvider returns the first provider that supports browsing
// (ArtistBrowser or AlbumBrowser), preferring the active provider.
func (m *Model) findBrowseProvider() playlist.Provider {
	return m.findProviderWith(func(p playlist.Provider) bool {
		if _, ok := p.(provider.ArtistBrowser); ok {
			return true
		}
		_, ok := p.(provider.AlbumBrowser)
		return ok
	})
}

func (m *Model) providerForTrackOwner(track playlist.Track) playlist.Provider {
	if track.Owner.IsZero() {
		return nil
	}
	if m.providers.active != nil && m.providers.pillIdx >= 0 && m.providers.pillIdx < len(m.providers.entries) {
		if m.providers.entries[m.providers.pillIdx].Key == track.Owner.Provider {
			return m.providers.active
		}
	}
	for _, pe := range m.providers.entries {
		if pe.Key == track.Owner.Provider {
			return pe.Provider
		}
	}
	return nil
}

func (m *Model) resolveTrackArtworkRef(ctx context.Context, track playlist.Track) (playlist.ArtworkRef, error) {
	resolver, ok := m.providerForTrackOwner(track).(provider.ArtworkResolver)
	if !ok {
		return playlist.NoArtwork(), nil
	}
	ref, err := resolver.ResolveArtwork(ctx, track)
	return ref, err
}

func (m *Model) openNavBrowserWith(prov playlist.Provider) {
	m.providers.nav.prov = prov
	m.providers.nav.visible = true
	m.providers.nav.mode = navBrowseModeMenu
	m.providers.nav.screen = navBrowseScreenList
	m.providers.nav.cursor = 0
	m.providers.nav.scroll = 0
	m.providers.nav.artists = nil
	m.providers.nav.albums = nil
	m.providers.nav.tracks = nil
	m.providers.nav.loading = false
	m.providers.nav.albumLoading = false
	m.providers.nav.albumDone = false
	m.providers.nav.searching = false
	m.providers.nav.search = ""
	m.providers.nav.searchIdx = nil
	m.providers.nav.selArtist = provider.ArtistInfo{}
	m.providers.nav.selAlbum = provider.AlbumInfo{}
	if ab, ok := prov.(provider.AlbumBrowser); ok {
		m.providers.nav.sortType = ab.DefaultAlbumSort()
	} else {
		m.providers.nav.sortType = ""
	}
}

// navUpdateSearch rebuilds navSearchIdx from the current navSearch query
// against whichever list is active on the current nav screen.
func (m *Model) navUpdateSearch() {
	q := strings.ToLower(m.providers.nav.search)
	if q == "" {
		m.providers.nav.searchIdx = nil
		return
	}
	m.providers.nav.searchIdx = nil
	switch {
	case m.providers.nav.mode == navBrowseModeByArtist && m.providers.nav.screen == navBrowseScreenList,
		m.providers.nav.mode == navBrowseModeByArtistAlbum && m.providers.nav.screen == navBrowseScreenList:
		for i, a := range m.providers.nav.artists {
			if strings.Contains(strings.ToLower(a.Name), q) {
				m.providers.nav.searchIdx = append(m.providers.nav.searchIdx, i)
			}
		}
	case m.providers.nav.mode == navBrowseModeByAlbum && m.providers.nav.screen == navBrowseScreenList,
		m.providers.nav.mode == navBrowseModeByArtistAlbum && m.providers.nav.screen == navBrowseScreenAlbums:
		for i, a := range m.providers.nav.albums {
			if strings.Contains(strings.ToLower(a.Name), q) ||
				strings.Contains(strings.ToLower(a.Artist), q) {
				m.providers.nav.searchIdx = append(m.providers.nav.searchIdx, i)
			}
		}
	case m.providers.nav.screen == navBrowseScreenTracks:
		for i, t := range m.providers.nav.tracks {
			if strings.Contains(strings.ToLower(t.Title), q) ||
				strings.Contains(strings.ToLower(t.Artist), q) ||
				strings.Contains(strings.ToLower(t.Album), q) {
				m.providers.nav.searchIdx = append(m.providers.nav.searchIdx, i)
			}
		}
	}
}

// navClearSearch resets the nav search state.
func (m *Model) navClearSearch() {
	m.providers.nav.searching = false
	m.providers.nav.search = ""
	m.providers.nav.searchIdx = nil
	m.providers.nav.cursor = 0
	m.providers.nav.scroll = 0
}

// fetchNavArtistAllTracksCmd first fetches the artist's album list, then fetches
// all tracks across every album. This is used by the "By Artist" browse mode.
// The provider must implement both ArtistBrowser and AlbumTrackLoader.
func (m *Model) fetchNavArtistAllTracksCmd(ab provider.ArtistBrowser, artistID string) tea.Cmd {
	loader, _ := m.providers.nav.prov.(provider.AlbumTrackLoader)
	return func() tea.Msg {
		albums, err := ab.ArtistAlbums(artistID)
		if err != nil {
			return err
		}
		if loader == nil {
			return navTracksLoadedMsg(nil)
		}
		var all []playlist.Track
		for _, album := range albums {
			tracks, err := loader.AlbumTracks(album.ID)
			if err != nil {
				return err
			}
			all = append(all, tracks...)
		}
		return navTracksLoadedMsg(all)
	}
}
