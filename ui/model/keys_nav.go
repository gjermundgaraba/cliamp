package model

import (
	tea "charm.land/bubbletea/v2"

	"cliamp/playlist"
	"cliamp/provider"
)

// handleNavBrowserKey processes key presses while the provider browser is open.
func (m *Model) handleNavBrowserKey(msg tea.KeyPressMsg) tea.Cmd {
	if m.providers.nav.prov == nil {
		m.providers.nav.visible = false
		return nil
	}

	// Search bar: active on any list/track screen (not the mode menu).
	if m.providers.nav.mode != navBrowseModeMenu {
		if m.providers.nav.searching {
			return m.handleNavSearchKey(msg)
		}
		if msg.String() == "/" {
			// Toggle: if already filtered, clear; otherwise open.
			if m.providers.nav.search != "" {
				m.navClearSearch()
			} else {
				m.providers.nav.searching = true
			}
			return nil
		}
	}

	switch m.providers.nav.mode {
	case navBrowseModeMenu:
		return m.handleNavMenuKey(msg)
	case navBrowseModeByAlbum:
		return m.handleNavByAlbumKey(msg)
	case navBrowseModeByArtist:
		return m.handleNavByArtistKey(msg)
	case navBrowseModeByArtistAlbum:
		return m.handleNavByArtistAlbumKey(msg)
	}
	return nil
}

func (m *Model) handleNavMenuKey(msg tea.KeyPressMsg) tea.Cmd {
	const menuItems = 3
	switch msg.String() {
	case "ctrl+c":
		m.providers.nav.visible = false
		return m.quit()
	case "up", "k":
		if m.providers.nav.cursor > 0 {
			m.providers.nav.cursor--
		}
	case "down", "j":
		if m.providers.nav.cursor < menuItems-1 {
			m.providers.nav.cursor++
		}
	case "enter", "l", "right":
		switch m.providers.nav.cursor {
		case 0: // By Album
			ab, ok := m.providers.nav.prov.(provider.AlbumBrowser)
			if !ok {
				return nil
			}
			m.providers.nav.mode = navBrowseModeByAlbum
			m.providers.nav.screen = navBrowseScreenList
			m.providers.nav.cursor = 0
			m.providers.nav.scroll = 0
			m.providers.nav.albums = nil
			m.providers.nav.albumLoading = true
			m.providers.nav.albumDone = false
			m.providers.nav.loading = false
			return fetchNavAlbumListCmd(ab, m.providers.nav.sortType, 0)
		case 1: // By Artist
			ab, ok := m.providers.nav.prov.(provider.ArtistBrowser)
			if !ok {
				return nil
			}
			m.providers.nav.mode = navBrowseModeByArtist
			m.providers.nav.screen = navBrowseScreenList
			m.providers.nav.cursor = 0
			m.providers.nav.scroll = 0
			m.providers.nav.artists = nil
			m.providers.nav.loading = true
			return fetchNavArtistsCmd(ab)
		case 2: // By Artist / Album
			ab, ok := m.providers.nav.prov.(provider.ArtistBrowser)
			if !ok {
				return nil
			}
			m.providers.nav.mode = navBrowseModeByArtistAlbum
			m.providers.nav.screen = navBrowseScreenList
			m.providers.nav.cursor = 0
			m.providers.nav.scroll = 0
			m.providers.nav.artists = nil
			m.providers.nav.loading = true
			return fetchNavArtistsCmd(ab)
		}
	case "esc", "N", "backspace", "b":
		m.providers.nav.visible = false
	}
	return nil
}

func (m *Model) handleNavByAlbumKey(msg tea.KeyPressMsg) tea.Cmd {
	switch m.providers.nav.screen {
	case navBrowseScreenList:
		return m.handleNavAlbumListKey(msg, false)
	case navBrowseScreenTracks:
		return m.handleNavTrackListKey(msg)
	}
	return nil
}

func (m *Model) handleNavByArtistKey(msg tea.KeyPressMsg) tea.Cmd {
	switch m.providers.nav.screen {
	case navBrowseScreenList:
		return m.handleNavArtistListKey(msg)
	case navBrowseScreenTracks:
		return m.handleNavTrackListKey(msg)
	}
	return nil
}

func (m *Model) handleNavByArtistAlbumKey(msg tea.KeyPressMsg) tea.Cmd {
	switch m.providers.nav.screen {
	case navBrowseScreenList:
		return m.handleNavArtistListKey(msg)
	case navBrowseScreenAlbums:
		return m.handleNavAlbumListKey(msg, true)
	case navBrowseScreenTracks:
		return m.handleNavTrackListKey(msg)
	}
	return nil
}

