package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"sshbrowse/internal/profile"
	"sshbrowse/internal/sshcmd"
	"sshbrowse/internal/sshconfig"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const confirmDeleteTimeout = 10 * time.Minute

type SSHConfigCandidate struct {
	Alias           string `json:"alias"`
	AlreadyImported bool   `json:"alreadyImported"`
}

type SSHConfigIssue struct {
	Source  string `json:"source"`
	Line    int    `json:"line"`
	Message string `json:"message"`
}

type SSHConfigScan struct {
	Candidates   []SSHConfigCandidate `json:"candidates"`
	Warnings     []SSHConfigIssue     `json:"warnings"`
	ShouldPrompt bool                 `json:"shouldPrompt"`
}

type SSHConfigImportRequest struct {
	Aliases          []string `json:"aliases"`
	Folder           string   `json:"folder"`
	AnswerOnboarding bool     `json:"answerOnboarding"`
}

type SSHConfigImportResult struct {
	Imported        int `json:"imported"`
	AlreadyImported int `json:"alreadyImported"`
}

// Connections is bound to the frontend: the saved connections CRUD, plus
// parsing of typed destinations for unsaved ones.
type Connections struct {
	store *profile.Store
	app   *application.App
}

func NewConnections(store *profile.Store, app *application.App) *Connections {
	return &Connections{store: store, app: app}
}

func (c *Connections) List() ([]profile.Connection, error) {
	return c.store.List()
}

func (c *Connections) ListFolders() ([]string, error) {
	return c.store.Folders()
}

// Save creates the connection when its id is empty, otherwise updates it.
func (c *Connections) Save(connection profile.Connection) (profile.Connection, error) {
	return c.store.Save(connection)
}

func (c *Connections) Delete(id string) error {
	return c.store.Delete(id)
}

func (c *Connections) DeleteMany(ids []string) error {
	return c.store.DeleteMany(ids)
}

// Move puts the connections into a folder and creates missing ancestors.
func (c *Connections) Move(ids []string, folder string) error {
	return c.store.Move(ids, folder)
}

// Reorder places selected connections before or after a target connection.
func (c *Connections) Reorder(ids []string, targetID string, after bool) error {
	return c.store.Reorder(ids, targetID, after)
}

func (c *Connections) CreateFolder(path string) error {
	return c.store.CreateFolder(path)
}

func (c *Connections) RenameFolder(path, newPath string) error {
	return c.store.RenameFolder(path, newPath)
}

func (c *Connections) DeleteFolder(path string) error {
	return c.store.DeleteFolder(path)
}

// ChooseIdentityFile opens the native file picker and returns the selected
// path. The key file is never read by the application.
func (c *Connections) ChooseIdentityFile() (string, error) {
	return c.app.Dialog.OpenFile().
		SetTitle("Choose identity file").
		CanChooseFiles(true).
		CanChooseDirectories(false).
		AttachToWindow(c.app.Window.Current()).
		PromptForSingleSelection()
}

// ConfirmDelete asks in a native dialog and reports whether the user chose
// Delete. Show returns before the dialog closes, so the answer arrives through
// the button callbacks. Delete is the default as in Finder's own delete
// alerts; Escape triggers Cancel.
func (c *Connections) ConfirmDelete(names []string) bool {
	title := fmt.Sprintf("Delete %d connections?", len(names))
	if len(names) == 1 {
		title = fmt.Sprintf("Delete %q?", names[0])
	}
	answer := make(chan bool, 1)
	dialog := c.app.Dialog.Warning().
		SetTitle(title).
		SetMessage("This cannot be undone.").
		AttachToWindow(c.app.Window.Current())
	dialog.AddButton("Cancel").SetAsCancel().OnClick(func() { answer <- false })
	dialog.AddButton("Delete").SetAsDefault().OnClick(func() { answer <- true })
	dialog.Show()
	// A dialog left open this long is treated as cancelled; a later click then
	// lands in the buffered channel and is ignored.
	select {
	case confirmed := <-answer:
		return confirmed
	case <-time.After(confirmDeleteTimeout):
		return false
	}
}

