package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"sshbrowse/internal/profile"
	"sshbrowse/internal/session"
	"sshbrowse/internal/sshcmd"
)

func TestSFTPLocalDirectory(t *testing.T) {
	tests := []struct {
		name          string
		downloadsName string
		prepare       func(t *testing.T, home string)
		wantDirectory string
	}{
		{
			name: "downloads directory",
			prepare: func(t *testing.T, home string) {
				t.Helper()
				if err := os.Mkdir(filepath.Join(home, "Downloads"), 0o755); err != nil {
					t.Fatalf("create Downloads: %v", err)
				}
			},
			wantDirectory: "Downloads",
		},
		{
			name:          "configured downloads directory",
			downloadsName: "Transfers",
			prepare: func(t *testing.T, home string) {
				t.Helper()
				for _, name := range []string{"Downloads", "Transfers"} {
					if err := os.Mkdir(filepath.Join(home, name), 0o755); err != nil {
						t.Fatalf("create %s: %v", name, err)
					}
				}
			},
			wantDirectory: "Transfers",
		},
		{
			name:          "missing downloads directory",
			wantDirectory: ".",
		},
		{
			name: "downloads file",
			prepare: func(t *testing.T, home string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(home, "Downloads"), []byte{}, 0o600); err != nil {
					t.Fatalf("create Downloads file: %v", err)
				}
			},
			wantDirectory: ".",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			downloadsName := test.downloadsName
			if downloadsName == "" {
				downloadsName = "Downloads"
			}
			if test.prepare != nil {
				test.prepare(t, home)
			}

			got := sftpLocalDirectoryForHome(home, filepath.Join(home, downloadsName))
			want := home
			if test.wantDirectory != "." {
				want = filepath.Join(home, test.wantDirectory)
			}
			if got != want {
				t.Fatalf("sftp local directory: got %q, want %q", got, want)
			}
		})
	}
}

func TestSessionsRejectStartAfterShutdown(t *testing.T) {
	sessions := NewSessions(nil, nil)
	if err := sessions.ServiceShutdown(); err != nil {
		t.Fatalf("shutdown empty service: %v", err)
	}

	err := sessions.start(1, session.Options{})
	if err == nil || err.Error() != "sessions are shutting down" {
		t.Fatalf("start after shutdown: got %v, want shutdown error", err)
	}
}

func TestSessionsResolveCurrentSavedJump(t *testing.T) {
	store := profile.NewStore(filepath.Join(t.TempDir(), "connections.json"))
	hop, err := store.Save(profile.Connection{Host: "old.invalid", User: "hop-user"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := store.Save(profile.Connection{Host: "target.invalid", JumpConnectionID: hop.ID})
	if err != nil {
		t.Fatal(err)
	}
	sessions := NewSessions(nil, store)
	hop.Host = "edited.invalid"
	hop.IdentityFile = "new-key"
	if _, err := store.Save(hop); err != nil {
		t.Fatal(err)
	}
	for _, sftp := range []bool{false, true} {
		got, err := sessions.connectionArgv(target, sftp)
		if err != nil {
			t.Fatal(err)
		}
		want, err := sshcmd.ConnectionArgv(target, []profile.Connection{hop}, sftp)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("saved hop edits were not applied: %q", got)
		}
	}
	if err := store.Delete(hop.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.connectionArgv(target, false); err == nil {
		t.Fatal("deleted hop silently accepted")
	}
}