// handleNavArtistListKey handles the artist list screen.
func (m *Model) handleNavArtistListKey(msg tea.KeyPressMsg) tea.Cmd {
	// Determine effective list length (filtered or full).
	listLen := len(m.providers.nav.artists)
	if len(m.providers.nav.searchIdx) > 0 {
		listLen = len(m.providers.nav.searchIdx)
	}

	switch msg.String() {
	case "ctrl+c":
		m.providers.nav.visible = false
		return m.quit()
	case "up", "k":
		if m.providers.nav.cursor > 0 {
			m.providers.nav.cursor--
			m.navMaybeAdjustScroll()
		}
	case "down", "j":
		if m.providers.nav.cursor < listLen-1 {
			m.providers.nav.cursor++
			m.navMaybeAdjustScroll()
		}
	case "enter", "l", "right":
		if m.providers.nav.loading || len(m.providers.nav.artists) == 0 {
			return nil
		}
		ab, ok := m.providers.nav.prov.(provider.ArtistBrowser)
		if !ok {
			return nil
		}
		// Resolve raw index (filtered or direct).
		rawIdx := m.providers.nav.cursor
		if len(m.providers.nav.searchIdx) > 0 && m.providers.nav.cursor < len(m.providers.nav.searchIdx) {
			rawIdx = m.providers.nav.searchIdx[m.providers.nav.cursor]
		}
		artist := m.providers.nav.artists[rawIdx]
		m.providers.nav.selArtist = artist
		m.providers.nav.loading = true
		if m.providers.nav.mode == navBrowseModeByArtistAlbum {
			// Drill into album list for this artist.
			m.providers.nav.albums = nil
			m.providers.nav.albumLoading = false
			m.providers.nav.screen = navBrowseScreenAlbums
			m.providers.nav.cursor = 0
			m.providers.nav.scroll = 0
			m.navClearSearch()
			return fetchNavArtistAlbumsCmd(ab, artist.ID)
		}
		m.navClearSearch()
		return m.fetchNavArtistAllTracksCmd(ab, artist.ID)
	case "esc", "h", "left", "backspace":
		// Back to menu.
		m.navClearSearch()
		m.providers.nav.mode = navBrowseModeMenu
		m.providers.nav.screen = navBrowseScreenList
	}
	return nil
}

// handleNavAlbumListKey handles the album list screen.
// artistAlbums=true means this is the artist's album sub-screen (ArtistAlbum mode), not the global list.
func (m *Model) handleNavAlbumListKey(msg tea.KeyPressMsg, artistAlbums bool) tea.Cmd {
	// Determine effective list length (filtered or full).
	listLen := len(m.providers.nav.albums)
	if len(m.providers.nav.searchIdx) > 0 {
		listLen = len(m.providers.nav.searchIdx)
	}

	switch msg.String() {
	case "ctrl+c":
		m.providers.nav.visible = false
		return m.quit()
	case "up", "k":
		if m.providers.nav.cursor > 0 {
			m.providers.nav.cursor--
			m.navMaybeAdjustScroll()
		}
	case "down", "j":
		if m.providers.nav.cursor < listLen-1 {
			m.providers.nav.cursor++
			m.navMaybeAdjustScroll()
			// Lazy-load next page: only trigger on the raw (unfiltered) list.
			if !artistAlbums && len(m.providers.nav.searchIdx) == 0 && !m.providers.nav.albumLoading && !m.providers.nav.albumDone && m.providers.nav.cursor >= len(m.providers.nav.albums)-10 {
				if ab, ok := m.providers.nav.prov.(provider.AlbumBrowser); ok {
					m.providers.nav.albumLoading = true
					return fetchNavAlbumListCmd(ab, m.providers.nav.sortType, len(m.providers.nav.albums))
				}
			}
		}
	case "enter", "l", "right":
		if (m.providers.nav.loading && !artistAlbums) || len(m.providers.nav.albums) == 0 {
			return nil
		}
		// Resolve raw index (filtered or direct).
		rawIdx := m.providers.nav.cursor
		if len(m.providers.nav.searchIdx) > 0 && m.providers.nav.cursor < len(m.providers.nav.searchIdx) {
			rawIdx = m.providers.nav.searchIdx[m.providers.nav.cursor]
		}
		album := m.providers.nav.albums[rawIdx]
		m.providers.nav.selAlbum = album
		m.providers.nav.loading = true
		m.navClearSearch()
		if l, ok := m.providers.nav.prov.(provider.AlbumTrackLoader); ok {
			return fetchNavAlbumTracksCmd(l, album.ID)
		}
		return nil
	case "s":
		if artistAlbums {
			return nil // Sort only applies to global album list.
		}
		ab, ok := m.providers.nav.prov.(provider.AlbumBrowser)
		if !ok {
			return nil
		}
		m.providers.nav.sortType = navNextSort(m.providers.nav.sortType, ab.AlbumSortTypes())
		m.providers.nav.albums = nil
		m.providers.nav.cursor = 0
		m.providers.nav.scroll = 0
		m.providers.nav.albumLoading = true
		m.providers.nav.albumDone = false
		m.navClearSearch()
		if saver, ok := m.providers.nav.prov.(provider.AlbumSortSaver); ok {
			if err := saver.SaveAlbumSort(m.providers.nav.sortType); err != nil {
				m.status.Showf(statusTTLDefault, "Sort save failed: %s", err)
			}
		}
		return fetchNavAlbumListCmd(ab, m.providers.nav.sortType, 0)
	case "esc", "h", "left", "backspace":
		m.navClearSearch()
		if artistAlbums {
			// Back to artist list.
			m.providers.nav.screen = navBrowseScreenList
		} else {
			// Back to menu.
			m.providers.nav.mode = navBrowseModeMenu
			m.providers.nav.screen = navBrowseScreenList
		}
	}
	return nil
}

