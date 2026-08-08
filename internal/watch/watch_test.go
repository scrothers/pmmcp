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
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scrothers/pmmcp/internal/watch"
)

func TestDebouncedWrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "app.go")
	if err := os.WriteFile(file, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * time.Second)
	if err := os.Chtimes(file, past, past); err != nil {
		t.Fatal(err)
	}

	w := watch.New(
		watch.WithDebounce(80*time.Millisecond),
		watch.WithPollInterval(20*time.Millisecond),
	)
	if err := w.Add(file); err != nil {
		t.Fatalf("Add: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)
	t.Cleanup(func() { _ = w.Close() })

	if err := os.WriteFile(file, []byte("v2-longer"), 0o644); err != nil {
		t.Fatal(err)
	}

	abs, _ := filepath.Abs(file)
	select {
	case ev := <-w.Events():
		if ev.Path != abs {
			t.Fatalf("Path = %q, want %q", ev.Path, abs)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for watch event")
	}
}

// TestDebounceCoalesces asserts the core property: a burst of writes spanning
// longer than the debounce window collapses to exactly one event (resetting
// timer). Under the old anchor-window behavior this burst emitted more than one.
func TestDebounceCoalesces(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "app.go")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * time.Second)
	if err := os.Chtimes(file, past, past); err != nil {
		t.Fatal(err)
	}

	w := watch.New(
		watch.WithDebounce(120*time.Millisecond),
		watch.WithPollInterval(15*time.Millisecond),
	)
	if err := w.Add(file); err != nil {
		t.Fatalf("Add: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)
	t.Cleanup(func() { _ = w.Close() })

	// Five writes over ~160ms (each grows the file so the change is detected),
	// which exceeds the 120ms debounce window.
	for i := range 5 {
		if err := os.WriteFile(file, bytes.Repeat([]byte("x"), i+1), 0o644); err != nil {
			t.Fatal(err)
		}
		if i < 4 {
			time.Sleep(40 * time.Millisecond)
		}
	}

	// Collect every event over a window long enough for coalesced emit plus any
	// spurious second emit the old behavior would produce.
	count := 0
	deadline := time.After(700 * time.Millisecond)
loop:
	for {
		select {
		case <-w.Events():
			count++
		case <-deadline:
			break loop
		}
	}
	if count != 1 {
		t.Fatalf("coalesced events = %d, want exactly 1", count)
	}
}

// TestDirInPlaceEdit verifies that editing an existing file inside a watched
// directory (which does not change the directory's own mtime) still fires.
func TestDirInPlaceEdit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * time.Second)
	_ = os.Chtimes(file, past, past)
	_ = os.Chtimes(dir, past, past)

	w := watch.New(
		watch.WithDebounce(60*time.Millisecond),
		watch.WithPollInterval(15*time.Millisecond),
	)
	if err := w.Add(dir); err != nil {
		t.Fatalf("Add dir: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)
	t.Cleanup(func() { _ = w.Close() })

	if err := os.WriteFile(file, []byte("v2-much-longer"), 0o644); err != nil {
		t.Fatal(err)
	}

	absDir, _ := filepath.Abs(dir)
	select {
	case ev := <-w.Events():
		if ev.Path != absDir {
			t.Fatalf("Path = %q, want dir %q", ev.Path, absDir)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event for in-place edit inside watched directory")
	}
}

func TestAddMissing(t *testing.T) {
	t.Parallel()
	w := watch.New()
	if err := w.Add(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("Add missing path: want error")
	}
}

func TestCloseIdempotentAndBeforeStart(t *testing.T) {
	t.Parallel()
	w := watch.New()
	if err := w.Close(); err != nil {
		t.Fatalf("close before start: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestAddAfterClose(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := watch.New()
	_ = w.Close()
	if err := w.Add(file); err == nil {
		t.Fatal("Add after Close: want error")
	}
}

func TestContextCancelClosesEvents(t *testing.T) {
	t.Parallel()
	w := watch.New(watch.WithPollInterval(10 * time.Millisecond))
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	cancel()

	deadline := time.After(time.Second)
	for {
		select {
		case _, ok := <-w.Events():
			if !ok {
				return // channel closed as expected
			}
		case <-deadline:
			t.Fatal("events channel not closed after context cancel")
		}
	}
}
