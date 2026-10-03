package profile

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestConnectionsFileReadHasSizeLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "connections.json")
	prefix, err := json.Marshal(fileFormat{
		Version:     currentFileVersion,
		Connections: []Connection{},
		Folders:     []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	atLimit := append(prefix, bytes.Repeat([]byte(" "), maxConnectionsFileSize-len(prefix))...)
	if err := os.WriteFile(path, atLimit, 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewStore(path)
	if _, err := store.List(); err != nil {
		t.Fatalf("file at size limit: %v", err)
	}

	if err := os.WriteFile(path, append(atLimit, ' '), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(); err == nil || !strings.Contains(err.Error(), "exceeds the 1 MiB size limit") {
		t.Fatalf("oversized file error = %v", err)
	}
}

func TestStoreWritesSchemaVersionOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "connections.json")
	if _, err := NewStore(path).Save(Connection{Name: "Current", Host: "current.example"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var parsed fileFormat
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Version != 1 {
		t.Fatalf("written profile version = %d, want 1", parsed.Version)
	}
}

func TestStoreRejectsUnsupportedFileVersions(t *testing.T) {
	for _, version := range []int{0, 2} {
		t.Run("version "+strconv.Itoa(version), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "connections.json")
			data, err := json.Marshal(fileFormat{
				Version:     version,
				Connections: []Connection{},
				Folders:     []string{},
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			wantError := "profile: unsupported file version " + strconv.Itoa(version)
			if _, err := NewStore(path).List(); err == nil || err.Error() != wantError {
				t.Fatalf("version %d load error = %v", version, err)
			}
		})
	}
}

func TestStorePersistsProfilePathWithSpaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile with spaces", "connections.json")
	store := NewStore(path)
	if _, err := store.Save(Connection{Name: "Windows path", Host: "host.example"}); err != nil {
		t.Fatal(err)
	}
	connections, err := NewStore(path).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(connections) != 1 || connections[0].Name != "Windows path" {
		t.Fatalf("connections = %+v", connections)
	}
}

func TestOversizedWritePreservesPreviousReadableFile(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "connections.json")
	store := NewStore(path)
	kept, err := store.Save(Connection{Name: "Kept", Host: "kept.example"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Save(Connection{
		Name: strings.Repeat("x", maxConnectionsFileSize),
		Host: "oversized.example",
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds the 1 MiB size limit") {
		t.Fatalf("oversized save error = %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("connections file changed after oversized save")
	}
	connections, err := store.List()
	if err != nil {
		t.Fatalf("previous file became unreadable: %v", err)
	}
	if len(connections) != 1 || connections[0].ID != kept.ID {
		t.Fatalf("connections = %+v", connections)
	}
}

func TestWriteSizeLimitIncludesTrailingNewline(t *testing.T) {
	connection := func(nameLength int) Connection {
		return Connection{
			ID:              "fixed",
			Name:            strings.Repeat("x", nameLength),
			Host:            "boundary.example",
			LocalForwards:   []string{},
			RemoteForwards:  []string{},
			DynamicForwards: []string{},
		}
	}
	serialize := func(value Connection) []byte {
		data, err := json.MarshalIndent(fileFormat{
			Version:     currentFileVersion,
			Connections: []Connection{value},
			Folders:     []string{},
		}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	nameLength := maxConnectionsFileSize - len(serialize(connection(0)))
	data := serialize(connection(nameLength))
	if len(data) != maxConnectionsFileSize {
		t.Fatalf("serialized boundary size = %d, want %d", len(data), maxConnectionsFileSize)
	}

	path := filepath.Join(t.TempDir(), "connections.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewStore(path)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(connection(nameLength)); err == nil || !strings.Contains(err.Error(), "exceeds the 1 MiB size limit") {
		t.Fatalf("boundary save error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("connections file changed after newline-sized save")
	}
	if _, err := store.List(); err != nil {
		t.Fatalf("boundary file became unreadable: %v", err)
	}
}

func TestProfileOperationsWaitForAnotherProcessLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "connections.json")
	store := NewStore(path)
	if _, err := store.Save(Connection{Name: "Kept", Host: "kept.example"}); err != nil {
		t.Fatal(err)
	}
	lockFile, err := acquireFileLock(path)
	if err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		close(started)
		_, err := store.Save(Connection{Name: "Added", Host: "added.example"})
		result <- err
	}()
	<-started
	select {
	case err := <-result:
		releaseFileLock(lockFile)
		t.Fatalf("profile operation bypassed the file lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	releaseFileLock(lockFile)

	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("profile operation did not complete after the file lock was released")
	}
	connections, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(connections) != 2 {
		t.Fatalf("connections = %+v", connections)
	}
}

func TestCurrentProfileKeepsConnectionsWhenOnboardingIsAnswered(t *testing.T) {
	path := filepath.Join(t.TempDir(), "connections.json")
	data, err := json.Marshal(fileFormat{
		Version:     currentFileVersion,
		Connections: []Connection{{ID: "saved", Name: "Saved", Host: "saved.example"}},
		Folders:     []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewStore(path)
	if err := store.AnswerSSHConfigImport(); err != nil {
		t.Fatal(err)
	}
	connections, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(connections) != 1 || connections[0].ID != "saved" || connections[0].Host != "saved.example" {
		t.Fatalf("connections = %+v", connections)
	}
	state, err := store.Onboarding()
	if err != nil || !state.SSHConfigImportAnswered {
		t.Fatalf("state = %+v, err = %v", state, err)
	}
}

func TestCurrentFoldersNormalizeInFirstSeenOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "connections.json")
	data, err := json.Marshal(fileFormat{
		Version: currentFileVersion,
		Connections: []Connection{
			{ID: "one", Folder: "Team / Production", Name: "One", Host: "one.example"},
			{ID: "two", Folder: "Archive", Name: "Two", Host: "two.example"},
			{ID: "three", Folder: "Team / Staging", Name: "Three", Host: "three.example"},
		},
		Folders: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewStore(path)
	folders, err := store.Folders()
	if err != nil {
		t.Fatal(err)
	}
	wantFolders := []string{"Team", "Team/Production", "Archive", "Team/Staging"}
	if !reflect.DeepEqual(folders, wantFolders) {
		t.Fatalf("folders = %#v, want %#v", folders, wantFolders)
	}
	connections, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{connections[0].Folder, connections[1].Folder, connections[2].Folder}; !reflect.DeepEqual(got, []string{"Team/Production", "Archive", "Team/Staging"}) {
		t.Fatalf("connection folders = %#v", got)
	}
}

func TestNormalizeFolderPath(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "root", input: "  ", want: ""},
		{name: "segments", input: " Team / Production ", want: "Team/Production"},
		{name: "empty segment", input: "Team//Production", wantErr: true},
		{name: "leading slash", input: "/Team", wantErr: true},
		{name: "traversal", input: "Team/../Production", wantErr: true},
		{name: "control", input: "Team\tProduction", wantErr: true},
		{name: "oversized", input: strings.Repeat("x", maxFolderPathBytes+1), wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := NormalizeFolderPath(test.input)
			if test.wantErr {
				if err == nil {
					t.Fatalf("NormalizeFolderPath(%q) = %q, nil", test.input, got)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("NormalizeFolderPath(%q) = %q, %v; want %q", test.input, got, err, test.want)
			}
		})
	}
}

func TestSaveMoveAndImportPersistFolderAncestors(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "connections.json"))
	saved, err := store.Save(Connection{Folder: "Team / Production", Host: "one.example"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Move([]string{saved.ID}, "Archive / 2026"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ImportSSHConfig([]Connection{
		{Name: "alias", Host: "alias", Folder: "Imported/Work", Provenance: &Provenance{SSHConfigAlias: "alias"}},
	}, false); err != nil {
		t.Fatal(err)
	}
	folders, err := store.Folders()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Team", "Team/Production", "Archive", "Archive/2026", "Imported", "Imported/Work"}
	if !reflect.DeepEqual(folders, want) {
		t.Fatalf("folders = %#v, want %#v", folders, want)
	}
}

func TestReorderConnectionsWithinFolder(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "connections.json"))
	connections := saveConnections(t, store,
		Connection{Folder: "Team", Name: "First", Host: "first.example"},
		Connection{Folder: "Team", Name: "Selected one", Host: "one.example"},
		Connection{Folder: "Team", Name: "Target", Host: "target.example"},
		Connection{Folder: "Team", Name: "Selected two", Host: "two.example"},
		Connection{Folder: "Team", Name: "Last", Host: "last.example"},
	)

	if err := store.Reorder([]string{connections[1].ID, connections[3].ID}, connections[2].ID, false); err != nil {
		t.Fatal(err)
	}
	got := listConnectionNames(t, store)
	want := []string{"First", "Selected one", "Selected two", "Target", "Last"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("connection order = %#v, want %#v", got, want)
	}
}

func TestReorderConnectionsAcrossFoldersAfterTarget(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "connections.json"))
	connections := saveConnections(t, store,
		Connection{Name: "First", Host: "first.example"},
		Connection{Folder: "Source", Name: "Selected one", Host: "one.example"},
		Connection{Folder: "Target folder", Name: "Target", Host: "target.example"},
		Connection{Folder: "Other", Name: "Selected two", Host: "two.example"},
		Connection{Name: "Last", Host: "last.example"},
	)

	// The input order reflects sidebar display order, which can differ from file order.
	if err := store.Reorder([]string{connections[3].ID, connections[1].ID}, connections[2].ID, true); err != nil {
		t.Fatal(err)
	}
	got, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	wantNames := []string{"First", "Target", "Selected two", "Selected one", "Last"}
	wantFolders := []string{"", "Target folder", "Target folder", "Target folder", ""}
	if len(got) != len(wantNames) {
		t.Fatalf("connections = %+v", got)
	}
	for i, connection := range got {
		if connection.Name != wantNames[i] || connection.Folder != wantFolders[i] {
			t.Fatalf("connections[%d] = %+v, want name %q in folder %q", i, connection, wantNames[i], wantFolders[i])
		}
	}
}

func TestReorderRejectsInvalidInputWithoutChangingConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "connections.json")
	store := NewStore(path)
	connections := saveConnections(t, store,
		Connection{Name: "First", Host: "first.example"},
		Connection{Name: "Target", Host: "target.example"},
	)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		ids      []string
		targetID string
	}{
		{name: "empty selection", targetID: connections[1].ID},
		{name: "empty selected ID", ids: []string{""}, targetID: connections[1].ID},
		{name: "duplicate selected ID", ids: []string{connections[0].ID, connections[0].ID}, targetID: connections[1].ID},
		{name: "missing selected ID", ids: []string{"missing"}, targetID: connections[1].ID},
		{name: "missing target", ids: []string{connections[0].ID}, targetID: "missing"},
		{name: "selected target", ids: []string{connections[1].ID}, targetID: connections[1].ID},
		{name: "empty target", ids: []string{connections[0].ID}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := store.Reorder(test.ids, test.targetID, false); err == nil {
				t.Fatal("Reorder succeeded")
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, before) {
				t.Fatal("connections file changed after invalid reorder")
			}
		})
	}
}

