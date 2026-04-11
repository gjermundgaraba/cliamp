package model

import (
	"fmt"
	"strings"

	"cliamp/provider"
	"cliamp/ui"
)

// — Navidrome browser renderers —

func (m Model) renderNavBrowser() string {
	var lines []string
	switch m.providers.nav.mode {
	case navBrowseModeMenu:
		lines = m.renderNavMenu()
	case navBrowseModeByAlbum:
		switch m.providers.nav.screen {
		case navBrowseScreenTracks:
			lines = m.renderNavTrackList()
		default:
			lines = m.renderNavAlbumList(false)
		}
	case navBrowseModeByArtist:
		switch m.providers.nav.screen {
		case navBrowseScreenTracks:
			lines = m.renderNavTrackList()
		default:
			lines = m.renderNavArtistList()
		}
	case navBrowseModeByArtistAlbum:
		switch m.providers.nav.screen {
		case navBrowseScreenAlbums:
			lines = m.renderNavAlbumList(true)
		case navBrowseScreenTracks:
			lines = m.renderNavTrackList()
		default:
			lines = m.renderNavArtistList()
		}
	default:
		lines = m.renderNavMenu()
	}
	return m.centerOverlay(strings.Join(m.appendFooterMessages(lines), "\n"))
}

func (m Model) renderNavMenu() []string {
	title := "B R O W S E"
	if m.providers.nav.prov != nil {
		title = spacedTitle(m.providers.nav.prov.Name())
	}
	lines := []string{
		titleStyle.Render(title),
		"",
	}

	items := []string{"By Album", "By Artist", "By Artist / Album"}
	for i, item := range items {
		lines = append(lines, cursorLine(item, i == m.providers.nav.cursor))
	}

	lines = append(lines, "",
		helpKey("↑↓", "Navigate ")+helpKey("Enter", "Select ")+helpKey("Esc", "Close"))

	return lines
}

func (m Model) renderNavArtistList() []string {
	lines := []string{titleStyle.Render("A R T I S T S"), ""}

	if m.providers.nav.loading && len(m.providers.nav.artists) == 0 {
		lines = append(lines, dimStyle.Render("  Loading artists..."), "", helpKey("Esc", "Back"))
		return lines
	}

	if len(m.providers.nav.artists) == 0 {
		lines = append(lines, dimStyle.Render("  No artists found."), "", helpKey("Esc", "Back"))
		return lines
	}

	items := m.navScrollItems(len(m.providers.nav.artists), func(i int) string {
		a := m.providers.nav.artists[i]
		return truncate(fmt.Sprintf("%s (%d albums)", a.Name, a.AlbumCount), ui.PanelWidth-6)
	})
	lines = append(lines, items...)

	lines = append(lines, "", m.navCountLine("artists", len(m.providers.nav.artists)))
	lines = append(lines, m.navSearchBar(
		helpKey("←↑↓→", "Navigate ")+helpKey("Enter", "Open ")+helpKey("/", "Search"))...)

	return lines
}

func (m Model) renderNavAlbumList(artistAlbums bool) []string {
	var titleStr string
	if artistAlbums {
		titleStr = titleStyle.Render("A L B U M S : " + m.providers.nav.selArtist.Name)
	} else {
		titleStr = titleStyle.Render("A L B U M S")
	}

	lines := []string{titleStr, ""}

	if !artistAlbums {
		sortLabel := m.navSortLabel(m.providers.nav.sortType)
		lines = append(lines, dimStyle.Render("  Sort: ")+activeToggle.Render(sortLabel), "")
	}

	if m.providers.nav.loading && len(m.providers.nav.albums) == 0 {
		lines = append(lines, dimStyle.Render("  Loading albums..."))
		help := helpKey("Esc", "Back")
		if !artistAlbums {
			help = helpKey("s", "Sort ") + help
		}
		lines = append(lines, "", help)
		return lines
	}

	if len(m.providers.nav.albums) == 0 {
		lines = append(lines, dimStyle.Render("  No albums found."))
		help := helpKey("Esc", "Back")
		if !artistAlbums {
			help = helpKey("s", "Sort ") + help
		}
		lines = append(lines, "", help)
		return lines
	}

	items := m.navScrollItems(len(m.providers.nav.albums), func(i int) string {
		a := m.providers.nav.albums[i]
		var label string
		if a.Year > 0 {
			label = fmt.Sprintf("%s — %s (%d)", a.Name, a.Artist, a.Year)
		} else {
			label = fmt.Sprintf("%s — %s", a.Name, a.Artist)
		}
		return truncate(label, ui.PanelWidth-6)
	})
	lines = append(lines, items...)

	if m.providers.nav.albumLoading {
		lines = append(lines, dimStyle.Render("  Loading more..."))
	} else {
		lines = append(lines, m.navCountLine("albums", len(m.providers.nav.albums)))
	}

	defaultHelp := helpKey("←↑↓→", "Navigate ") + helpKey("Enter", "Open ")
	if !artistAlbums {
		defaultHelp += helpKey("s", "Sort ")
	}
	defaultHelp += helpKey("/", "Search")
	lines = append(lines, m.navSearchBar(defaultHelp)...)

	return lines
}

