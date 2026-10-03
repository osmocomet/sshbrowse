package main

import (
	"embed"
	"fmt"
	"io"
)

//go:embed NOTICE docs/legal/THIRD_PARTY_NOTICES.txt
var licenseDocuments embed.FS

func writeLicenseDocuments(args []string, output io.Writer) (bool, error) {
	if len(args) != 2 || args[1] != "--licenses" {
		return false, nil
	}

	documents := []struct {
		path string
		name string
	}{
		{path: "NOTICE", name: "NOTICE"},
		{path: "docs/legal/THIRD_PARTY_NOTICES.txt", name: "THIRD_PARTY_NOTICES.txt"},
	}
	for _, document := range documents {
		content, err := licenseDocuments.ReadFile(document.path)
		if err != nil {
			return true, fmt.Errorf("read embedded %s: %w", document.name, err)
		}
		if _, err := fmt.Fprintf(output, "===== %s =====\n", document.name); err != nil {
			return true, err
		}
		if _, err := output.Write(content); err != nil {
			return true, err
		}
		if len(content) == 0 || content[len(content)-1] != '\n' {
			if _, err := io.WriteString(output, "\n"); err != nil {
				return true, err
			}
		}
		if _, err := io.WriteString(output, "\n"); err != nil {
			return true, err
		}
	}
	return true, nil
}
