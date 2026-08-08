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

package logcap_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/logcap"
)

func TestFilterLevelJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	body := `{"level":"info","msg":"ok"}
{"level":"error","msg":"bad"}
{"level":"debug","msg":"x"}
`
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := logcap.FilterLevel(dir, logcap.StructuredOptions{Stream: "stdout", MinLevel: "error", Lines: 50})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "bad") {
		t.Fatalf("missing error line: %q", out)
	}
	if strings.Contains(out, "ok") || strings.Contains(out, `"debug"`) {
		t.Fatalf("should filter lower levels: %q", out)
	}
}