func (m Model) renderNavTrackList() []string {
	var breadcrumb string
	switch m.providers.nav.mode {
	case navBrowseModeByArtist:
		breadcrumb = "A R T I S T : " + m.providers.nav.selArtist.Name
	case navBrowseModeByAlbum:
		breadcrumb = "A L B U M : " + m.providers.nav.selAlbum.Name
	case navBrowseModeByArtistAlbum:
		breadcrumb = m.providers.nav.selArtist.Name + " / " + m.providers.nav.selAlbum.Name
	}

	lines := []string{titleStyle.Render(breadcrumb), ""}

	if m.providers.nav.loading && len(m.providers.nav.tracks) == 0 {
		lines = append(lines, dimStyle.Render("  Loading tracks..."), "", helpKey("Esc", "Back"))
		return lines
	}

	if len(m.providers.nav.tracks) == 0 {
		lines = append(lines, dimStyle.Render("  No tracks found."), "", helpKey("Esc", "Back"))
		return lines
	}

	maxVisible := m.plVisible
	if maxVisible < 5 {
		maxVisible = 5
	}

	useFilter := len(m.providers.nav.searchIdx) > 0 || m.providers.nav.search != ""

	if useFilter {
		items := m.navScrollItems(len(m.providers.nav.tracks), func(i int) string {
			return fmt.Sprintf("%d. %s", i+1, truncate(m.providers.nav.tracks[i].DisplayName(), ui.PanelWidth-8))
		})
		lines = append(lines, items...)
	} else {
		scroll := m.providers.nav.scroll
		rendered := 0
		prevAlbum := ""
		if scroll > 0 {
			prevAlbum = m.providers.nav.tracks[scroll-1].Album
		}

		for i := scroll; i < len(m.providers.nav.tracks) && rendered < maxVisible; i++ {
			t := m.providers.nav.tracks[i]

			if album := t.Album; album != "" && album != prevAlbum {
				lines = append(lines, m.albumSeparator(album, t.Year))
				if rendered >= maxVisible {
					break
				}
			}
			prevAlbum = t.Album

			label := fmt.Sprintf("%d. %s", i+1, truncate(t.DisplayName(), ui.PanelWidth-8))
			lines = append(lines, cursorLine(label, i == m.providers.nav.cursor))
			rendered++
		}

		lines = padLines(lines, maxVisible, rendered)
	}

	lines = append(lines, "", m.navCountLine("tracks", len(m.providers.nav.tracks)))
	lines = append(lines, m.navSearchBar(
		helpKey("←↑↓→", "Navigate ")+
			helpKey("Enter", "Play ")+
			helpKey("q", "Queue ")+
			helpKey("R", "Replace ")+
			helpKey("a", "Append ")+
			helpKey("/", "Search"))...)
	return lines
}

func (m Model) navSortLabel(sortID string) string {
	if ab, ok := m.providers.nav.prov.(provider.AlbumBrowser); ok {
		for _, st := range ab.AlbumSortTypes() {
			if st.ID == sortID {
				return st.Label
			}
		}
	}
	return sortID
}

func spacedTitle(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "B R O W S E"
	}
	runes := []rune(strings.ToUpper(s))
	parts := make([]string, 0, len(runes))
	for _, r := range runes {
		parts = append(parts, string(r))
	}
	return strings.Join(parts, " ")
}
