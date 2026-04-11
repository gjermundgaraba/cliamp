package model

import (
	tea "charm.land/bubbletea/v2"

	"cliamp/provider"
)

// maybeLoadCatalogBatch triggers a catalog batch fetch when the cursor is near the
// bottom of the provider list and more entries are available.
func (m *Model) maybeLoadCatalogBatch() tea.Cmd {
	loader, ok := m.providers.active.(provider.CatalogLoader)
	if !ok {
		return nil
	}
	if m.providers.catalog.loading || m.providers.catalog.done {
		return nil
	}
	if cs, ok := m.providers.active.(provider.CatalogSearcher); ok && cs.IsSearching() {
		return nil
	}
	if m.providers.cursor >= len(m.providers.lists)-10 {
		m.providers.catalog.loading = true
		return fetchCatalogBatchCmd(loader, m.providers.catalog.offset, catalogBatchSize)
	}
	return nil
}

// toggleProviderFavorite toggles favorite status for the current entry in the
// provider list (only works for providers implementing FavoriteToggler + SectionedList).
func (m *Model) toggleProviderFavorite() tea.Cmd {
	ft, ok := m.providers.active.(provider.FavoriteToggler)
	if !ok || len(m.providers.lists) == 0 {
		return nil
	}
	id := m.providers.lists[m.providers.cursor].ID
	if sl, ok := m.providers.active.(provider.SectionedList); ok {
		if !sl.IsFavoritableID(id) {
			return nil
		}
	}
	added, name, err := ft.ToggleFavorite(id)
	if err != nil {
		return nil
	}
	if added {
		m.status.Showf(statusTTLMedium, "Favorited: %s", name)
	} else {
		m.status.Showf(statusTTLMedium, "Removed: %s", name)
	}

	prevID := id
	if lists, err := m.providers.active.Playlists(); err == nil {
		m.providers.lists = lists
		for i, p := range m.providers.lists {
			if p.ID == prevID {
				m.providers.cursor = i
				return nil
			}
		}
		if m.providers.cursor >= len(m.providers.lists) {
			m.providers.cursor = max(0, len(m.providers.lists)-1)
		}
	}
	return nil
}
