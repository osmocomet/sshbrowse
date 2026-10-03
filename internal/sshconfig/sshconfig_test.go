package sshconfig

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestScanFixture(t *testing.T) {
	// The fixture is copied into a fake ~/.ssh because its relative Includes
	// resolve against that directory, exactly as OpenSSH does.
	home, sshDir := sshHome(t)
	if err := os.CopyFS(sshDir, os.DirFS(filepath.Join("testdata", "basic"))); err != nil {
		t.Fatal(err)
	}
	result := Scan(filepath.Join(sshDir, "config"), Options{HomeDir: home})
	if len(result.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", result.Warnings)
	}
	if aliases := candidateAliases(result.Candidates); !reflect.DeepEqual(aliases, []string{"nested", "first", "alpha", "beta"}) {
		t.Fatalf("aliases = %v", aliases)
	}
	alpha := result.Candidates[2]
	if alpha.Source != filepath.Join(sshDir, "config") || alpha.Line != 5 {
		t.Fatalf("alpha location = %+v", alpha)
	}
}

func TestWildcardsAndNegationsAreNotAliases(t *testing.T) {
	result := Scan(writeConfig(t, "Host * !prod-jump\nHost *-jump 10.86.*\nHost lab-1 lab-?\n"), Options{})
	if aliases := candidateAliases(result.Candidates); !reflect.DeepEqual(aliases, []string{"lab-1"}) {
		t.Fatalf("aliases = %v", aliases)
	}
}

func TestRepeatedAliasIsListedOnceAtFirstDeclaration(t *testing.T) {
	path := writeConfig(t, "Host leaf-1\n HostName 192.0.2.11\nHost LEAF-1 leaf-2\n User network\n")
	result := Scan(path, Options{})
	want := []Candidate{{Alias: "leaf-1", Source: path, Line: 1}, {Alias: "leaf-2", Source: path, Line: 3}}
	if !reflect.DeepEqual(result.Candidates, want) {
		t.Fatalf("candidates = %+v, want %+v", result.Candidates, want)
	}
}

func TestIncludeInsideHostBlockIsFollowed(t *testing.T) {
	home, directory := sshHome(t)
	writeFile(t, filepath.Join(directory, "config"), "Host outer\n Include child\n")
	writeFile(t, filepath.Join(directory, "child"), "Host inner\n")
	result := Scan(filepath.Join(directory, "config"), Options{HomeDir: home})
	if aliases := candidateAliases(result.Candidates); !reflect.DeepEqual(aliases, []string{"outer", "inner"}) {
		t.Fatalf("aliases = %v, warnings = %v", aliases, result.Warnings)
	}
}

func TestUnterminatedQuoteWarnsAndParsingContinues(t *testing.T) {
	result := Scan(writeConfig(t, "Host \"broken\nHost fine\n"), Options{})
	if aliases := candidateAliases(result.Candidates); !reflect.DeepEqual(aliases, []string{"fine"}) {
		t.Fatalf("aliases = %v", aliases)
	}
	if !hasWarning(result.Warnings, "unterminated quoted value") {
		t.Fatalf("warnings = %v", result.Warnings)
	}
}

func TestIncludeCycleWarnsAndMissingFileIsSilent(t *testing.T) {
	home, directory := sshHome(t)
	root := filepath.Join(directory, "config")
	child := filepath.Join(directory, "child")
	writeFile(t, root, "Include child missing\nHost root\n")
	writeFile(t, child, "Include config\nHost child\n")

	result := Scan(root, Options{HomeDir: home})
	if aliases := candidateAliases(result.Candidates); !reflect.DeepEqual(aliases, []string{"child", "root"}) {
		t.Fatalf("candidates = %v", aliases)
	}
	if !hasWarning(result.Warnings, "include cycle") || hasWarning(result.Warnings, "missing") {
		t.Fatalf("warnings = %v", result.Warnings)
	}
}

