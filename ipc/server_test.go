package ipc

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testSocketPath(t *testing.T) string {
	t.Helper()
	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("cliamp-%d-%d.sock", os.Getpid(), time.Now().UnixNano()))
	t.Cleanup(func() {
		_ = os.Remove(sockPath)
		_ = os.Remove(sockPath + ".pid")
	})
	return sockPath
}

func TestDispatchSeekSendsIPCSeekMsgWithDuration(t *testing.T) {
	var sent any
	s := &Server{
		disp: DispatcherFunc(func(msg interface{}) {
			sent = msg
		}),
	}

	resp := s.dispatch(Request{Cmd: "seek", Value: 12.25})
	if !resp.OK {
		t.Fatalf("dispatch() response = %#v, want OK", resp)
	}

	got, ok := sent.(SeekMsg)
	if !ok {
		t.Fatalf("dispatch() sent %T, want ipc.SeekMsg", sent)
	}
	want := SeekMsg{Offset: 12250 * time.Millisecond}
	if got != want {
		t.Fatalf("dispatch() sent %#v, want %#v", got, want)
	}
}

func TestClaimServerRejectsSecondClaim(t *testing.T) {
	sockPath := testSocketPath(t)
	srv, err := ClaimServer(sockPath)
	if err != nil {
		t.Fatalf("ClaimServer: %v", err)
	}
	defer srv.Close()

	if _, err := ClaimServer(sockPath); err == nil {
		t.Fatal("second ClaimServer() succeeded, want error")
	}
}

func TestClaimedServerReportsStartupUntilDispatcherAttached(t *testing.T) {
	sockPath := testSocketPath(t)
	srv, err := ClaimServer(sockPath)
	if err != nil {
		t.Fatalf("ClaimServer: %v", err)
	}
	defer srv.Close()

	resp := srv.dispatch(Request{Cmd: "status"})
	if resp.OK || resp.Error != "cliamp is still starting" {
		t.Fatalf("dispatch() = %#v, want startup error", resp)
	}
}

func TestClaimServerCleansCorruptPID(t *testing.T) {
	sockPath := testSocketPath(t)
	if err := os.WriteFile(sockPath+".pid", []byte("bad-pid\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	srv, err := ClaimServer(sockPath)
	if err != nil {
		t.Fatalf("ClaimServer: %v", err)
	}
	defer srv.Close()
}
