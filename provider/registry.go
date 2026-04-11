package provider

import "strings"

type Descriptor struct {
	Key               string
	Name              string
	DefaultSelectable bool
}

var descriptors = []Descriptor{
	{Key: KeyRadio, Name: "Radio", DefaultSelectable: true},
	{Key: KeyLocal, Name: "Local"},
	{Key: KeyNavidrome, Name: "Navidrome", DefaultSelectable: true},
	{Key: KeyPlex, Name: "Plex", DefaultSelectable: true},
	{Key: KeyJellyfin, Name: "Jellyfin", DefaultSelectable: true},
	{Key: KeySpotify, Name: "Spotify", DefaultSelectable: true},
	{Key: KeyYT, Name: "YouTube (All)", DefaultSelectable: true},
	{Key: KeyYouTube, Name: "YouTube", DefaultSelectable: true},
	{Key: KeyYTMusic, Name: "YouTube Music", DefaultSelectable: true},
}

func DisplayName(key string) string {
	desc, ok := findDescriptor(key)
	if !ok {
		return key
	}
	return desc.Name
}

func NormalizeDefaultProviderKey(raw string) (string, bool) {
	return normalizeKey(raw, true)
}

func DefaultProviderUsage() string {
	keys := make([]string, 0, len(descriptors))
	for _, desc := range descriptors {
		if desc.DefaultSelectable {
			keys = append(keys, desc.Key)
		}
	}
	return strings.Join(keys, ", ")
}

func normalizeKey(raw string, defaultOnly bool) (string, bool) {
	desc, ok := findDescriptor(raw)
	if !ok {
		return "", false
	}
	if defaultOnly && !desc.DefaultSelectable {
		return "", false
	}
	return desc.Key, true
}

func findDescriptor(raw string) (Descriptor, bool) {
	key := strings.ToLower(strings.TrimSpace(raw))
	for _, desc := range descriptors {
		if desc.Key == key {
			return desc, true
		}
	}
	return Descriptor{}, false
}
