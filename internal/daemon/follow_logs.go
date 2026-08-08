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
	"context"
	"os"
	"path/filepath"
	"time"
)

// followLogDir waits up to d for new bytes on stdout/stderr logs and returns a text preview.
func followLogDir(ctx context.Context, dir string, d time.Duration) string {
	stdout := filepath.Join(dir, "stdout.log")
	stderr := filepath.Join(dir, "stderr.log")
	startOut := fileSize(stdout)
	startErr := fileSize(stderr)
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return readSince(stdout, startOut) + readSince(stderr, startErr)
		default:
		}
		if fileSize(stdout) > startOut || fileSize(stderr) > startErr {
			return readSince(stdout, startOut) + readSince(stderr, startErr)
		}
		time.Sleep(50 * time.Millisecond)
	}
	return readSince(stdout, startOut) + readSince(stderr, startErr)
}

func fileSize(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}

func readSince(path string, off int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	if off > 0 {
		_, _ = f.Seek(off, 0)
	}
	buf := make([]byte, 64*1024)
	n, _ := f.Read(buf)
	if n <= 0 {
		return ""
	}
	return string(buf[:n])
}