func TestIncludePathForms(t *testing.T) {
	home, sshDir := sshHome(t)
	homeInclude := filepath.Join(home, "home.conf")
	absoluteInclude := filepath.Join(t.TempDir(), "absolute.conf")
	relativeInclude := filepath.Join(sshDir, "relative.conf")
	writeFile(t, homeInclude, "Host home\n")
	writeFile(t, absoluteInclude, "Host absolute\n")
	writeFile(t, relativeInclude, "Host relative\n")
	// The root lives outside ~/.ssh so a relative Include resolving against the
	// including file's directory would miss relative.conf.
	root := filepath.Join(t.TempDir(), "config")
	// Windows treats backslashes in an unquoted Include token as escapes. The
	// quoted, slash-normalized form is valid OpenSSH syntax on every platform
	// and also works when the temporary directory contains spaces.
	absoluteIncludeToken := filepath.ToSlash(absoluteInclude)
	writeFile(t, root, "Include ~/home.conf \""+absoluteIncludeToken+"\" relative.conf\n")

	result := Scan(root, Options{HomeDir: home})
	if aliases := candidateAliases(result.Candidates); !reflect.DeepEqual(aliases, []string{"home", "absolute", "relative"}) {
		t.Fatalf("candidates = %v, warnings = %v", aliases, result.Warnings)
	}

	withoutHome := Scan(root, Options{})
	if aliases := candidateAliases(withoutHome.Candidates); !reflect.DeepEqual(aliases, []string{"absolute"}) {
		t.Fatalf("candidates without home = %v", aliases)
	}
	if !hasWarning(withoutHome.Warnings, "home directory is unavailable") {
		t.Fatalf("warnings without home = %v", withoutHome.Warnings)
	}
}

func TestConfiguredBounds(t *testing.T) {
	t.Run("depth", func(t *testing.T) {
		home, directory := sshHome(t)
		writeFile(t, filepath.Join(directory, "config"), "Include one\nHost root\n")
		writeFile(t, filepath.Join(directory, "one"), "Include two\nHost one\n")
		writeFile(t, filepath.Join(directory, "two"), "Host two\n")
		result := Scan(filepath.Join(directory, "config"), Options{HomeDir: home, MaxIncludeDepth: 1})
		if !hasWarning(result.Warnings, "include depth") {
			t.Fatalf("warnings = %v", result.Warnings)
		}
	})

	t.Run("files", func(t *testing.T) {
		home, directory := sshHome(t)
		writeFile(t, filepath.Join(directory, "config"), "Include one two\nHost root\n")
		writeFile(t, filepath.Join(directory, "one"), "Host one\n")
		writeFile(t, filepath.Join(directory, "two"), "Host two\n")
		result := Scan(filepath.Join(directory, "config"), Options{HomeDir: home, MaxFiles: 2})
		if !hasWarning(result.Warnings, "file count") {
			t.Fatalf("warnings = %v", result.Warnings)
		}
	})

	t.Run("size", func(t *testing.T) {
		result := Scan(writeConfig(t, "Host far-too-large\n"), Options{MaxFileSize: 8})
		if !hasWarning(result.Warnings, "size limit") || len(result.Candidates) != 0 {
			t.Fatalf("result = %+v", result)
		}
	})
}

func TestSystemConfigurationIsNeverRead(t *testing.T) {
	result := Scan(systemConfigPath, Options{})
	if len(result.Candidates) != 0 || !hasWarning(result.Warnings, "system SSH configuration ignored") {
		t.Fatalf("result = %+v", result)
	}
}

func candidateAliases(candidates []Candidate) []string {
	aliases := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		aliases = append(aliases, candidate.Alias)
	}
	return aliases
}

func hasWarning(warnings []Issue, text string) bool {
	return slices.ContainsFunc(warnings, func(issue Issue) bool {
		return strings.Contains(issue.Message, text) || strings.Contains(issue.Source, text)
	})
}

// sshHome creates a fake home with an empty .ssh directory, which is where
// relative Include paths resolve.
func sshHome(t *testing.T) (home, sshDir string) {
	t.Helper()
	home = t.TempDir()
	sshDir = filepath.Join(home, ".ssh")
	if err := os.Mkdir(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	return home, sshDir
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	writeFile(t, path, content)
	return path
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
