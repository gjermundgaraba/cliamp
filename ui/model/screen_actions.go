package model

import (
	tea "charm.land/bubbletea/v2"

	"cliamp/playlist"
	"cliamp/provider"
	"cliamp/ui"
)

func (m *Model) openQueue() {
	if m.focus != focusPlaylist {
		return
	}
	m.queue.cursor = 0
	m.pushScreen(screenQueue)
}

func (m *Model) openSearch() {
	m.search.query = ""
	m.search.results = nil
	m.search.cursor = 0
	m.prevFocus = m.focus
	m.focus = focusSearch
	m.pushScreen(screenSearch)
}

func (m *Model) openNetSearch(soundcloud bool) {
	m.netSearch.query = ""
	m.netSearch.soundcloud = soundcloud
	m.prevFocus = m.focus
	m.focus = focusNetSearch
	m.pushScreen(screenNetSearch)
}

func (m *Model) openSpotSearch() {
	prov := m.findProviderWith(func(p playlist.Provider) bool {
		_, ok := p.(provider.Searcher)
		return ok
	})
	if prov == nil {
		return
	}
	m.spotSearch = spotSearchState{
		prov:   prov,
		screen: spotSearchInput,
	}
	m.pushScreen(screenSpotSearch)
}

func (m *Model) openTrackInfo() {
	m.pushScreen(screenInfo)
}

func (m *Model) toggleLyrics() tea.Cmd {
	if m.activeScreen() == screenLyrics {
		m.closeScreen(screenLyrics)
		return nil
	}
	m.pushScreen(screenLyrics)
	if m.lyrics.loading {
		return nil
	}
	artist, title := m.lyricsArtistTitle()
	if artist == "" || title == "" {
		return nil
	}
	query := artist + "\n" + title
	if query == m.lyrics.query {
		return nil
	}
	m.lyrics.query = query
	m.lyrics.loading = true
	m.lyrics.lines = nil
	m.lyrics.err = nil
	return fetchLyricsCmd(artist, title)
}

func (m *Model) openURLInput() {
	m.urlInput = ""
	m.pushScreen(screenURLInput)
}

func (m *Model) toggleFullVisualizer() {
	m.toggleBaseScreen(screenFullVisualizer)
	if m.fullVis {
		m.vis.Rows = max(ui.DefaultVisRows, (m.height-10)*4/5)
		ui.PanelWidth = max(0, m.width-2*ui.PaddingH)
		return
	}
	m.vis.Rows = ui.DefaultVisRows
	m.restorePanelWidth()
}

func (m *Model) openDevicePicker() tea.Cmd {
	m.devicePicker.cursor = 0
	m.pushScreen(screenDevicePicker)
	if len(m.devicePicker.devices) > 0 {
		return nil
	}
	m.devicePicker.loading = true
	return listDevicesCmd()
}

func (m *Model) openKeymap() {
	m.pushScreen(screenKeymap)
}