// handleNavTrackListKey handles the final track-list screen (used by all modes).
func (m *Model) handleNavTrackListKey(msg tea.KeyPressMsg) tea.Cmd {
	// Determine effective list length (filtered or full).
	listLen := len(m.providers.nav.tracks)
	if len(m.providers.nav.searchIdx) > 0 {
		listLen = len(m.providers.nav.searchIdx)
	}

	switch msg.String() {
	case "ctrl+c":
		m.providers.nav.visible = false
		return m.quit()
	case "up", "k":
		if m.providers.nav.cursor > 0 {
			m.providers.nav.cursor--
			m.navMaybeAdjustScroll()
		}
	case "down", "j":
		if m.providers.nav.cursor < listLen-1 {
			m.providers.nav.cursor++
			m.navMaybeAdjustScroll()
		}
	case "enter":
		// Play the selected track immediately, then enqueue everything from that
		// position to the end of the list (capped at 500 total tracks added).
		if len(m.providers.nav.tracks) == 0 {
			return nil
		}
		rawIdx := m.providers.nav.cursor
		if len(m.providers.nav.searchIdx) > 0 && m.providers.nav.cursor < len(m.providers.nav.searchIdx) {
			rawIdx = m.providers.nav.searchIdx[m.providers.nav.cursor]
		}
		if rawIdx < len(m.providers.nav.tracks) {
			const maxAdd = 500
			m.player.Stop()
			m.player.ClearPreload()

			// Build the slice of tracks to add: from rawIdx to end (or 500 max).
			var toAdd []playlist.Track
			if len(m.providers.nav.searchIdx) > 0 {
				// Filtered: use positions from navCursor onward in the filtered list.
				for j := m.providers.nav.cursor; j < len(m.providers.nav.searchIdx) && len(toAdd) < maxAdd; j++ {
					toAdd = append(toAdd, m.providers.nav.tracks[m.providers.nav.searchIdx[j]])
				}
			} else {
				for i := rawIdx; i < len(m.providers.nav.tracks) && len(toAdd) < maxAdd; i++ {
					toAdd = append(toAdd, m.providers.nav.tracks[i])
				}
			}

			m.playlist.Add(toAdd...)
			newIdx := m.playlist.Len() - len(toAdd)
			m.playlist.SetIndex(newIdx)
			m.plCursor = newIdx
			m.adjustScroll()
			if len(toAdd) > 1 {
				m.status.Showf(statusTTLMedium, "Playing: %s (+%d queued)", toAdd[0].DisplayName(), len(toAdd)-1)
			} else {
				m.status.Showf(statusTTLMedium, "Playing: %s", toAdd[0].DisplayName())
			}
			cmd := m.playCurrentTrack()
			m.notifyPlayback()
			return cmd
		}
	case "R":
		// Replace playlist with all displayed tracks and close browser.
		tracks := m.providers.nav.tracks
		if len(m.providers.nav.searchIdx) > 0 {
			// Replace with only the filtered subset.
			filtered := make([]playlist.Track, 0, len(m.providers.nav.searchIdx))
			for _, i := range m.providers.nav.searchIdx {
				filtered = append(filtered, m.providers.nav.tracks[i])
			}
			tracks = filtered
		}
		if len(tracks) > 0 {
			m.player.Stop()
			m.player.ClearPreload()
			m.resetYTDLBatch()
			m.replacePlaylist(tracks)
			m.plCursor = 0
			m.plScroll = 0
			m.playlist.SetIndex(0)
			m.focus = focusPlaylist
			m.providers.nav.visible = false
			cmd := m.playCurrentTrack()
			m.notifyPlayback()
			return cmd
		}
	case "a":
		// Append all displayed tracks to the playlist (keep current playback).
		tracks := m.providers.nav.tracks
		if len(m.providers.nav.searchIdx) > 0 {
			filtered := make([]playlist.Track, 0, len(m.providers.nav.searchIdx))
			for _, i := range m.providers.nav.searchIdx {
				filtered = append(filtered, m.providers.nav.tracks[i])
			}
			tracks = filtered
		}
		if len(tracks) > 0 {
			wasEmpty := m.playlist.Len() == 0
			m.playlist.Add(tracks...)
			m.status.Showf(statusTTLMedium, "Added %d tracks", len(tracks))
			if wasEmpty || !m.player.IsPlaying() {
				m.playlist.SetIndex(0)
				cmd := m.playCurrentTrack()
				m.notifyPlayback()
				return cmd
			}
		}
	case "q":
		// Add selected track to playlist and queue it to play next.
		if len(m.providers.nav.tracks) == 0 {
			return nil
		}
		rawIdx := m.providers.nav.cursor
		if len(m.providers.nav.searchIdx) > 0 && m.providers.nav.cursor < len(m.providers.nav.searchIdx) {
			rawIdx = m.providers.nav.searchIdx[m.providers.nav.cursor]
		}
		if rawIdx < len(m.providers.nav.tracks) {
			t := m.providers.nav.tracks[rawIdx]
			m.playlist.Add(t)
			newIdx := m.playlist.Len() - 1
			m.playlist.Queue(newIdx)
			m.status.Showf(statusTTLMedium, "Queued: %s", t.DisplayName())
			if !m.player.IsPlaying() {
				m.playlist.Next()
				cmd := m.playCurrentTrack()
				m.notifyPlayback()
				return cmd
			}
		}
	case "esc", "h", "left", "backspace":
		// Navigate back one level depending on the mode and how we got here.
		m.navClearSearch()
		m.providers.nav.cursor = 0
		m.providers.nav.scroll = 0
		switch m.providers.nav.mode {
		case navBrowseModeByAlbum:
			m.providers.nav.screen = navBrowseScreenList
		case navBrowseModeByArtist:
			m.providers.nav.screen = navBrowseScreenList
		case navBrowseModeByArtistAlbum:
			m.providers.nav.screen = navBrowseScreenAlbums
		}
	}
	return nil
}

