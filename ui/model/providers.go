package model

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"cliamp/internal/sessionflow"
	"cliamp/internal/source"
	"cliamp/playlist"
	"cliamp/provider"
)

func (m *Model) providerKeyFor(prov playlist.Provider) string {
	if prov == m.provider {
		return m.currentProviderKey()
	}
	for _, pe := range m.providers {
		if pe.Provider == prov {
			return pe.Key
		}
	}
	return ""
}

func (m *Model) currentProviderKey() string {
	if m.activeProviderKey != "" || m.provider == nil {
		return m.activeProviderKey
	}
	for _, pe := range m.providers {
		if pe.Provider == m.provider {
			return pe.Key
		}
	}
	return ""
}

func (m *Model) setPendingProviderPlaylist(idx int) {
	m.cancelPendingRestore()
	sourceID := m.providerLists[idx].SourceID
	if sourceID == "" {
		sourceID = m.providerLists[idx].ID
	}
	m.pendingSource = source.Ref{
		ProviderKey: m.currentProviderKey(),
		Kind:        source.Playlist,
		ID:          sourceID,
	}
}

// resetProviderNav resets provider navigation and search state to the top.
func (m *Model) resetProviderNav() {
	m.provCursor = 0
	m.provScroll = 0
	m.provLoading = true
	m.provSearch.active = false
	m.provSearch.query = ""
	m.provSearch.results = nil
	m.provSearch.cursor = 0
}

// StartInProvider configures the model to begin in the provider browse view.
// Call this from main when no CLI tracks or pending URLs were given.
func (m *Model) StartInProvider() {
	if m.provider != nil {
		m.focus = focusProvider
		m.resetProviderNav()
	}
}

func (m *Model) providerLoadCmd() tea.Cmd {
	if m.restore.pending() {
		if m.currentProviderKey() == m.restore.plan.State.Source.ProviderKey {
			return fetchRestoreTracksCmd(
				m.sessionPlanner,
				m.restore.plan,
				m.restore.token,
			)
		}
		m.restore = pendingRestore{token: m.restore.token}
	}
	return fetchProviderListsCmd(m.provider)
}

func (m *Model) ensureProviderListsLoaded() tea.Cmd {
	if m.provider == nil || m.provLoading || m.providerLists != nil {
		return nil
	}
	m.provLoading = true
	return fetchProviderListsCmd(m.provider)
}

// switchProvider sets the active provider by pill index and fetches its playlists.
func (m *Model) switchProvider(idx int) tea.Cmd {
	if idx < 0 || idx >= len(m.providers) {
		return nil
	}

	targetKey := m.providers[idx].Key
	currentProviderKey := m.currentProviderKey()
	plan := sessionflow.PlanProviderSwitch(m.sessionPlanner, sessionflow.ProviderSwitchInput{
		TargetKey:          targetKey,
		CurrentProviderKey: currentProviderKey,
		CurrentOwnerKey:    m.currentSessionOwnerKey(),
		Sessions:           m.providerSessions,
	})
	if !plan.SelectingCurrent {
		m.captureCurrentProviderState()
	}

	m.cancelPendingRestore()
	m.pendingSource = source.Ref{}

	m.provPillIdx = idx
	m.provider = m.providers[idx].Provider
	m.activeProviderKey = targetKey
	m.provSignIn = false
	m.catalogBatch = catalogBatchState{}

	if !plan.SelectingCurrent {
		switch plan.Action {
		case sessionflow.ProviderSwitchRestoreHydrated:
			m.providerLists = nil
			m.provLoading = false
			cmd := m.applyRestoredTracks(plan.Restore.State, plan.Restore.Tracks)
			return tea.Batch(cmd, m.backgroundRefetchCmd(plan.BackgroundRefetchSource))
		case sessionflow.ProviderSwitchRestoreDeferred:
			m.restore = pendingRestore{
				plan:  plan.Restore,
				token: m.restore.token + 1,
			}
		}
	}

	m.providerLists = nil
	m.resetProviderNav()
	m.focus = focusProvider
	return m.providerLoadCmd()
}

