// Package profile stores saved connections in one JSON file.
package profile

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"

	"sshbrowse/internal/atomicfile"
)

const (
	configDirectoryName    = "sshbrowse"
	maxConnectionsFileSize = 1 * 1024 * 1024
)

const (
	currentFileVersion = 1
	maxFolderPathBytes = 255
)

type Connection struct {
	ID               string      `json:"id"`     // empty for a connection that is not saved
	Folder           string      `json:"folder"` // normalized sidebar path; empty means root
	Name             string      `json:"name"`
	Host             string      `json:"host"`
	User             string      `json:"user"`
	Port             int         `json:"port"` // 0 means ssh's default
	IdentityFile     string      `json:"identityFile"`
	JumpHost         string      `json:"jumpHost"` // raw OpenSSH ProxyJump; retained for compatibility
	JumpConnectionID string      `json:"jumpConnectionId,omitempty"`
	AgentForwarding  bool        `json:"agentForwarding"`
	X11Forwarding    bool        `json:"x11Forwarding"`
	LocalForwards    []string    `json:"localForwards"`   // ssh -L specs, e.g. "8080:localhost:80"
	RemoteForwards   []string    `json:"remoteForwards"`  // ssh -R specs
	DynamicForwards  []string    `json:"dynamicForwards"` // ssh -D specs, e.g. "1080"
	Provenance       *Provenance `json:"provenance,omitempty"`
}

// Provenance marks a connection imported from ssh_config, so a rescan does not
// offer the same alias again. It has no effect on how the connection runs.
type Provenance struct {
	SSHConfigAlias string `json:"sshConfigAlias"`
}

type OnboardingState struct {
	SSHConfigImportAnswered bool `json:"sshConfigImportAnswered,omitempty"`
}

type fileFormat struct {
	Version     int             `json:"version"`
	Connections []Connection    `json:"connections"`
	Folders     []string        `json:"folders"`
	Onboarding  OnboardingState `json:"onboarding,omitempty"`
}

// Store reads and writes the connections file. The file is re-read on every
// call so hand edits are picked up; operations also hold a sidecar advisory
// lock so separate app instances cannot lose read-modify-write updates.
type Store struct {
	path string
	mu   sync.Mutex
}

// DefaultPath is the platform's user config directory plus sshbrowse/connections.json.
func DefaultPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, configDirectoryName, "connections.json"), nil
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

// List returns connections in file order, which is also display order.
func (s *Store) List() ([]Connection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lockFile, err := acquireFileLock(s.path)
	if err != nil {
		return nil, err
	}
	defer releaseFileLock(lockFile)
	file, err := s.load()
	return file.Connections, err
}

// Folders returns folder paths in sidebar order. Empty folders are included.
func (s *Store) Folders() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lockFile, err := acquireFileLock(s.path)
	if err != nil {
		return nil, err
	}
	defer releaseFileLock(lockFile)
	file, err := s.load()
	return file.Folders, err
}

