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

package secret

import "testing"

// White-box test for extractKey (reachable in production only via a decrypted
// SOPS body, so tested directly here across the JSON, dotenv, and YAML fallbacks).
func TestExtractKey(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		data    string
		key     string
		want    string
		wantErr bool
	}{
		{"json string", `{"db":"pw1","other":"x"}`, "db", "pw1", false},
		{"json missing", `{"db":"pw1"}`, "nope", "", true},
		{"json non-string", `{"n":42}`, "n", "42", false},
		{"dotenv", "DB=pw2\nOTHER=y\n", "DB", "pw2", false},
		{"dotenv missing", "DB=pw2\n", "nope", "", true},
		{"yaml top-level", "db: pw3\nother: y\n", "db", "pw3", false},
		{"empty key returns whole", `{"db":"pw"}`, "", `{"db":"pw"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := extractKey([]byte(tc.data), tc.key)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("extractKey = %q, want %q", got, tc.want)
			}
		})
	}
}
