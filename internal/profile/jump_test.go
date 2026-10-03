package profile

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveRejectsJumpCyclesWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "connections.json")
	store := NewStore(path)
	a, err := store.Save(Connection{Host: "a.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.Save(Connection{Host: "b.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	a.JumpConnectionID = b.ID
	if _, err := store.Save(a); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, jumpID := range []string{a.ID, b.ID} {
		b.JumpConnectionID = jumpID
		if _, err := store.Save(b); err == nil || !strings.Contains(err.Error(), "cycle") {
			t.Fatalf("cycle save: %v", err)
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Fatal("rejected save changed profiles")
		}
	}
}

func TestSaveRejectsJumpLimitInDependentRoute(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "connections.json"))
	base, err := store.Save(Connection{Host: "base.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	previous := base
	for i := 0; i < MaxJumpHops; i++ {
		previous, err = store.Save(Connection{Host: fmt.Sprintf("hop%d.invalid", i), JumpConnectionID: previous.ID})
		if err != nil {
			t.Fatal(err)
		}
	}
	extra, err := store.Save(Connection{Host: "extra.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	// The edited connection has only one hop, but its dependent now has nine.
	base.JumpConnectionID = extra.ID
	if _, err := store.Save(base); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("dependent jump limit save: %v", err)
	}
}

func TestSaveAllowsEditsUnrelatedToMissingJump(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "connections.json"))
	hop, err := store.Save(Connection{Host: "hop.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := store.Save(Connection{Host: "target.invalid", JumpConnectionID: hop.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(hop.ID); err != nil {
		t.Fatal(err)
	}
	target.Name = "Rename dangling connection"
	if _, err := store.Save(target); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(Connection{Host: "unrelated.invalid"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(Connection{Host: "new.invalid", JumpConnectionID: hop.ID}); err == nil {
		t.Fatal("new missing reference accepted")
	}
	target.JumpConnectionID = ""
	if _, err := store.Save(target); err != nil {
		t.Fatal(err)
	}
}