// Save appends a connection when its ID is empty, otherwise replaces the
// connection with that ID in place.
func (s *Store) Save(connection Connection) (Connection, error) {
	var err error
	connection, err = normalise(connection)
	if err != nil {
		return Connection{}, err
	}
	if connection.Host == "" {
		return Connection{}, errors.New("host is required")
	}
	if connection.Name == "" {
		connection.Name = connection.Host
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	lockFile, err := acquireFileLock(s.path)
	if err != nil {
		return Connection{}, err
	}
	defer releaseFileLock(lockFile)
	file, err := s.load()
	if err != nil {
		return Connection{}, err
	}

	jumpChanged := connection.ID == ""
	if connection.ID == "" {
		connection.Provenance = nil
		connection.ID, err = newID()
		if err != nil {
			return Connection{}, err
		}
		file.Connections = append(file.Connections, connection)
	} else {
		replaced := false
		for i := range file.Connections {
			if file.Connections[i].ID == connection.ID {
				jumpChanged = file.Connections[i].JumpConnectionID != connection.JumpConnectionID
				connection.Provenance = file.Connections[i].Provenance
				file.Connections[i] = connection
				replaced = true
			}
		}
		if !replaced {
			return Connection{}, fmt.Errorf("connection %q not found", connection.ID)
		}
	}
	if jumpChanged {
		if err := validateJumpUpdate(connection.ID, file.Connections); err != nil {
			return Connection{}, err
		}
	}
	file.Folders = addFolderAndAncestors(file.Folders, connection.Folder)
	return connection, s.write(file)
}

func (s *Store) Delete(id string) error {
	return s.DeleteMany([]string{id})
}

// DeleteMany removes all requested connections in one atomic file replacement.
func (s *Store) DeleteMany(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lockFile, err := acquireFileLock(s.path)
	if err != nil {
		return err
	}
	defer releaseFileLock(lockFile)
	file, err := s.load()
	if err != nil {
		return err
	}
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	kept := make([]Connection, 0, len(file.Connections))
	for _, connection := range file.Connections {
		if !wanted[connection.ID] {
			kept = append(kept, connection)
		}
	}
	removed := len(file.Connections) - len(kept)
	if removed != len(wanted) {
		return fmt.Errorf("%d of %d connections not found", len(wanted)-removed, len(wanted))
	}
	file.Connections = kept
	return s.write(file)
}

// Move puts the connections into folder, "" being root, in one write.
func (s *Store) Move(ids []string, folder string) error {
	folder, err := NormalizeFolderPath(folder)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lockFile, err := acquireFileLock(s.path)
	if err != nil {
		return err
	}
	defer releaseFileLock(lockFile)
	file, err := s.load()
	if err != nil {
		return err
	}
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	moved := 0
	for i := range file.Connections {
		if wanted[file.Connections[i].ID] {
			file.Connections[i].Folder = folder
			moved++
		}
	}
	if moved != len(wanted) {
		return fmt.Errorf("%d of %d connections not found", len(wanted)-moved, len(wanted))
	}
	file.Folders = addFolderAndAncestors(file.Folders, folder)
	return s.write(file)
}

// Reorder moves the selected connections next to targetID in the supplied
// order. The selected connections join the target's folder.
func (s *Store) Reorder(ids []string, targetID string, after bool) error {
	if len(ids) == 0 {
		return errors.New("at least one connection is required")
	}
	if targetID == "" {
		return errors.New("target connection is required")
	}
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" {
			return errors.New("connection IDs are required")
		}
		if wanted[id] {
			return fmt.Errorf("connection %q was selected more than once", id)
		}
		if id == targetID {
			return errors.New("target connection cannot be selected")
		}
		wanted[id] = true
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	lockFile, err := acquireFileLock(s.path)
	if err != nil {
		return err
	}
	defer releaseFileLock(lockFile)
	file, err := s.load()
	if err != nil {
		return err
	}

	var target Connection
	targetFound := false
	for _, connection := range file.Connections {
		if connection.ID == targetID {
			target = connection
			targetFound = true
			break
		}
	}
	if !targetFound {
		return fmt.Errorf("connection %q not found", targetID)
	}
	selectedByID := make(map[string]Connection, len(ids))
	remaining := make([]Connection, 0, len(file.Connections))
	for _, connection := range file.Connections {
		if wanted[connection.ID] {
			connection.Folder = target.Folder
			selectedByID[connection.ID] = connection
			continue
		}
		remaining = append(remaining, connection)
	}
	if len(selectedByID) != len(wanted) {
		return fmt.Errorf("%d of %d connections not found", len(wanted)-len(selectedByID), len(wanted))
	}
	selected := make([]Connection, 0, len(ids))
	for _, id := range ids {
		selected = append(selected, selectedByID[id])
	}

	// Resolve the target position after removing the selected connections.
	// The target itself is never selected, so it remains in this slice.
	targetIndex := -1
	for i, connection := range remaining {
		if connection.ID == targetID {
			targetIndex = i
			break
		}
	}
	if after {
		targetIndex++
	}
	reordered := make([]Connection, 0, len(file.Connections))
	reordered = append(reordered, remaining[:targetIndex]...)
	reordered = append(reordered, selected...)
	reordered = append(reordered, remaining[targetIndex:]...)
	file.Connections = reordered
	return s.write(file)
}

