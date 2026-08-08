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

package daemon

import (
	"errors"
	"testing"
)

func TestMustJSONMarshalError(t *testing.T) {
	t.Parallel()
	// A channel value cannot be marshaled to JSON.
	got := mustJSON(struct{ C chan int }{C: make(chan int)})
	if string(got) != "{}" {
		t.Fatalf("mustJSON on an unmarshalable value = %q, want {}", got)
	}
}

func TestErrStringNil(t *testing.T) {
	t.Parallel()
	if got := errString(nil); got != "" {
		t.Fatalf("errString(nil) = %q, want empty", got)
	}
}

func TestErrStringNonNil(t *testing.T) {
	t.Parallel()
	if got := errString(errors.New("boom")); got != "boom" {
		t.Fatalf("errString = %q, want boom", got)
	}
}
