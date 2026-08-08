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

package watch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Event is a debounced filesystem change notification.
type Event struct {
	// Path is the watched path that changed (file or directory).
	Path string
	// At is when the debounced event was emitted.
	At time.Time
}

// Watcher polls path mtimes and emits debounced change events.
//
// Polling (every PollInterval, default 200ms) keeps the implementation simple
// and hermetic in tests. A future fsnotify backend can share the same API.
type Watcher struct {
	debounce time.Duration
	poll     time.Duration
	maxWait  time.Duration

	mu      sync.Mutex
	paths   map[string]fileSnap            // path -> last known mtime+size
	dirs    map[string]map[string]fileSnap // dir path -> entry name -> snap
	pending map[string]pendingChange       // path -> debounce window bookkeeping

	events chan Event
	cancel context.CancelFunc
	done   chan struct{}
	closed bool
}

type fileSnap struct {
	mod   time.Time
	size  int64
	isDir bool
}

// pendingChange tracks the debounce window for a path: first is the start of the
// burst, last is the most recent change (reset on every change).
type pendingChange struct {
	first time.Time
	last  time.Time
}

// Option configures a Watcher.
type Option func(*Watcher)

// WithDebounce sets the debounce window (default 300ms).
func WithDebounce(d time.Duration) Option {
	return func(w *Watcher) {
		if d > 0 {
			w.debounce = d
		}
	}
}

// WithPollInterval sets how often paths are polled (default 200ms).
func WithPollInterval(d time.Duration) Option {
	return func(w *Watcher) {
		if d > 0 {
			w.poll = d
		}
	}
}

// WithMaxWait caps how long a continuously-changing path may postpone its event.
// Zero (default) means unbounded: the event fires only once the burst quiets for
// the debounce interval.
func WithMaxWait(d time.Duration) Option {
	return func(w *Watcher) {
		if d > 0 {
			w.maxWait = d
		}
	}
}

// New creates a Watcher. Call Start to begin polling; Close to stop.
func New(opts ...Option) *Watcher {
	w := &Watcher{
		debounce: 300 * time.Millisecond,
		poll:     200 * time.Millisecond,
		paths:    make(map[string]fileSnap),
		dirs:     make(map[string]map[string]fileSnap),
		pending:  make(map[string]pendingChange),
		events:   make(chan Event, 64),
		done:     make(chan struct{}),
	}
	for _, o := range opts {
		o(w)
	}
	return w
}

// Events returns the channel of debounced change events.
func (w *Watcher) Events() <-chan Event {
	return w.events
}

// Add registers a path (file or directory) to watch.
// Directories are watched non-recursively: the directory's own mtime catches
// entry create/delete/rename, and a snapshot of the immediate entries' mtime and
// size catches in-place edits to files directly inside it.
func (w *Watcher) Add(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("watch: add: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("watch: add: %w", err)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return fmt.Errorf("watch: closed")
	}
	w.paths[abs] = fileSnap{mod: info.ModTime(), size: info.Size(), isDir: info.IsDir()}
	if info.IsDir() {
		entries, err := snapshotDir(abs)
		if err != nil {
			return fmt.Errorf("watch: add: %w", err)
		}
		w.dirs[abs] = entries
	}
	return nil
}

// direntInfo fetches a directory entry's FileInfo. It is a variable (rather
// than a direct de.Info() call) so tests can simulate the TOCTOU race where
// an entry is removed between os.ReadDir and the per-entry stat, without
// depending on real filesystem timing.
var direntInfo = func(de os.DirEntry) (os.FileInfo, error) { return de.Info() }

// snapshotDir records mtime+size for the immediate entries of dir.
func snapshotDir(dir string) (map[string]fileSnap, error) {
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	m := make(map[string]fileSnap, len(des))
	for _, de := range des {
		fi, err := direntInfo(de)
		if err != nil {
			// Entry vanished between ReadDir and stat (TOCTOU): skip it
			// rather than failing the whole snapshot.
			continue
		}
		m[de.Name()] = fileSnap{mod: fi.ModTime(), size: fi.Size(), isDir: de.IsDir()}
	}
	return m, nil
}

// Start begins the poll loop until ctx is canceled or Close is called.
// It is safe to call once; subsequent calls are no-ops if already running.
func (w *Watcher) Start(ctx context.Context) {
	w.mu.Lock()
	if w.cancel != nil || w.closed {
		w.mu.Unlock()
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	w.mu.Unlock()

	go w.loop(runCtx)
}

// Close stops the watcher and closes the Events channel.
func (w *Watcher) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	cancel := w.cancel
	started := cancel != nil
	w.mu.Unlock()
	if !started {
		// Start was never called; close channels ourselves.
		close(w.done)
		close(w.events)
		return nil
	}
	cancel()
	<-w.done
	return nil
}

func (w *Watcher) loop(ctx context.Context) {
	defer close(w.done)
	defer close(w.events)

	ticker := time.NewTicker(w.poll)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			w.tick(now)
		}
	}
}

func (w *Watcher) tick(now time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}

	for path, last := range w.paths {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		mt := info.ModTime()
		sz := info.Size()
		// mtime changed in either direction (catches a restore to an older
		// mtime) or size changed (catches same-second content writes).
		changed := !mt.Equal(last.mod) || sz != last.size
		if last.isDir && w.dirChanged(path) {
			changed = true
		}
		if changed {
			w.paths[path] = fileSnap{mod: mt, size: sz, isDir: last.isDir}
			pc := w.pending[path]
			if pc.first.IsZero() {
				pc.first = now
			}
			// Reset the quiet-period timer on every change so a burst coalesces
			// into a single event once it settles.
			pc.last = now
			w.pending[path] = pc
		}
	}

	// Emit paths whose debounce window has elapsed (quiet for debounce, or the
	// maxWait cap reached). On a full buffer keep the entry and retry next tick
	// so the event is never silently lost.
	for path, pc := range w.pending {
		quiet := now.Sub(pc.last) >= w.debounce
		capped := w.maxWait > 0 && now.Sub(pc.first) >= w.maxWait
		if !quiet && !capped {
			continue
		}
		select {
		case w.events <- Event{Path: path, At: now}:
			delete(w.pending, path)
		default:
			// Buffer full: keep pending; retry next tick.
		}
	}
}

// dirChanged compares the current immediate entries of dir against the last
// snapshot and updates the snapshot. It reports whether any entry's mtime or
// size changed, or an entry was added or removed.
func (w *Watcher) dirChanged(dir string) bool {
	cur, err := snapshotDir(dir)
	if err != nil {
		return false
	}
	prev := w.dirs[dir]
	changed := len(cur) != len(prev)
	if !changed {
		for name, cs := range cur {
			ps, ok := prev[name]
			if !ok || !cs.mod.Equal(ps.mod) || cs.size != ps.size {
				changed = true
				break
			}
		}
	}
	if changed {
		w.dirs[dir] = cur
	}
	return changed
}
