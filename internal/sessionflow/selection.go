package sessionflow

type ProviderPrefs struct {
	Explicit string
	Selected string
}

type ProviderRef struct {
	Key       string
	Available bool
}

type ProviderSelection struct {
	Index int
	Key   string
}

func ResolveStartupProvider(providers []ProviderRef, prefs ProviderPrefs) ProviderSelection {
	for _, key := range []string{prefs.Explicit, prefs.Selected} {
		if choice, ok := selectAvailableProvider(providers, key); ok {
			return choice
		}
	}
	for i, provider := range providers {
		if provider.Available {
			return ProviderSelection{Index: i, Key: provider.Key}
		}
	}
	return ProviderSelection{Index: -1}
}

func selectAvailableProvider(providers []ProviderRef, key string) (ProviderSelection, bool) {
	if key == "" {
		return ProviderSelection{}, false
	}
	for i, provider := range providers {
		if provider.Available && provider.Key == key {
			return ProviderSelection{Index: i, Key: provider.Key}, true
		}
	}
	return ProviderSelection{}, false
}