// CreateFolder adds a folder path and any missing ancestors.
func (s *Store) CreateFolder(path string) error {
	path, err := NormalizeFolderPath(path)
	if err != nil {
		return err
	}
	if path == "" {
		return errors.New("folder path is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lockFile, err := acquireFileLock(s.path)
	if err != nil {
		return err
	}
	defer releaseFileLock(lockFile)
	file, err := s.load()
	if err != nil {
		return err
	}
	if containsFolder(file.Folders, path) {
		return fmt.Errorf("folder %q already exists", path)
	}
	file.Folders = addFolderAndAncestors(file.Folders, path)
	return s.write(file)
}

// RenameFolder renames a folder and its complete subtree in one write.
func (s *Store) RenameFolder(path, newPath string) error {
	path, err := NormalizeFolderPath(path)
	if err != nil {
		return fmt.Errorf("current folder: %w", err)
	}
	newPath, err = NormalizeFolderPath(newPath)
	if err != nil {
		return fmt.Errorf("new folder: %w", err)
	}
	if path == "" || newPath == "" {
		return errors.New("folder paths are required")
	}
	if path == newPath {
		return errors.New("new folder path must be different")
	}
	if isInFolder(newPath, path) {
		return errors.New("a folder cannot be moved into its own subtree")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	lockFile, err := acquireFileLock(s.path)
	if err != nil {
		return err
	}
	defer releaseFileLock(lockFile)
	file, err := s.load()
	if err != nil {
		return err
	}
	if !containsFolder(file.Folders, path) {
		return fmt.Errorf("folder %q not found", path)
	}
	existing := make(map[string]bool, len(file.Folders))
	for _, folder := range file.Folders {
		if !isInFolder(folder, path) {
			existing[folder] = true
		}
	}
	for _, folder := range file.Folders {
		if !isInFolder(folder, path) {
			continue
		}
		renamed := newPath + strings.TrimPrefix(folder, path)
		if _, err := NormalizeFolderPath(renamed); err != nil {
			return fmt.Errorf("renamed folder %q: %w", folder, err)
		}
		if existing[renamed] {
			return fmt.Errorf("folder %q already exists", renamed)
		}
	}

	renamedFolders := make([]string, 0, len(file.Folders))
	insertedAncestors := false
	for _, folder := range file.Folders {
		if isInFolder(folder, path) {
			if !insertedAncestors {
				renamedFolders = addParentFolders(renamedFolders, newPath)
				insertedAncestors = true
			}
			renamedFolders = append(renamedFolders, newPath+strings.TrimPrefix(folder, path))
			continue
		}
		renamedFolders = appendUniqueFolder(renamedFolders, folder)
	}
	for i := range file.Connections {
		if isInFolder(file.Connections[i].Folder, path) {
			file.Connections[i].Folder = newPath + strings.TrimPrefix(file.Connections[i].Folder, path)
		}
	}
	file.Folders = renamedFolders
	return s.write(file)
}

// DeleteFolder removes a folder only when it contains no connections or subfolders.
func (s *Store) DeleteFolder(path string) error {
	path, err := NormalizeFolderPath(path)
	if err != nil {
		return err
	}
	if path == "" {
		return errors.New("the root folder cannot be deleted")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lockFile, err := acquireFileLock(s.path)
	if err != nil {
		return err
	}
	defer releaseFileLock(lockFile)
	file, err := s.load()
	if err != nil {
		return err
	}
	if !containsFolder(file.Folders, path) {
		return fmt.Errorf("folder %q not found", path)
	}
	for _, folder := range file.Folders {
		if folder != path && isInFolder(folder, path) {
			return fmt.Errorf("folder %q contains subfolders", path)
		}
	}
	for _, connection := range file.Connections {
		if connection.Folder == path {
			return fmt.Errorf("folder %q contains connections", path)
		}
	}
	kept := make([]string, 0, len(file.Folders)-1)
	for _, folder := range file.Folders {
		if folder != path {
			kept = append(kept, folder)
		}
	}
	file.Folders = kept
	return s.write(file)
}

func (s *Store) Onboarding() (OnboardingState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lockFile, err := acquireFileLock(s.path)
	if err != nil {
		return OnboardingState{}, err
	}
	defer releaseFileLock(lockFile)
	file, err := s.load()
	return file.Onboarding, err
}

func (s *Store) AnswerSSHConfigImport() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lockFile, err := acquireFileLock(s.path)
	if err != nil {
		return err
	}
	defer releaseFileLock(lockFile)
	file, err := s.load()
	if err != nil {
		return err
	}
	if file.Onboarding.SSHConfigImportAnswered {
		return nil
	}
	file.Onboarding.SSHConfigImportAnswered = true
	return s.write(file)
}

// ImportSSHConfig adds all connections with one atomic file replacement. IDs
// are assigned before the write; an error leaves every existing profile intact.
func (s *Store) ImportSSHConfig(connections []Connection, answerOnboarding bool) ([]Connection, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lockFile, err := acquireFileLock(s.path)
	if err != nil {
		return nil, 0, err
	}
	defer releaseFileLock(lockFile)
	file, err := s.load()
	if err != nil {
		return nil, 0, err
	}

	aliases := make(map[string]bool)
	for _, connection := range file.Connections {
		if connection.Provenance != nil {
			aliases[strings.ToLower(connection.Provenance.SSHConfigAlias)] = true
		}
	}
	imported := make([]Connection, 0, len(connections))
	alreadyImported := 0
	for _, connection := range connections {
		connection, err = normalise(connection)
		if err != nil {
			return nil, 0, err
		}
		if connection.Provenance == nil || connection.Provenance.SSHConfigAlias == "" {
			return nil, 0, errors.New("SSH config alias is required")
		}
		aliasKey := strings.ToLower(connection.Provenance.SSHConfigAlias)
		if aliases[aliasKey] {
			alreadyImported++
			continue
		}
		if connection.Host == "" {
			return nil, 0, fmt.Errorf("host is required for %q", connection.Provenance.SSHConfigAlias)
		}
		if connection.Name == "" {
			connection.Name = connection.Host
		}
		connection.ID, err = newID()
		if err != nil {
			return nil, 0, err
		}
		aliases[aliasKey] = true
		imported = append(imported, connection)
		file.Folders = addFolderAndAncestors(file.Folders, connection.Folder)
	}
	file.Connections = append(file.Connections, imported...)
	if answerOnboarding {
		file.Onboarding.SSHConfigImportAnswered = true
	}
	if len(imported) == 0 && !answerOnboarding {
		return imported, alreadyImported, nil
	}
	if err := s.write(file); err != nil {
		return nil, 0, err
	}
	return imported, alreadyImported, nil
}

func (s *Store) load() (fileFormat, error) {
	file, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return fileFormat{Version: currentFileVersion, Connections: []Connection{}, Folders: []string{}}, nil
	}
	if err != nil {
		return fileFormat{}, err
	}
	defer file.Close() // Read-only file; close cannot affect stored profiles.
	data, err := io.ReadAll(io.LimitReader(file, maxConnectionsFileSize+1))
	if err != nil {
		return fileFormat{}, fmt.Errorf("profile: read %s: %w", s.path, err)
	}
	if len(data) > maxConnectionsFileSize {
		return fileFormat{}, fmt.Errorf("profile: %s exceeds the 1 MiB size limit", s.path)
	}
	var parsed fileFormat
	if err := json.Unmarshal(data, &parsed); err != nil {
		return fileFormat{}, fmt.Errorf("profile: parse %s: %w", s.path, err)
	}
	if parsed.Version != currentFileVersion {
		return fileFormat{}, fmt.Errorf("profile: unsupported file version %d", parsed.Version)
	}
	for i := range parsed.Connections {
		parsed.Connections[i], err = normalise(parsed.Connections[i])
		if err != nil {
			return fileFormat{}, fmt.Errorf("profile: connection %q: %w", parsed.Connections[i].ID, err)
		}
	}
	if parsed.Connections == nil {
		parsed.Connections = []Connection{}
	}
	normalizedFolders := make([]string, 0, len(parsed.Folders))
	for _, folder := range parsed.Folders {
		folder, err = NormalizeFolderPath(folder)
		if err != nil {
			return fileFormat{}, fmt.Errorf("profile: folder: %w", err)
		}
		normalizedFolders = addFolderAndAncestors(normalizedFolders, folder)
	}
	for _, connection := range parsed.Connections {
		normalizedFolders = addFolderAndAncestors(normalizedFolders, connection.Folder)
	}
	parsed.Folders = normalizedFolders
	return parsed, nil
}

// write replaces the file through a unique temp file and rename, so a process
// crash mid-write leaves the previous file intact rather than a truncated one.
// Callers hold the profile lock; there is no fsync, so a power loss at that
// moment can still lose the write.
func (s *Store) write(file fileFormat) error {
	file.Version = currentFileVersion
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}

	if len(data) >= maxConnectionsFileSize {
		return fmt.Errorf("profile: serialized %s exceeds the 1 MiB size limit", s.path)
	}

	serialized := make([]byte, len(data)+1)
	copy(serialized, data)
	serialized[len(data)] = '\n'

	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	return atomicfile.WriteReplace(s.path, serialized, 0o600)
}

// normalise trims text fields and turns nil slices into empty ones so the JSON
// always has arrays, never null.
func normalise(connection Connection) (Connection, error) {
	var err error
	connection.Folder, err = NormalizeFolderPath(connection.Folder)
	if err != nil {
		return Connection{}, err
	}
	connection.Name = strings.TrimSpace(connection.Name)
	connection.Host = strings.TrimSpace(connection.Host)
	connection.User = strings.TrimSpace(connection.User)
	connection.IdentityFile = strings.TrimSpace(connection.IdentityFile)
	connection.JumpHost = strings.TrimSpace(connection.JumpHost)
	connection.JumpConnectionID = strings.TrimSpace(connection.JumpConnectionID)
	if connection.JumpHost != "" && connection.JumpConnectionID != "" {
		return Connection{}, errors.New("choose either a saved jump connection or raw ProxyJump")
	}
	connection.LocalForwards = cleanSpecs(connection.LocalForwards)
	connection.RemoteForwards = cleanSpecs(connection.RemoteForwards)
	connection.DynamicForwards = cleanSpecs(connection.DynamicForwards)
	if connection.Provenance != nil {
		connection.Provenance.SSHConfigAlias = strings.TrimSpace(connection.Provenance.SSHConfigAlias)
	}
	return connection, nil
}

// NormalizeFolderPath trims each segment and returns its slash-separated form.
// Empty segments, traversal names, control characters, and oversized paths are rejected.
func NormalizeFolderPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	parts := strings.Split(path, "/")
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return "", errors.New("folder path cannot contain empty segments")
		}
		if part == "." || part == ".." {
			return "", fmt.Errorf("folder segment %q is not allowed", part)
		}
		if strings.IndexFunc(part, unicode.IsControl) >= 0 {
			return "", errors.New("folder path cannot contain control characters")
		}
		parts[i] = part
	}
	normalized := strings.Join(parts, "/")
	if len(normalized) > maxFolderPathBytes {
		return "", fmt.Errorf("folder path exceeds %d bytes", maxFolderPathBytes)
	}
	return normalized, nil
}

