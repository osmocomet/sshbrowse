package buildinfo

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"runtime/debug"
)

const applicationDescription = "SSH session manager"

const defaultVersion = "0.0.0-dev"

// Version is replaced by release builds with the VERSION build input.
// Development binaries retain an explicit non-release value.
var Version = defaultVersion

// Commit is set to the full Git revision by release builds.
var Commit string

func VersionValue() string {
	if Version == "" {
		return defaultVersion
	}
	return Version
}

func Description() string {
	if Commit != "" {
		return description(VersionValue(), Commit, nil)
	}
	buildInfo, _ := debug.ReadBuildInfo()
	return description(VersionValue(), "", buildInfo)
}

func description(version, commit string, buildInfo *debug.BuildInfo) string {
	var revision string
	modified := false
	if commit != "" {
		revision = commit
	} else if buildInfo != nil {
		for _, setting := range buildInfo.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.modified":
				modified = setting.Value == "true"
			}
		}
	}
	result := applicationDescription + "\nVersion " + version
	if revision == "" {
		return result
	}
	if len(revision) > 7 {
		revision = revision[:7]
	}
	if modified {
		revision += " (modified)"
	}

	return result + "\nCommit " + revision
}

// InstanceID gives each executable location its own single-instance domain.
// This lets a development checkout and an installed copy share profiles while
// still allowing both applications to run during local validation.
func InstanceID(executablePath string) string {
	path := filepath.Clean(executablePath)
	if absolutePath, err := filepath.Abs(path); err == nil {
		path = absolutePath
	}
	hash := sha256.Sum256([]byte(path))
	return "local.sshbrowse.path.h" + hex.EncodeToString(hash[:])
}
