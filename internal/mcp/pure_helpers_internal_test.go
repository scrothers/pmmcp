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

package mcp

import (
	"testing"

	"github.com/scrothers/pmmcp/internal/api"
)

func TestIdOrName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		seg  string
		want api.IDPayload
	}{
		{name: "proc- prefix resolves by id", seg: "proc-1", want: api.IDPayload{ID: "proc-1"}},
		{name: "no prefix resolves by name", seg: "api", want: api.IDPayload{Name: "api"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := idOrName(tt.seg); got != tt.want {
				t.Errorf("idOrName(%q) = %+v, want %+v", tt.seg, got, tt.want)
			}
		})
	}
}

func TestLogsPayload(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		seg   string
		lines int
		want  api.LogsPayload
	}{
		{name: "proc- prefix resolves by id", seg: "proc-1", lines: 200, want: api.LogsPayload{ID: "proc-1", Lines: 200}},
		{name: "no prefix resolves by name", seg: "api", lines: 200, want: api.LogsPayload{Name: "api", Lines: 200}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := logsPayload(tt.seg, tt.lines); got != tt.want {
				t.Errorf("logsPayload(%q, %d) = %+v, want %+v", tt.seg, tt.lines, got, tt.want)
			}
		})
	}
}
