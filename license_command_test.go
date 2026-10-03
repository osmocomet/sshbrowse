package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestWriteLicenseDocuments(t *testing.T) {
	var output bytes.Buffer
	handled, err := writeLicenseDocuments([]string{"sshbrowse", "--licenses"}, &output)
	if err != nil {
		t.Fatalf("writeLicenseDocuments: %v", err)
	}
	if !handled {
		t.Fatal("--licenses was not handled")
	}
	notice, err := os.ReadFile("NOTICE")
	if err != nil {
		t.Fatal(err)
	}
	thirdPartyNotices, err := os.ReadFile("docs/legal/THIRD_PARTY_NOTICES.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("===== NOTICE =====\n%s\n===== THIRD_PARTY_NOTICES.txt =====\n%s\n", notice, thirdPartyNotices)
	if output.String() != want {
		t.Error("license output must contain the complete notice followed by the complete third-party notices")
	}
	for _, marker := range []string{
		"===== NOTICE =====",
		"Copyright © 2026. All rights reserved.",
		"SSHBrowse is free for personal, non-commercial use.",
		"Professional, workplace, or commercial use requires a paid license.",
		"You may clone, copy, build, inspect,",
		"Independent redistribution, resale, or publication of SSHBrowse, including",
		"===== THIRD_PARTY_NOTICES.txt =====",
		"## github.com/wailsapp/wails/v3",
		"Copyright (c) 2018-Present Lea Anthony",
		"## @xterm/xterm",
	} {
		if !strings.Contains(output.String(), marker) {
			t.Errorf("license output missing %q", marker)
		}
	}
}
