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
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// LevelOrder ranks log levels for min_level filtering (exact-key lookups only;
// classification uses the severity-ordered levelRanks slice for determinism).
var LevelOrder = map[string]int{
	"trace":   10,
	"debug":   20,
	"info":    30,
	"warn":    40,
	"warning": 40,
	"error":   50,
	"fatal":   60,
	"panic":   70,
}

// levelRanks lists levels highest-severity first so plain-text classification is
// deterministic: a line carrying more than one marker is ranked by the most
// severe one, independent of map-iteration order.
var levelRanks = []struct {
	name string
	rank int
}{
	{"panic", 70},
	{"fatal", 60},
	{"error", 50},
	{"warning", 40},
	{"warn", 40},
	{"info", 30},
	{"debug", 20},
	{"trace", 10},
}

// StructuredOptions filters JSON/NDJSON log lines by level.
type StructuredOptions struct {
	Stream   string
	MinLevel string // e.g. "error"
	Lines    int
}

// FilterLevel returns lines at or above MinLevel.
// Supports JSON objects with "level" field, or text prefixes like "ERROR:" / "level=error".
func FilterLevel(dir string, opts StructuredOptions) (string, error) {
	stream, err := normalizeStream(opts.Stream)
	if err != nil {
		return "", err
	}
	minLevel := strings.ToLower(strings.TrimSpace(opts.MinLevel))
	if minLevel == "" {
		minLevel = "info"
	}
	minRank, ok := LevelOrder[minLevel]
	if !ok {
		return "", fmt.Errorf("logcap: unknown min_level %q", opts.MinLevel)
	}
	lines := opts.Lines
	if lines <= 0 {
		lines = defaultTailLines
	}
	var matched []string
	for _, name := range streamFiles(stream) {
		all, err := readAllLines(filepath.Join(dir, name))
		if err != nil {
			return "", err
		}
		label := streamLabel(name)
		for _, ln := range all {
			if lineLevelRank(ln) >= minRank {
				if stream == "both" {
					matched = append(matched, label+"| "+ln)
				} else {
					matched = append(matched, ln)
				}
			}
		}
	}
	matched = lastN(matched, lines)
	return capOutput(strings.Join(matched, "\n")), nil
}

func lineLevelRank(line string) int {
	trim := strings.TrimSpace(line)
	if strings.HasPrefix(trim, "{") {
		var m map[string]any
		if json.Unmarshal([]byte(trim), &m) == nil {
			if lv, ok := m["level"].(string); ok {
				if r, ok := LevelOrder[strings.ToLower(lv)]; ok {
					return r
				}
			}
			if lv, ok := m["lvl"].(string); ok {
				if r, ok := LevelOrder[strings.ToLower(lv)]; ok {
					return r
				}
			}
		}
	}
	low := strings.ToLower(trim)
	for _, lv := range levelRanks {
		if strings.Contains(low, "level="+lv.name) || strings.HasPrefix(low, lv.name+":") || strings.Contains(low, "\"level\":\""+lv.name+"\"") {
			return lv.rank
		}
	}
	// default treat as info
	return LevelOrder["info"]
}
