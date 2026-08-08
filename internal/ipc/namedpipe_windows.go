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

//go:build windows

package ipc

import (
	"fmt"
	"net"
	"strings"

	"github.com/Microsoft/go-winio"
)

// listenNamedPipe creates a Windows named pipe with restrictive ACL for the current user.
func listenNamedPipe(endpoint string) (net.Listener, error) {
	// Normalize to \\.\pipe\name form.
	path := endpoint
	path = strings.ReplaceAll(path, `/`, `\`)
	if !strings.HasPrefix(strings.ToLower(path), `\\.\pipe\`) {
		return nil, fmt.Errorf("ipc: invalid named pipe path %q", endpoint)
	}
	// SDDL: only current user (owner) has full access; no Everyone/Users.
	// winio.ListenPipe with default config restricts to the creating user.
	ln, err := winio.ListenPipe(path, &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;OW)",
		MessageMode:        true,
		InputBufferSize:    65536,
		OutputBufferSize:   65536,
	})
	if err != nil {
		return nil, fmt.Errorf("ipc: named pipe listen: %w", err)
	}
	return ln, nil
}