func saveConnections(t *testing.T, store *Store, connections ...Connection) []Connection {
	t.Helper()
	saved := make([]Connection, 0, len(connections))
	for _, connection := range connections {
		created, err := store.Save(connection)
		if err != nil {
			t.Fatal(err)
		}
		saved = append(saved, created)
	}
	return saved
}

func listConnectionNames(t *testing.T, store *Store) []string {
	t.Helper()
	connections, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(connections))
	for _, connection := range connections {
		names = append(names, connection.Name)
	}
	return names
}

func TestFolderCRUDPreservesEmptyFoldersAndRenamesSubtree(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "connections.json"))
	if err := store.CreateFolder("Team/Production/Empty"); err != nil {
		t.Fatal(err)
	}
	saved, err := store.Save(Connection{Folder: "Team/Production", Name: "Server", Host: "server.example"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RenameFolder("Team", "Customers/Acme"); err != nil {
		t.Fatal(err)
	}
	folders, err := store.Folders()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Customers", "Customers/Acme", "Customers/Acme/Production", "Customers/Acme/Production/Empty"}
	if !reflect.DeepEqual(folders, want) {
		t.Fatalf("folders = %#v, want %#v", folders, want)
	}
	connections, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(connections) != 1 || connections[0].ID != saved.ID || connections[0].Folder != "Customers/Acme/Production" {
		t.Fatalf("connections = %+v", connections)
	}
	if err := store.DeleteFolder("Customers/Acme/Production/Empty"); err != nil {
		t.Fatal(err)
	}
	folders, err = store.Folders()
	if err != nil {
		t.Fatal(err)
	}
	if containsFolder(folders, "Customers/Acme/Production/Empty") {
		t.Fatalf("empty folder survived deletion: %#v", folders)
	}
}

func TestFolderSurvivesDeletingItsLastConnection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "connections.json")
	store := NewStore(path)
	saved, err := store.Save(Connection{Folder: "Team/Empty Later", Host: "server.example"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(saved.ID); err != nil {
		t.Fatal(err)
	}
	folders, err := NewStore(path).Folders()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(folders, []string{"Team", "Team/Empty Later"}) {
		t.Fatalf("folders = %#v", folders)
	}
}

func TestFolderCRUDRejectsInvalidAndNonEmptyChanges(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "connections.json"))
	if err := store.CreateFolder("Team/Child"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(Connection{Folder: "Team", Host: "server.example"}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "invalid save path", run: func() error {
			_, err := store.Save(Connection{Folder: "Team//Other", Host: "other.example"})
			return err
		}},
		{name: "invalid move path", run: func() error { return store.Move([]string{"missing"}, "Team//Other") }},
		{name: "duplicate", run: func() error { return store.CreateFolder("Team") }},
		{name: "invalid path", run: func() error { return store.CreateFolder("Team//Other") }},
		{name: "rename into subtree", run: func() error { return store.RenameFolder("Team", "Team/Child/New") }},
		{name: "rename collision", run: func() error { return store.RenameFolder("Team/Child", "Team") }},
		{name: "delete with child", run: func() error { return store.DeleteFolder("Team") }},
		{name: "delete root", run: func() error { return store.DeleteFolder("") }},
		{name: "missing", run: func() error { return store.DeleteFolder("Missing") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); err == nil {
				t.Fatal("operation succeeded")
			}
		})
	}
}

