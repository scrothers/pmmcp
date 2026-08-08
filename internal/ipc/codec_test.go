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

package ipc_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/api"
	"github.com/scrothers/pmmcp/internal/ipc"
)

func TestFrameRoundTrip(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	req := api.Request{APIVersion: "1.0", Method: "hello"}
	if err := ipc.WriteFrame(&buf, req); err != nil {
		t.Fatal(err)
	}
	var got api.Request
	if err := ipc.ReadFrame(&buf, &got); err != nil {
		t.Fatal(err)
	}
	if got.Method != "hello" || got.APIVersion != "1.0" {
		t.Fatalf("%+v", got)
	}
}

// errAfterN is an io.Writer that errors on the Nth call to Write (1-indexed);
// earlier calls succeed by discarding the bytes.
type errAfterN struct {
	n     int
	calls int
}

func (w *errAfterN) Write(p []byte) (int, error) {
	w.calls++
	if w.calls >= w.n {
		return 0, errors.New("boom")
	}
	return len(p), nil
}

func TestWriteFrameMarshalError(t *testing.T) {
	t.Parallel()
	// A channel value cannot be marshaled to JSON.
	if err := ipc.WriteFrame(&bytes.Buffer{}, struct{ C chan int }{C: make(chan int)}); err == nil {
		t.Fatal("WriteFrame with unmarshalable value: want error, got nil")
	}
}

func TestWriteFrameHeaderWriteError(t *testing.T) {
	t.Parallel()
	w := &errAfterN{n: 1}
	if err := ipc.WriteFrame(w, api.Request{Method: "x"}); err == nil {
		t.Fatal("WriteFrame with failing header write: want error, got nil")
	}
}

func TestWriteFrameBodyWriteError(t *testing.T) {
	t.Parallel()
	w := &errAfterN{n: 2}
	if err := ipc.WriteFrame(w, api.Request{Method: "x"}); err == nil {
		t.Fatal("WriteFrame with failing body write: want error, got nil")
	}
}

func TestReadFrameHeaderReadError(t *testing.T) {
	t.Parallel()
	// Fewer than 4 bytes: io.ReadFull on the header fails.
	r := bytes.NewReader([]byte{0x01, 0x02})
	var dest api.Request
	if err := ipc.ReadFrame(r, &dest); err == nil {
		t.Fatal("ReadFrame with truncated header: want error, got nil")
	}
}

func TestReadFrameTooLarge(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], 33<<20) // over the 32MiB cap
	buf.Write(hdr[:])
	var dest api.Request
	if err := ipc.ReadFrame(&buf, &dest); err == nil {
		t.Fatal("ReadFrame with oversized frame: want error, got nil")
	}
}

func TestReadFrameBodyReadError(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], 10) // claims 10 bytes, provides none
	buf.Write(hdr[:])
	var dest api.Request
	if err := ipc.ReadFrame(&buf, &dest); err == nil {
		t.Fatal("ReadFrame with truncated body: want error, got nil")
	}
}

func TestReadFrameUnmarshalError(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	body := []byte("not json")
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(body)))
	buf.Write(hdr[:])
	buf.Write(body)
	var dest api.Request
	err := ipc.ReadFrame(&buf, &dest)
	if err == nil {
		t.Fatal("ReadFrame with invalid JSON body: want error, got nil")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Fatalf("err = %v, want mention of unmarshal", err)
	}
}
