// Copyright 2026 Steven Crothers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logcap

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	// DefaultMaxOutputBytes caps returned payload size (~256 KiB).
	DefaultMaxOutputBytes = 256 * 1024

	defaultTailLines  = 200
	defaultErrorLines = 100
	defaultMaxMatches = 100
	truncateMarker    = "\n... [truncated]\n"
)

// errorHeuristic matches common error markers case-insensitively.
var errorHeuristic = regexp.MustCompile(`(?i)\b(error|panic|fatal|exception|traceback)\b`)

// TailOptions configures Tail.
type TailOptions struct {
	// Stream is "stdout", "stderr", or "both" (default "both").
	Stream string
	// Lines is the number of trailing lines (default 200 when ≤ 0).
	Lines int
}

// GrepOptions configures Grep.
type GrepOptions struct {
	// Stream is "stdout", "stderr", or "both" (default "both").
	Stream string
	// Pattern is a RE2 regular expression.
	Pattern string
	// MaxMatches caps the number of matching lines (default 100 when ≤ 0).
	MaxMatches int
	// Context is the number of surrounding lines to include for each match.
	Context int
}

// ErrorsOptions configures Errors.
type ErrorsOptions struct {
	// Stream is "stdout", "stderr", or "both" (default "both").
	Stream string
	// Lines is the max number of error lines returned (default 100 when ≤ 0).
	// When positive, the last Lines matches are kept (tail semantics).
	Lines int
}

// Tail returns the last Lines of the selected stream(s) under dir.
// When Stream is "both", each line is prefixed with "stdout| " or "stderr| ".
// Output is capped at DefaultMaxOutputBytes.
func Tail(dir string, opts TailOptions) (string, error) {
	stream, err := normalizeStream(opts.Stream)
	if err != nil {
		return "", err
	}
	lines := opts.Lines
	if lines <= 0 {
		lines = defaultTailLines
	}

	var parts []string
	for _, name := range streamFiles(stream) {
		fileLines, err := readAllLines(filepath.Join(dir, name))
		if err != nil {
			return "", err
		}
		tail := lastN(fileLines, lines)
		label := streamLabel(name)
		for _, ln := range tail {
			if stream == "both" {
				parts = append(parts, label+"| "+ln)
			} else {
				parts = append(parts, ln)
			}
		}
	}
	return capOutput(strings.Join(parts, "\n")), nil
}

// Grep returns lines matching Pattern under dir, with optional context lines.
// Matching lines are formatted as "N:text" (1-based line numbers within the file).
// When Stream is "both", lines are prefixed with "stdout| " or "stderr| ".
// Output is capped at DefaultMaxOutputBytes.
func Grep(dir string, opts GrepOptions) (string, error) {
	stream, err := normalizeStream(opts.Stream)
	if err != nil {
		return "", err
	}
	if opts.Pattern == "" {
		return "", fmt.Errorf("logcap: empty grep pattern")
	}
	re, err := regexp.Compile(opts.Pattern)
	if err != nil {
		return "", fmt.Errorf("logcap: pattern: %w", err)
	}
	maxMatches := opts.MaxMatches
	if maxMatches <= 0 {
		maxMatches = defaultMaxMatches
	}
	if opts.Context < 0 {
		opts.Context = 0
	}

	var out []string
	matches := 0
	for _, name := range streamFiles(stream) {
		fileLines, err := readAllLines(filepath.Join(dir, name))
		if err != nil {
			return "", err
		}
		// Collect match indices first so context windows can be merged.
		var matchIdx []int
		for i, ln := range fileLines {
			if re.MatchString(ln) {
				matchIdx = append(matchIdx, i)
				if len(matchIdx) >= maxMatches-matches {
					break
				}
			}
		}
		if len(matchIdx) == 0 {
			continue
		}
		// Emit merged windows.
		emitted := make([]bool, len(fileLines))
		label := streamLabel(name)
		firstWindow := true
		for _, mi := range matchIdx {
			if matches >= maxMatches {
				break
			}
			start := mi - opts.Context
			if start < 0 {
				start = 0
			}
			end := mi + opts.Context
			if end >= len(fileLines) {
				end = len(fileLines) - 1
			}
			// Separator between non-overlapping context groups when context > 0.
			if opts.Context > 0 && !firstWindow {
				// If this window doesn't abut previous emission, add a marker.
				gap := true
				for j := start; j <= end; j++ {
					if emitted[j] {
						gap = false
						break
					}
				}
				if gap && len(out) > 0 {
					out = append(out, "--")
				}
			}
			firstWindow = false
			for j := start; j <= end; j++ {
				if emitted[j] {
					continue
				}
				emitted[j] = true
				line := formatNumbered(j+1, fileLines[j])
				if stream == "both" {
					line = label + "| " + line
				}
				out = append(out, line)
			}
			matches++
		}
		if matches >= maxMatches {
			break
		}
	}
	return capOutput(strings.Join(out, "\n")), nil
}