func TestRenameFolderRejectsOversizedDescendantWithoutChangingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "connections.json")
	store := NewStore(path)
	if err := store.CreateFolder("Parent/Child"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	err = store.RenameFolder("Parent", strings.Repeat("x", maxFolderPathBytes))
	if err == nil || !strings.Contains(err.Error(), "folder path exceeds") {
		t.Fatalf("rename error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("connections file changed after invalid subtree rename")
	}
	if _, err := store.Folders(); err != nil {
		t.Fatalf("folders became unreadable: %v", err)
	}
}

func TestDeleteFolderRejectsConnectionAndRenameFailureIsAtomic(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "connections.json")
	store := NewStore(path)
	if _, err := store.Save(Connection{Folder: "Team/Production", Host: "server.example"}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteFolder("Team/Production"); err == nil || !strings.Contains(err.Error(), "contains connections") {
		t.Fatalf("delete error = %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	restore := blockProfileReplacement(t, directory, path)
	renameErr := store.RenameFolder("Team", "Customers")
	restore()
	if renameErr == nil {
		t.Fatal("RenameFolder succeeded with a blocked profile replacement")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatal("connections file changed after failed subtree rename")
	}
}

func TestImportSSHConfigIsOneBatchAndDeduplicatesAliases(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "connections.json"))
	existing, err := store.Save(Connection{Name: "Kept", Host: "kept.example"})
	if err != nil {
		t.Fatal(err)
	}
	first := importedConnection("alpha", "alpha.example")
	duplicate := importedConnection("ALPHA", "duplicate.example")
	second := importedConnection("beta", "beta.example")
	imported, duplicates, err := store.ImportSSHConfig([]Connection{first, duplicate, second}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(imported) != 2 || duplicates != 1 || imported[0].ID == "" || imported[1].ID == "" {
		t.Fatalf("imported = %+v, duplicates = %d", imported, duplicates)
	}
	connections, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{connections[0].ID, connections[1].Name, connections[2].Name}; !reflect.DeepEqual(got, []string{existing.ID, "alpha", "beta"}) {
		t.Fatalf("stored = %+v", connections)
	}
}

func TestImportWriteFailurePreservesExistingProfiles(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "connections.json")
	store := NewStore(path)
	existing, err := store.Save(Connection{Name: "Kept", Host: "kept.example"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	restore := blockProfileReplacement(t, directory, path)
	_, _, importErr := store.ImportSSHConfig([]Connection{importedConnection("alpha", "alpha.example")}, false)
	restore()
	if importErr == nil {
		t.Fatal("ImportSSHConfig succeeded with a blocked profile replacement")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatal("connections file changed after failed batch write")
	}
	connections, err := store.List()
	if err != nil || len(connections) != 1 || connections[0].ID != existing.ID {
		t.Fatalf("connections = %+v, err = %v", connections, err)
	}
}

func blockProfileReplacement(t *testing.T, directory, path string) func() {
	t.Helper()
	target, blockedMode, restoredMode := directory, os.FileMode(0o500), os.FileMode(0o700)
	if runtime.GOOS == "windows" {
		// Windows ignores the read-only attribute on directories, but a
		// read-only destination file prevents MoveFileEx from replacing it.
		target, blockedMode, restoredMode = path, 0o400, 0o600
	}
	if err := os.Chmod(target, blockedMode); err != nil {
		t.Fatal(err)
	}
	restore := func() {
		t.Helper()
		if err := os.Chmod(target, restoredMode); err != nil {
			t.Errorf("restore profile write permissions: %v", err)
		}
	}
	t.Cleanup(restore)
	return restore
}

func TestSavePreservesExistingProvenanceAndClearsNewProvenance(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "connections.json"))
	imported, _, err := store.ImportSSHConfig([]Connection{importedConnection("alpha", "alpha.example")}, false)
	if err != nil {
		t.Fatal(err)
	}
	updated := imported[0]
	updated.Name = "Edited"
	updated.Provenance = nil
	saved, err := store.Save(updated)
	if err != nil || saved.Provenance == nil || saved.Provenance.SSHConfigAlias != "alpha" {
		t.Fatalf("saved = %+v, err = %v", saved, err)
	}
	created, err := store.Save(importedConnection("fake", "manual.example"))
	if err != nil || created.Provenance != nil {
		t.Fatalf("created = %+v, err = %v", created, err)
	}
}

func TestDeleteManyRemovesRequestedConnectionsInOneWrite(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "connections.json"))
	first, err := store.Save(Connection{Name: "First", Host: "first.example"})
	if err != nil {
		t.Fatal(err)
	}
	kept, err := store.Save(Connection{Name: "Kept", Host: "kept.example"})
	if err != nil {
		t.Fatal(err)
	}
	last, err := store.Save(Connection{Name: "Last", Host: "last.example"})
	if err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteMany([]string{first.ID, last.ID}); err != nil {
		t.Fatal(err)
	}
	connections, err := store.List()
	if err != nil || len(connections) != 1 || connections[0].ID != kept.ID {
		t.Fatalf("connections = %+v, err = %v", connections, err)
	}
}