// handleNavSearchKey handles key input while the nav search bar is open.
func (m *Model) handleNavSearchKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.Code {
	case tea.KeyEscape:
		m.providers.nav.searching = false
		return nil
	case tea.KeyEnter:
		m.providers.nav.searching = false
		return nil
	case tea.KeyBackspace, tea.KeyDelete:
		if m.providers.nav.search != "" {
			m.providers.nav.search = removeLastRune(m.providers.nav.search)
			m.providers.nav.cursor = 0
			m.providers.nav.scroll = 0
			m.navUpdateSearch()
		}
		return nil
	}
	if len(msg.Text) > 0 {
		m.providers.nav.search += msg.Text
		m.providers.nav.cursor = 0
		m.providers.nav.scroll = 0
		m.navUpdateSearch()
	}
	return nil
}

// navNextSort returns the next sort option, wrapping around the list.
func navNextSort(s string, types []provider.SortType) string {
	for i, t := range types {
		if t.ID == s {
			return types[(i+1)%len(types)].ID
		}
	}
	if len(types) > 0 {
		return types[0].ID
	}
	return s
}

// navMaybeAdjustScroll keeps navCursor visible within the rendered list window.
func (m *Model) navMaybeAdjustScroll() {
	visible := m.plVisible
	if visible < 5 {
		visible = 5
	}
	if m.providers.nav.cursor < m.providers.nav.scroll {
		m.providers.nav.scroll = m.providers.nav.cursor
	}
	if m.providers.nav.cursor >= m.providers.nav.scroll+visible {
		m.providers.nav.scroll = m.providers.nav.cursor - visible + 1
	}
}
