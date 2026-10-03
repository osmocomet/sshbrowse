package app

import (
	"testing"

	"sshbrowse/internal/profile"
	"sshbrowse/internal/sshconfig"
)

func TestMakeSSHConfigScanMarksImportedAliasesAndOnboarding(t *testing.T) {
	parsed := sshconfig.Result{Candidates: []sshconfig.Candidate{
		{Alias: "alpha"},
		{Alias: "beta"},
	}}
	connections := []profile.Connection{
		{Provenance: &profile.Provenance{SSHConfigAlias: "ALPHA"}},
	}

	scan := makeSSHConfigScan(parsed, connections, profile.OnboardingState{})
	if !scan.Candidates[0].AlreadyImported || scan.Candidates[1].AlreadyImported || !scan.ShouldPrompt {
		t.Fatalf("scan = %+v", scan)
	}

	scan = makeSSHConfigScan(parsed, connections, profile.OnboardingState{SSHConfigImportAnswered: true})
	if scan.ShouldPrompt {
		t.Fatal("answered onboarding prompted again")
	}
}

func TestImportFolderAcceptsAndNormalizesNewNestedPath(t *testing.T) {
	connections := NewConnections(nil, nil)
	got, err := connections.importFolder(" Customers / Acme ")
	if err != nil || got != "Customers/Acme" {
		t.Fatalf("importFolder = %q, %v", got, err)
	}
	got, err = connections.importFolder("")
	if err != nil || got != "Imported" {
		t.Fatalf("empty importFolder = %q, %v", got, err)
	}
	if _, err := connections.importFolder("Customers//Acme"); err == nil {
		t.Fatal("invalid import folder was accepted")
	}
}
