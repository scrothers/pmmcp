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

package ipc

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// WriteFrame writes a length-prefixed JSON message (4-byte big-endian length).
func WriteFrame(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("ipc: marshal: %w", err)
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(b)))
	if _, err := w.Write(hdr[:]); err != nil {
		return fmt.Errorf("ipc: write hdr: %w", err)
	}
	if _, err := w.Write(b); err != nil {
		return fmt.Errorf("ipc: write body: %w", err)
	}
	return nil
}

// ReadFrame reads one length-prefixed JSON message into dest.
func ReadFrame(r io.Reader, dest any) error {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return fmt.Errorf("ipc: read hdr: %w", err)
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > 32<<20 {
		return fmt.Errorf("ipc: frame too large: %d", n)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return fmt.Errorf("ipc: read body: %w", err)
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("ipc: unmarshal: %w", err)
	}
	return nil
}
