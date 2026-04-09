package resume

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"testing"
	"time"

	"cliamp/internal/session"
	"cliamp/internal/source"
	"cliamp/playlist"
)

func TestResumeLockHelperProcess(t *testing.T) {
	if os.Getenv("CLIAMP_RESUME_LOCK_HELPER") != "1" {
		return
	}

	holdMs, err := strconv.Atoi(os.Getenv("CLIAMP_RESUME_LOCK_HOLD_MS"))
	if err != nil {
		fmt.Printf("hold duration: %v\n", err)
		os.Exit(2)
	}
	if err := withLock(func() error {
		fmt.Println("ready")
		time.Sleep(time.Duration(holdMs) * time.Millisecond)
		return nil
	}); err != nil {
		fmt.Printf("lock error: %v\n", err)
		os.Exit(2)
	}
	os.Exit(0)
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	snapshot := session.PersistedSnapshot{
		LastProviderKey: "navidrome",
		ProviderSessions: map[string]session.State{
			"navidrome": {
				OwnerKey:    "navidrome",
				PositionSec: 42,
				Source: source.Ref{
					ProviderKey: "navidrome",
					Kind:        source.Playlist,
					ID:          "pl-123",
				},
				Playlist: session.PlaylistState{
					Current: session.TrackRef{Index: 3, Path: "/music/track.mp3", MetaKey: "navidrome.id", MetaValue: "song-abc"},
					Cursor:  session.TrackRef{Index: 1, Path: "/music/current-source.mp3"},
					Queue:   []session.TrackRef{{Index: 0, Path: "/music/next.mp3"}},
				},
			},
			"radio": {
				OwnerKey: "radio",
				Tracks: []playlist.Track{
					{Path: "https://radio.example.com/stream", Title: "Radio", Stream: true, Realtime: true},
				},
				Playlist: session.PlaylistState{
					Current: session.TrackRef{Index: 0, Path: "https://radio.example.com/stream"},
					Cursor:  session.TrackRef{Index: 0, Path: "https://radio.example.com/stream"},
				},
			},
		},
	}

	Save(snapshot)

	got := Load()
	if !reflect.DeepEqual(got, snapshot) {
		t.Fatalf("Load() = %+v, want %+v", got, snapshot)
	}
}

func TestClearRemovesState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	Save(session.PersistedSnapshot{
		LastProviderKey: "radio",
		ProviderSessions: map[string]session.State{
			"radio": {
				OwnerKey: "radio",
				Tracks:   []playlist.Track{{Path: "/music/track.mp3"}},
			},
		},
	})

	Clear()

	if cleared := Load(); !reflect.DeepEqual(cleared, session.PersistedSnapshot{}) {
		t.Errorf("state should be empty after Clear(), got %+v", cleared)
	}
}

func TestSaveLoadLastProviderKeyOnly(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	snapshot := session.PersistedSnapshot{LastProviderKey: "jellyfin"}

	Save(snapshot)

	if got := Load(); !reflect.DeepEqual(got, snapshot) {
		t.Fatalf("Load() = %+v, want %+v", got, snapshot)
	}
}

func TestSaveNoOpsOnEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	Save(session.PersistedSnapshot{})

	f, _ := stateFile()
	if _, err := os.Stat(f); err == nil {
		t.Error("Save should not create file for empty snapshot")
	}

	Save(session.PersistedSnapshot{
		ProviderSessions: map[string]session.State{
			"test": {},
		},
	})

	if _, err := os.Stat(f); err == nil {
		t.Error("Save should not create file when all entries are non-persistable")
	}
}

func TestSaveWaitsForLockRelease(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	cmd := exec.Command(os.Args[0], "-test.run=TestResumeLockHelperProcess")
	cmd.Env = append(os.Environ(),
		"CLIAMP_RESUME_LOCK_HELPER=1",
		"CLIAMP_RESUME_LOCK_HOLD_MS=400",
		"HOME="+dir,
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start helper: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	sc := bufio.NewScanner(stdout)
	deadline := time.Now().Add(5 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		if sc.Scan() && sc.Text() == "ready" {
			ready = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ready {
		t.Fatal("helper did not acquire lock in time")
	}

	snapshot := session.PersistedSnapshot{LastProviderKey: "radio"}
	done := make(chan struct{})
	go func() {
		Save(snapshot)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Save returned before lock was released")
	case <-time.After(100 * time.Millisecond):
	}

	if err := cmd.Wait(); err != nil {
		t.Fatalf("helper exit: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Save did not complete after lock release")
	}

	if got := Load(); !reflect.DeepEqual(got, snapshot) {
		t.Fatalf("Load() = %+v, want %+v", got, snapshot)
	}
}

func TestSaveLastWriterWins(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	first := session.PersistedSnapshot{
		LastProviderKey: "radio",
		ProviderSessions: map[string]session.State{
			"radio": {
				OwnerKey: "radio",
				Tracks: []playlist.Track{
					{Path: "https://radio.example.com/first", Title: "First", Stream: true, Realtime: true},
				},
			},
		},
	}
	second := session.PersistedSnapshot{
		LastProviderKey: "local",
		ProviderSessions: map[string]session.State{
			"local": {
				OwnerKey: "local",
				Tracks: []playlist.Track{
					{Path: "/music/final.mp3", Title: "Final"},
				},
			},
		},
	}

	Save(first)
	Save(second)

	if got := Load(); !reflect.DeepEqual(got, second) {
		t.Fatalf("Load() = %+v, want %+v", got, second)
	}
}
