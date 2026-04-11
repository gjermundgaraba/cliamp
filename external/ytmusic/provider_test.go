package ytmusic

import "testing"

func TestYoutubeArtworkRefUsesVideoID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
	}{
		{
			name: "video id",
			id:   "abc123",
			want: "youtube:abc123",
		},
		{
			name: "empty id",
			id:   "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := youtubeArtworkRef(tt.id).CacheKey; got != tt.want {
				t.Fatalf("youtubeArtworkRef(%q).CacheKey = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}
