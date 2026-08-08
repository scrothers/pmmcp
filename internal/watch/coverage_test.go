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

package watch_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/watch"
)

// TestWithMaxWaitCapsBurst covers WithMaxWait: a path that keeps changing
// faster than the debounce window would otherwise postpone its event
// forever, but the maxWait cap forces an emit once the burst has run long
// enough.
func TestWithMaxWaitCapsBurst(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "busy.txt")
	if err := os.WriteFile(file, []byte("v0"), 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * time.Second)
	if err := os.Chtimes(file, past, past); err != nil {
		t.Fatal(err)
	}

	w := watch.New(
		watch.WithDebounce(500*time.Millisecond), // never quiets during the burst
		watch.WithPollInterval(10*time.Millisecond),
		watch.WithMaxWait(120*time.Millisecond),
	)
	if err := w.Add(file); err != nil {
		t.Fatalf("Add: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)
	t.Cleanup(func() { _ = w.Close() })

	stop := time.After(300 * time.Millisecond)
	n := 0
loop:
	for {
		select {
		case <-stop:
			break loop
		default:
			n++
			if err := os.WriteFile(file, bytesRepeat(n), 0o644); err != nil {
				t.Fatal(err)
			}
			time.Sleep(15 * time.Millisecond)
		}
	}

	select {
	case ev := <-w.Events():
		abs, _ := filepath.Abs(file)
		if ev.Path != abs {
			t.Fatalf("Path = %q, want %q", ev.Path, abs)
		}
	case <-time.After(time.Second):
		t.Fatal("maxWait cap did not force an event during a continuous burst")
	}
}

func bytesRepeat(n int) []byte {
	b := make([]byte, n%50+1)
	for i := range b {
		b[i] = 'x'
	}
	return b
}

// TestAddAbsPathError covers the filepath.Abs error branch in Add: Abs only
// fails (on Linux) when the process's current working directory itself has
// been removed and the path passed in is relative. This mutates process-wide
// cwd, so it must not run in parallel with other tests that Add relative
// paths; every other Add-based test in this package uses t.TempDir(), which
// is always absolute.
func TestAddAbsPathError(t *testing.T) {
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	w := watch.New()
	if err := w.Add("relative-path"); err == nil {
		t.Fatal("Add from a deleted cwd: want error")
	}
}

// TestAddDirPermissionDenied covers the snapshotDir error branch inside Add
// (os.Stat succeeds on the directory itself, but os.ReadDir cannot list it).
func TestAddDirPermissionDenied(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	w := watch.New()
	if err := w.Add(dir); err == nil {
		t.Fatal("Add on an unreadable directory: want error")
	}
}

// TestDirChangedReadPermissionRevoked covers dirChanged's error branch: a
// directory watched successfully at Add time whose read permission is
// revoked afterward must not panic or flag a change, just skip the tick.
func TestDirChangedReadPermissionRevoked(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "watched")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	w := watch.New(watch.WithPollInterval(10 * time.Millisecond))
	if err := w.Add(dir); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)
	t.Cleanup(func() { _ = w.Close() })

	// No event should fire; the tick loop must just skip the unreadable dir.
	select {
	case ev := <-w.Events():
		t.Fatalf("unexpected event for unreadable directory: %+v", ev)
	case <-time.After(150 * time.Millisecond):
	}
}

// TestSnapshotDirSkipsVanishedEntry covers the de.Info() TOCTOU error branch
// in snapshotDir via the direntInfo test seam: one entry's stat fails (as if
// removed between os.ReadDir and the per-entry stat), and the snapshot must
// still succeed, simply omitting that entry.
func TestSnapshotDirSkipsVanishedEntry(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	restore := watch.SetDirentInfoForTest(func(de os.DirEntry) (os.FileInfo, error) {
		if de.Name() == "a.txt" {
			return nil, os.ErrNotExist
		}
		return de.Info()
	})
	defer restore()

	w := watch.New()
	if err := w.Add(dir); err != nil {
		t.Fatalf("Add: %v, want the vanished entry skipped rather than failing", err)
	}
}

// TestMissingPathSkipped covers the os.Stat error branch in tick: a watched
// path removed after Add must be skipped on every subsequent tick without
// panicking or emitting spurious events.
func TestMissingPathSkipped(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "gone.txt")
	if err := os.WriteFile(file, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := watch.New(watch.WithPollInterval(10 * time.Millisecond))
	if err := w.Add(file); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)
	t.Cleanup(func() { _ = w.Close() })

	select {
	case ev := <-w.Events():
		t.Fatalf("unexpected event for a missing path: %+v", ev)
	case <-time.After(150 * time.Millisecond):
	}
}

// TestStartIsIdempotent covers the already-started guard in Start.
func TestStartIsIdempotent(t *testing.T) {
	t.Parallel()
	w := watch.New(watch.WithPollInterval(10 * time.Millisecond))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)
	w.Start(ctx) // no-op: must not spawn a second loop or panic
	t.Cleanup(func() { _ = w.Close() })
}

// TestFullBufferRetriesUntilDrained covers the full-buffer default branch in
// tick's emit loop: with more pending paths than the 64-slot events buffer,
// at least one path's send must default (buffer full) and retry on a later
// tick rather than being dropped.
func TestFullBufferRetriesUntilDrained(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	const n = 80
	files := make([]string, n)
	past := time.Now().Add(-2 * time.Second)
	for i := range n {
		f := filepath.Join(dir, "f"+string(rune('a'+i%26))+string(rune('0'+i/26)))
		if err := os.WriteFile(f, []byte("v0"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(f, past, past); err != nil {
			t.Fatal(err)
		}
		files[i] = f
	}

	w := watch.New(
		watch.WithDebounce(20*time.Millisecond),
		watch.WithPollInterval(10*time.Millisecond),
	)
	for _, f := range files {
		if err := w.Add(f); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)
	t.Cleanup(func() { _ = w.Close() })

	// Change every file at once so all n pending entries become eligible to
	// emit around the same tick, exceeding the 64-slot buffer.
	for _, f := range files {
		if err := os.WriteFile(f, []byte("v1-longer"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Deliberately don't drain yet: give every entry time to become ready
	// and queue up against the unread 64-slot buffer, so the excess over 64
	// is guaranteed to hit the full-buffer retry path at least once instead
	// of just keeping pace with a concurrent reader.
	time.Sleep(200 * time.Millisecond)

	seen := make(map[string]bool, n)
	deadline := time.After(3 * time.Second)
	for len(seen) < n {
		select {
		case ev := <-w.Events():
			seen[ev.Path] = true
		case <-deadline:
			t.Fatalf("only received %d/%d events before deadline (dropped on full buffer?)", len(seen), n)
		}
	}
}