func (m *Model) backgroundRefetchCmd(sourceRef source.Ref) tea.Cmd {
	prov := m.sessionPlanner.Provider(sourceRef.ProviderKey)
	if prov == nil {
		return nil
	}
	sr, ok := prov.(source.Restorer)
	if !ok {
		return nil
	}
	return func() tea.Msg {
		tracks, err := loadProviderTracks(func() ([]playlist.Track, error) {
			return sr.RestoreSource(sourceRef)
		})
		return cachedRestoreRefetchMsg{source: sourceRef, tracks: tracks, err: err}
	}
}

// switchToProvider finds a provider by config key and switches to it.
// Returns nil if the provider is not configured.
func (m *Model) switchToProvider(key string) tea.Cmd {
	if idx, ok := m.providerIndexByKey[key]; ok {
		return m.switchProvider(idx)
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

func (m *Model) openNavBrowserWith(prov playlist.Provider) {
	m.navBrowser.prov = prov
	m.navBrowser.mode = navBrowseModeMenu
	m.navBrowser.screen = navBrowseScreenList
	m.navBrowser.cursor = 0
	m.navBrowser.scroll = 0
	m.navBrowser.artists = nil
	m.navBrowser.albums = nil
	m.navBrowser.tracks = nil
	m.navBrowser.loading = false
	m.navBrowser.albumLoading = false
	m.navBrowser.albumDone = false
	m.navBrowser.searching = false
	m.navBrowser.search = ""
	m.navBrowser.searchIdx = nil
	m.navBrowser.selArtist = provider.ArtistInfo{}
	m.navBrowser.selAlbum = provider.AlbumInfo{}
	if ab, ok := prov.(provider.AlbumBrowser); ok {
		m.navBrowser.sortType = ab.DefaultAlbumSort()
	} else {
		m.navBrowser.sortType = ""
	}
	m.pushScreen(screenNavBrowser)
}

// navUpdateSearch rebuilds navSearchIdx from the current navSearch query
// against whichever list is active on the current nav screen.
func (m *Model) navUpdateSearch() {
	q := strings.ToLower(m.navBrowser.search)
	if q == "" {
		m.navBrowser.searchIdx = nil
		return
	}
	m.navBrowser.searchIdx = nil
	switch {
	case m.navBrowser.mode == navBrowseModeByArtist && m.navBrowser.screen == navBrowseScreenList,
		m.navBrowser.mode == navBrowseModeByArtistAlbum && m.navBrowser.screen == navBrowseScreenList:
		for i, a := range m.navBrowser.artists {
			if strings.Contains(strings.ToLower(a.Name), q) {
				m.navBrowser.searchIdx = append(m.navBrowser.searchIdx, i)
			}
		}
	case m.navBrowser.mode == navBrowseModeByAlbum && m.navBrowser.screen == navBrowseScreenList,
		m.navBrowser.mode == navBrowseModeByArtistAlbum && m.navBrowser.screen == navBrowseScreenAlbums:
		for i, a := range m.navBrowser.albums {
			if strings.Contains(strings.ToLower(a.Name), q) ||
				strings.Contains(strings.ToLower(a.Artist), q) {
				m.navBrowser.searchIdx = append(m.navBrowser.searchIdx, i)
			}
		}
	case m.navBrowser.screen == navBrowseScreenTracks:
		for i, t := range m.navBrowser.tracks {
			if strings.Contains(strings.ToLower(t.Title), q) ||
				strings.Contains(strings.ToLower(t.Artist), q) ||
				strings.Contains(strings.ToLower(t.Album), q) {
				m.navBrowser.searchIdx = append(m.navBrowser.searchIdx, i)
			}
		}
	}
}

// navClearSearch resets the nav search state.
func (m *Model) navClearSearch() {
	m.navBrowser.searching = false
	m.navBrowser.search = ""
	m.navBrowser.searchIdx = nil
	m.navBrowser.cursor = 0
	m.navBrowser.scroll = 0
}

// fetchNavArtistAllTracksCmd first fetches the artist's album list, then fetches
// all tracks across every album. This is used by the "By Artist" browse mode.
// The provider must implement both ArtistBrowser and AlbumTrackLoader.
func (m *Model) fetchNavArtistAllTracksCmd(ab provider.ArtistBrowser, artistID string) tea.Cmd {
	loader, _ := m.navBrowser.prov.(provider.AlbumTrackLoader)
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