// Errors returns lines that match the error heuristic under dir.
// The heuristic is case-insensitive word matches for error, panic, fatal,
// exception, and traceback. Returns the last Lines matches (tail of errors).
// Output is capped at DefaultMaxOutputBytes.
func Errors(dir string, opts ErrorsOptions) (string, error) {
	stream, err := normalizeStream(opts.Stream)
	if err != nil {
		return "", err
	}
	limit := opts.Lines
	if limit <= 0 {
		limit = defaultErrorLines
	}

	var matched []string
	for _, name := range streamFiles(stream) {
		fileLines, err := readAllLines(filepath.Join(dir, name))
		if err != nil {
			return "", err
		}
		label := streamLabel(name)
		for i, ln := range fileLines {
			if !errorHeuristic.MatchString(ln) {
				continue
			}
			line := formatNumbered(i+1, ln)
			if stream == "both" {
				line = label + "| " + line
			}
			matched = append(matched, line)
		}
	}
	// Keep the last `limit` error lines.
	if len(matched) > limit {
		matched = matched[len(matched)-limit:]
	}
	return capOutput(strings.Join(matched, "\n")), nil
}

func normalizeStream(s string) (string, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return "both", nil
	}
	switch s {
	case "stdout", "stderr", "both":
		return s, nil
	default:
		return "", fmt.Errorf("logcap: invalid stream %q", s)
	}
}

func streamFiles(stream string) []string {
	switch stream {
	case "stdout":
		return []string{stdoutName}
	case "stderr":
		return []string{stderrName}
	default:
		return []string{stdoutName, stderrName}
	}
}

func streamLabel(name string) string {
	switch name {
	case stdoutName:
		return "stdout"
	case stderrName:
		return "stderr"
	default:
		return name
	}
}

// maxLineBytes caps a single retained line. Longer lines are truncated (with a
// marker) rather than failing the whole read, so one pathological line can't
// take down tail/grep/errors.
const maxLineBytes = 1024 * 1024

const lineTruncatedMarker = " ...[line truncated]"

func readAllLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("logcap: open %s: %w", filepath.Base(path), err)
	}
	defer func() { _ = f.Close() }()

	br := bufio.NewReaderSize(f, 64*1024)
	var lines []string
	for {
		line, atEOF, rerr := readLimitedLine(br)
		if rerr != nil {
			return nil, fmt.Errorf("logcap: read %s: %w", filepath.Base(path), rerr)
		}
		if line != nil {
			lines = append(lines, string(line))
		}
		if atEOF {
			return lines, nil
		}
	}
}

// readLimitedLine reads one newline-terminated line, retaining at most
// maxLineBytes and discarding the remainder of an overlong line. It returns the
// line (nil only at a clean EOF with no trailing bytes), whether EOF was
// reached, and any non-EOF read error. The trailing '\n' and '\r' are stripped.
func readLimitedLine(br *bufio.Reader) (line []byte, atEOF bool, err error) {
	var buf []byte
	truncated := false
	for {
		chunk, rerr := br.ReadSlice('\n')
		if len(buf) < maxLineBytes {
			if room := maxLineBytes - len(buf); len(chunk) > room {
				buf = append(buf, chunk[:room]...)
				truncated = true
			} else {
				buf = append(buf, chunk...)
			}
		} else if len(chunk) > 0 {
			truncated = true
		}
		if errors.Is(rerr, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(rerr, io.EOF) {
			if len(buf) == 0 {
				return nil, true, nil
			}
			return finishLine(buf, truncated), true, nil
		}
		if rerr != nil {
			return nil, false, rerr
		}
		return finishLine(buf, truncated), false, nil
	}
}

func finishLine(buf []byte, truncated bool) []byte {
	if n := len(buf); n > 0 && buf[n-1] == '\n' {
		buf = buf[:n-1]
	}
	if n := len(buf); n > 0 && buf[n-1] == '\r' {
		buf = buf[:n-1]
	}
	if truncated {
		buf = append(buf, lineTruncatedMarker...)
	}
	return buf
}

func lastN(lines []string, n int) []string {
	if n <= 0 || len(lines) == 0 {
		return nil
	}
	if n >= len(lines) {
		return lines
	}
	return lines[len(lines)-n:]
}

func formatNumbered(n int, text string) string {
	return fmt.Sprintf("%d:%s", n, text)
}

func capOutput(s string) string {
	if len(s) <= DefaultMaxOutputBytes {
		return s
	}
	// Truncate on a byte boundary; prefer cutting at a line boundary when possible.
	cut := DefaultMaxOutputBytes - len(truncateMarker)
	if cut < 0 {
		cut = 0
	}
	prefix := s[:cut]
	if i := strings.LastIndexByte(prefix, '\n'); i > 0 {
		prefix = prefix[:i]
	}
	return prefix + truncateMarker
}
