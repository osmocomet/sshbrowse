// Package sshconfig lists the Host aliases declared in OpenSSH user
// configuration. Nothing else is read from it: an imported connection runs as
// `ssh <alias>`, so ssh resolves every option itself.
package sshconfig

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	DefaultMaxIncludeDepth = 16
	DefaultMaxFiles        = 128
	DefaultMaxFileSize     = 2 * 1024 * 1024
)

type Options struct {
	HomeDir         string
	MaxIncludeDepth int
	MaxFiles        int
	MaxFileSize     int64
}

// Candidate is one literal Host alias and where it was first declared.
type Candidate struct {
	Alias  string
	Source string
	Line   int
}

type Issue struct {
	Source  string
	Line    int
	Message string
}

type Result struct {
	Candidates []Candidate
	Warnings   []Issue
}

type parser struct {
	options Options
	result  Result
	files   int
	stack   map[string]bool // files currently being read, for cycle detection
	seen    map[string]bool // lower-cased aliases already listed
}

// Scan starts at rootPath and returns the aliases plus non-fatal limitations.
// SSH configuration is only opened for reading.
func Scan(rootPath string, options Options) Result {
	options = withDefaults(options)
	parser := parser{options: options, stack: make(map[string]bool), seen: make(map[string]bool)}
	parser.parseFile(rootPath, 0)
	return parser.result
}

func DefaultOptions() (Options, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return Options{}, err
	}
	return withDefaults(Options{HomeDir: homeDir}), nil
}

func withDefaults(options Options) Options {
	if options.MaxIncludeDepth <= 0 {
		options.MaxIncludeDepth = DefaultMaxIncludeDepth
	}
	if options.MaxFiles <= 0 {
		options.MaxFiles = DefaultMaxFiles
	}
	if options.MaxFileSize <= 0 {
		options.MaxFileSize = DefaultMaxFileSize
	}
	return options
}

func (p *parser) parseFile(path string, depth int) {
	if depth > p.options.MaxIncludeDepth {
		p.warn(path, 0, fmt.Sprintf("include depth exceeds limit of %d", p.options.MaxIncludeDepth))
		return
	}
	if p.files >= p.options.MaxFiles {
		p.warn(path, 0, fmt.Sprintf("file count exceeds limit of %d", p.options.MaxFiles))
		return
	}

	cleanPath, err := filepath.Abs(path)
	if err != nil {
		p.warn(path, 0, fmt.Sprintf("resolve path: %v", err))
		return
	}
	cyclePath := cleanPath
	if resolved, resolveErr := filepath.EvalSymlinks(cleanPath); resolveErr == nil {
		cyclePath = resolved
	}
	if isSystemConfig(cleanPath, cyclePath) {
		p.warn(cleanPath, 0, "system SSH configuration ignored")
		return
	}
	if p.stack[cyclePath] {
		p.warn(cleanPath, 0, "include cycle ignored")
		return
	}

	fileInfo, err := os.Stat(cleanPath)
	if err != nil {
		// OpenSSH ignores a missing Include target silently.
		if !errors.Is(err, os.ErrNotExist) {
			p.warn(cleanPath, 0, fmt.Sprintf("read file: %v", err))
		}
		return
	}
	if !fileInfo.Mode().IsRegular() {
		p.warn(cleanPath, 0, "read file: not a regular file")
		return
	}
	if fileInfo.Size() > p.options.MaxFileSize {
		p.warn(cleanPath, 0, fmt.Sprintf("file exceeds size limit of %d bytes", p.options.MaxFileSize))
		return
	}
	file, err := os.Open(cleanPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			p.warn(cleanPath, 0, fmt.Sprintf("read file: %v", err))
		}
		return
	}
	defer file.Close()
	p.files++
	p.stack[cyclePath] = true
	defer delete(p.stack, cyclePath)

	data, err := io.ReadAll(io.LimitReader(file, p.options.MaxFileSize+1))
	if err != nil {
		p.warn(cleanPath, 0, fmt.Sprintf("read file: %v", err))
		return
	}
	if int64(len(data)) > p.options.MaxFileSize {
		p.warn(cleanPath, 0, fmt.Sprintf("file exceeds size limit of %d bytes", p.options.MaxFileSize))
		return
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	maxToken := int(min(p.options.MaxFileSize+1, int64(maxInt())))
	scanner.Buffer(make([]byte, 32*1024), maxToken)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSuffix(scanner.Text(), "\r")
		p.parseLine(cleanPath, lineNumber, line, depth)
	}
	if err := scanner.Err(); err != nil {
		p.warn(cleanPath, lineNumber, fmt.Sprintf("read file: %v", err))
	}
}

