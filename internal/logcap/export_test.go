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
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/scrothers/pmmcp/internal/logcap"
)

func TestExportTarGz(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stdout.log"), []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := logcap.ExportTarGz(dir, &buf); err != nil {
		t.Fatal(err)
	}
	if buf.Len() < 20 {
		t.Fatalf("archive too small: %d", buf.Len())
	}
}