// Parse turns "[user@]host[:port]" into an unsaved connection.
func (c *Connections) Parse(destination string) (profile.Connection, error) {
	return sshcmd.ParseDestination(destination)
}

// ScanSSHConfig lists the Host aliases in ~/.ssh/config and its includes
// without writing either SSH configuration or managed profiles.
func (c *Connections) ScanSSHConfig() (SSHConfigScan, error) {
	result, err := scanDefaultSSHConfig()
	if err != nil {
		return SSHConfigScan{}, err
	}
	connections, err := c.store.List()
	if err != nil {
		return SSHConfigScan{}, err
	}
	state, err := c.store.Onboarding()
	if err != nil {
		return SSHConfigScan{}, err
	}
	return makeSSHConfigScan(result, connections, state), nil
}

// ImportSSHConfig creates one connection per selected alias, with the alias as
// its host and nothing else, so ssh resolves the rest from the config at
// connect time. It rescans first so a stale UI selection cannot import an
// alias that no longer exists. The store performs one batch write.
func (c *Connections) ImportSSHConfig(request SSHConfigImportRequest) (SSHConfigImportResult, error) {
	result, err := scanDefaultSSHConfig()
	if err != nil {
		return SSHConfigImportResult{}, err
	}
	folder, err := c.importFolder(request.Folder)
	if err != nil {
		return SSHConfigImportResult{}, err
	}
	selected := make(map[string]bool, len(request.Aliases))
	for _, alias := range request.Aliases {
		selected[strings.ToLower(alias)] = true
	}
	profiles := make([]profile.Connection, 0, len(selected))
	for _, candidate := range result.Candidates {
		if !selected[strings.ToLower(candidate.Alias)] {
			continue
		}
		profiles = append(profiles, profile.Connection{
			Folder:     folder,
			Name:       candidate.Alias,
			Host:       candidate.Alias,
			Provenance: &profile.Provenance{SSHConfigAlias: candidate.Alias},
		})
		delete(selected, strings.ToLower(candidate.Alias))
	}
	if len(selected) > 0 {
		return SSHConfigImportResult{}, fmt.Errorf("selected SSH config aliases are no longer eligible")
	}
	imported, alreadyImported, err := c.store.ImportSSHConfig(profiles, request.AnswerOnboarding)
	if err != nil {
		return SSHConfigImportResult{}, err
	}
	return SSHConfigImportResult{Imported: len(imported), AlreadyImported: alreadyImported}, nil
}

func (c *Connections) AnswerSSHConfigImport() error {
	return c.store.AnswerSSHConfigImport()
}

func scanDefaultSSHConfig() (sshconfig.Result, error) {
	options, err := sshconfig.DefaultOptions()
	if err != nil {
		return sshconfig.Result{}, err
	}
	return sshconfig.Scan(filepath.Join(options.HomeDir, ".ssh", "config"), options), nil
}

func makeSSHConfigScan(result sshconfig.Result, connections []profile.Connection, state profile.OnboardingState) SSHConfigScan {
	importedAliases := make(map[string]bool)
	for _, connection := range connections {
		if connection.Provenance != nil {
			importedAliases[strings.ToLower(connection.Provenance.SSHConfigAlias)] = true
		}
	}
	candidates := make([]SSHConfigCandidate, 0, len(result.Candidates))
	newCandidates := 0
	for _, candidate := range result.Candidates {
		alreadyImported := importedAliases[strings.ToLower(candidate.Alias)]
		if !alreadyImported {
			newCandidates++
		}
		candidates = append(candidates, SSHConfigCandidate{Alias: candidate.Alias, AlreadyImported: alreadyImported})
	}
	return SSHConfigScan{
		Candidates:   candidates,
		Warnings:     issues(result.Warnings),
		ShouldPrompt: !state.SSHConfigImportAnswered && newCandidates > 0,
	}
}

func issues(source []sshconfig.Issue) []SSHConfigIssue {
	converted := make([]SSHConfigIssue, 0, len(source))
	for _, issue := range source {
		converted = append(converted, SSHConfigIssue{Source: issue.Source, Line: issue.Line, Message: issue.Message})
	}
	return converted
}

func (c *Connections) importFolder(requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return "Imported", nil
	}
	return profile.NormalizeFolderPath(requested)
}
