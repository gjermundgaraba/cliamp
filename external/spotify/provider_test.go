//go:build !windows

package spotify

import (
	"testing"
)

func TestPlaylistAccessible(t *testing.T) {
	const me = "user123"

	tests := []struct {
		name          string
		ownerID       string
		collaborative bool
		userID        string
		want          bool
	}{
		{"own playlist", me, false, me, true},
		{"own collaborative", me, true, me, true},
		{"other user's playlist", "otheruser", false, me, false},
		{"other user's collaborative", "otheruser", true, me, true},
		{"no userID fallback", "otheruser", false, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := spotifyPlaylistItem{
				ID:            "pl1",
				Name:          "Test",
				Collaborative: tt.collaborative,
			}
			item.Owner.ID = tt.ownerID

			got := playlistAccessible(item, tt.userID)
			if got != tt.want {
				t.Errorf("playlistAccessible(owner=%q, collaborative=%v, userID=%q) = %v, want %v",
					tt.ownerID, tt.collaborative, tt.userID, got, tt.want)
			}
		})
	}
}

func TestSpotifyTrackToPlaylistTrackAddsArtwork(t *testing.T) {
	track := spotifyTrackToPlaylistTrack(&spotifyTrackObj{
		ID:   "track-1",
		Name: "Song",
		Artists: []struct {
			Name string `json:"name"`
		}{
			{Name: "Artist"},
		},
		Album: spotifyTrackAlbum{
			ID:          "album-1",
			Name:        "Album",
			ReleaseDate: "2024-05-01",
			Images: []spotifyImage{
				{URL: "https://i.scdn.co/image/abc"},
			},
		},
		DurationMs:  215000,
		TrackNumber: 7,
	})

	if track.Artwork.CacheKey != "spotify:album:album-1" {
		t.Fatalf("Artwork.CacheKey = %q, want spotify:album:album-1", track.Artwork.CacheKey)
	}
	if track.Artwork.URL != "https://i.scdn.co/image/abc" {
		t.Fatalf("Artwork.URL = %q, want artwork URL", track.Artwork.URL)
	}
	if track.Year != 2024 {
		t.Fatalf("Year = %d, want 2024", track.Year)
	}
	if track.Owner.Provider != "spotify" || track.Owner.ID != "track-1" {
		t.Fatalf("Owner = %+v, want spotify/track-1", track.Owner)
	}
}
