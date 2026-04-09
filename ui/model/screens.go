package model

func (s topLevelScreen) isBaseScreen() bool {
	return s == screenMain || s == screenFullVisualizer
}

func (m *Model) screenActive(screen topLevelScreen) bool {
	if screen.isBaseScreen() {
		return m.screenBase == screen
	}
	for _, active := range m.screenStack {
		if active == screen {
			return true
		}
	}
	return false
}

func (m *Model) setBaseScreen(screen topLevelScreen) {
	if !screen.isBaseScreen() {
		screen = screenMain
	}
	m.screenBase = screen
	m.syncTopLevelScreenFlags()
}

func (m *Model) pushScreen(screen topLevelScreen) {
	if screen.isBaseScreen() {
		m.setBaseScreen(screen)
		return
	}
	if m.activeScreen() == screen {
		m.syncTopLevelScreenFlags()
		return
	}
	m.screenStack = append(m.screenStack, screen)
	m.syncTopLevelScreenFlags()
}

func (m *Model) closeScreen(screen topLevelScreen) {
	if screen.isBaseScreen() {
		if m.screenBase == screen {
			m.screenBase = screenMain
		}
		m.syncTopLevelScreenFlags()
		return
	}
	for i := len(m.screenStack) - 1; i >= 0; i-- {
		if m.screenStack[i] != screen {
			continue
		}
		m.screenStack = append(m.screenStack[:i], m.screenStack[i+1:]...)
		break
	}
	m.syncTopLevelScreenFlags()
}

func (m *Model) toggleBaseScreen(screen topLevelScreen) {
	if m.screenBase == screen {
		m.screenBase = screenMain
	} else {
		m.screenBase = screen
	}
	m.syncTopLevelScreenFlags()
}

func (m *Model) syncTopLevelScreenFlags() {
	m.fullVis = m.screenBase == screenFullVisualizer
	m.keymap.visible = m.screenActive(screenKeymap)
	m.themePicker.visible = m.screenActive(screenThemePicker)
	m.devicePicker.visible = m.screenActive(screenDevicePicker)
	m.fileBrowser.visible = m.screenActive(screenFileBrowser)
	m.navBrowser.visible = m.screenActive(screenNavBrowser)
	m.plManager.visible = m.screenActive(screenPlaylistManager)
	m.spotSearch.visible = m.screenActive(screenSpotSearch)
	m.queue.visible = m.screenActive(screenQueue)
	m.showInfo = m.screenActive(screenInfo)
	m.search.active = m.screenActive(screenSearch)
	m.netSearch.active = m.screenActive(screenNetSearch)
	m.urlInputting = m.screenActive(screenURLInput)
	m.lyrics.visible = m.screenActive(screenLyrics)
	m.jumping = m.screenActive(screenJump)
}