func addFolderAndAncestors(folders []string, path string) []string {
	if path == "" {
		return folders
	}
	parts := strings.Split(path, "/")
	for i := range parts {
		folders = appendUniqueFolder(folders, strings.Join(parts[:i+1], "/"))
	}
	return folders
}

func addParentFolders(folders []string, path string) []string {
	if !strings.Contains(path, "/") {
		return folders
	}
	parts := strings.Split(path, "/")
	for i := 1; i < len(parts); i++ {
		folders = appendUniqueFolder(folders, strings.Join(parts[:i], "/"))
	}
	return folders
}

func appendUniqueFolder(folders []string, path string) []string {
	if path != "" && !containsFolder(folders, path) {
		return append(folders, path)
	}
	return folders
}

func containsFolder(folders []string, path string) bool {
	for _, folder := range folders {
		if folder == path {
			return true
		}
	}
	return false
}

func isInFolder(path, ancestor string) bool {
	return path == ancestor || strings.HasPrefix(path, ancestor+"/")
}

func cleanSpecs(specs []string) []string {
	cleaned := make([]string, 0, len(specs))
	for _, spec := range specs {
		spec = strings.TrimSpace(spec)
		if spec != "" {
			cleaned = append(cleaned, spec)
		}
	}
	return cleaned
}

func newID() (string, error) {
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(randomBytes), nil
}