func isSystemConfig(cleanPath, resolvedPath string) bool {
	for _, systemPath := range systemConfigPaths() {
		if sameConfigPath(cleanPath, systemPath) || sameConfigPath(resolvedPath, systemPath) {
			return true
		}
		systemResolved, err := filepath.EvalSymlinks(systemPath)
		if err == nil && sameConfigPath(resolvedPath, systemResolved) {
			return true
		}
	}
	return false
}

// parseLine looks at Host and Include only. Include is textual in OpenSSH, so
// one inside a Host or Match block still declares whatever Host lines the
// included file contains. Every other keyword is left to ssh.
func (p *parser) parseLine(source string, lineNumber int, line string, depth int) {
	fields, err := splitFields(line)
	if err != nil {
		p.warn(source, lineNumber, err.Error())
		return
	}
	if len(fields) == 0 {
		return
	}
	switch strings.ToLower(fields[0]) {
	case "host":
		p.addAliases(source, lineNumber, fields[1:])
	case "include":
		if len(fields) == 1 {
			p.warn(source, lineNumber, "Include requires a path")
			return
		}
		for _, pattern := range fields[1:] {
			p.parseInclude(source, lineNumber, pattern, depth+1)
		}
	}
}

// addAliases lists each literal pattern once. Wildcards and negations select
// hosts but do not name one, so they are not connections.
func (p *parser) addAliases(source string, lineNumber int, patterns []string) {
	if len(patterns) == 0 {
		p.warn(source, lineNumber, "Host requires a pattern")
		return
	}
	for _, pattern := range patterns {
		if strings.HasPrefix(pattern, "!") || strings.ContainsAny(pattern, "*?") {
			continue
		}
		key := strings.ToLower(pattern)
		if p.seen[key] {
			continue
		}
		p.seen[key] = true
		p.result.Candidates = append(p.result.Candidates, Candidate{Alias: pattern, Source: source, Line: lineNumber})
	}
}

func (p *parser) parseInclude(source string, lineNumber int, pattern string, depth int) {
	expanded, err := expandPath(pattern, p.options.HomeDir)
	if err != nil {
		p.warn(source, lineNumber, fmt.Sprintf("Include %q: %v", pattern, err))
		return
	}
	matches, err := filepath.Glob(expanded)
	if err != nil {
		p.warn(source, lineNumber, fmt.Sprintf("Include %q: %v", pattern, err))
		return
	}
	sort.Strings(matches)
	if len(matches) == 0 && !strings.ContainsAny(pattern, "*?[") {
		p.parseFile(expanded, depth)
		return
	}
	for _, match := range matches {
		if p.files >= p.options.MaxFiles {
			p.warn(source, lineNumber, fmt.Sprintf("file count exceeds limit of %d", p.options.MaxFiles))
			break
		}
		p.parseFile(match, depth)
	}
}

// OpenSSH resolves a relative Include in user configuration against ~/.ssh,
// never against the directory of the including file.
func expandPath(path, homeDir string) (string, error) {
	if path == "~" {
		if homeDir == "" {
			return "", errors.New("home directory is unavailable")
		}
		return homeDir, nil
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		if homeDir == "" {
			return "", errors.New("home directory is unavailable")
		}
		path = filepath.Join(homeDir, path[2:])
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	if homeDir == "" {
		return "", errors.New("home directory is unavailable")
	}
	return filepath.Join(homeDir, ".ssh", path), nil
}

func (p *parser) warn(source string, line int, message string) {
	p.result.Warnings = append(p.result.Warnings, Issue{Source: source, Line: line, Message: message})
}

// splitFields splits one ssh_config line into keyword and arguments, honouring
// quotes, backslash escapes, "#" comments and the keyword=value form.
func splitFields(line string) ([]string, error) {
	var fields []string
	var current strings.Builder
	quote := rune(0)
	escaped := false
	flush := func() {
		if current.Len() > 0 {
			fields = append(fields, current.String())
			current.Reset()
		}
	}
	for _, character := range line {
		if escaped {
			current.WriteRune(character)
			escaped = false
			continue
		}
		if character == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if character == quote {
				quote = 0
			} else {
				current.WriteRune(character)
			}
			continue
		}
		switch character {
		case '\'', '"':
			quote = character
		case '#':
			flush()
			return fields, nil
		case ' ', '\t':
			flush()
		case '=':
			// OpenSSH accepts keyword=value. Once the value starts, later
			// equals signs are part of that value.
			if len(fields) == 0 && current.Len() > 0 {
				flush()
			} else if len(fields) != 1 || current.Len() > 0 {
				current.WriteRune(character)
			}
		default:
			current.WriteRune(character)
		}
	}
	if escaped {
		current.WriteRune('\\')
	}
	if quote != 0 {
		return nil, errors.New("unterminated quoted value")
	}
	flush()
	return fields, nil
}

func maxInt() int {
	return int(^uint(0) >> 1)
}