func TestDeleteManyMissingIDPreservesConnections(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "connections.json"))
	first, err := store.Save(Connection{Name: "First", Host: "first.example"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Save(Connection{Name: "Second", Host: "second.example"})
	if err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteMany([]string{first.ID, "missing"}); err == nil {
		t.Fatal("DeleteMany succeeded with a missing ID")
	}
	connections, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(connections) != 2 {
		t.Fatalf("connections = %+v", connections)
	}
	if got := []string{connections[0].ID, connections[1].ID}; !reflect.DeepEqual(got, []string{first.ID, second.ID}) {
		t.Fatalf("connections = %+v", connections)
	}
}

func importedConnection(alias, host string) Connection {
	return Connection{
		Name:       alias,
		Host:       host,
		Folder:     "Imported",
		Provenance: &Provenance{SSHConfigAlias: alias},
	}
}

func TestJumpConnectionPersistenceAndRawCompatibility(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "connections.json"))
	hop, err := store.Save(Connection{Host: "hop.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := store.Save(Connection{Host: "raw-target.invalid", JumpHost: "user@jump.invalid:2222"})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.Save(Connection{Host: "saved-target.invalid", JumpConnectionID: " " + hop.ID + " "})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 3 || listed[1].JumpHost != raw.JumpHost || listed[1].JumpConnectionID != "" || listed[2].JumpConnectionID != hop.ID || saved.JumpConnectionID != hop.ID {
		t.Fatalf("jump settings did not round trip: %+v", listed)
	}
	if _, err := store.Save(Connection{Host: "target.invalid", JumpHost: "raw", JumpConnectionID: "saved"}); err == nil {
		t.Fatal("ambiguous jump settings accepted")
	}
}
