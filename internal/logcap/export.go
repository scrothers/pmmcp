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

package logcap

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// ExportOptions configures ExportTarGzWithOptions.
type ExportOptions struct {
	// IncludeArchives adds rotated segments (stdout.log.*, stderr.log.*) in
	// addition to the active logs.
	IncludeArchives bool
	// Meta is optional secret-free process metadata merged into manifest.json.
	Meta map[string]string
}

// manifestEntry describes one archived file.
type manifestEntry struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
}

// manifest is the export manifest written as manifest.json.
type manifest struct {
	GeneratedAt string            `json:"generated_at"`
	Files       []manifestEntry   `json:"files"`
	Meta        map[string]string `json:"meta,omitempty"`
}

// ExportTarGz writes a gzip tar of the active logs and their rotated archives
// under dir to w, plus a manifest.json describing the bundle. It checks the tar
// and gzip trailers on close, so a truncated archive is reported as an error
// rather than as silent success.
func ExportTarGz(dir string, w io.Writer) error {
	return ExportTarGzWithOptions(dir, w, ExportOptions{IncludeArchives: true})
}

// ExportTarGzWithOptions is ExportTarGz with explicit options.
func ExportTarGzWithOptions(dir string, w io.Writer, opts ExportOptions) (err error) {
	gw := gzip.NewWriter(w)
	tw := tar.NewWriter(gw)
	// Close tar then gzip explicitly, surfacing trailer-flush errors that the
	// defer form would otherwise drop.
	defer func() {
		if cerr := tw.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("logcap: export tar close: %w", cerr)
		}
		if cerr := gw.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("logcap: export gzip close: %w", cerr)
		}
	}()

	names, ferr := exportFileNames(dir, opts.IncludeArchives)
	if ferr != nil {
		return ferr
	}

	man := manifest{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Meta: opts.Meta}
	for _, name := range names {
		fi, serr := os.Stat(filepath.Join(dir, name))
		if serr != nil {
			return fmt.Errorf("logcap: export stat: %w", serr)
		}
		man.Files = append(man.Files, manifestEntry{
			Name:    name,
			Size:    fi.Size(),
			ModTime: fi.ModTime().UTC().Format(time.RFC3339),
		})
	}

	manData, merr := json.MarshalIndent(man, "", "  ")
	if merr != nil {
		return fmt.Errorf("logcap: export manifest: %w", merr)
	}
	if werr := writeTarBytes(tw, "manifest.json", manData); werr != nil {
		return werr
	}
	for _, name := range names {
		if aerr := addFileToTar(tw, dir, name); aerr != nil {
			return aerr
		}
	}
	return nil
}

// exportFileNames lists the log files to bundle, in stable order.
func exportFileNames(dir string, includeArchives bool) ([]string, error) {
	if !includeArchives {
		return existing(dir, []string{stdoutName, stderrName}), nil
	}
	var names []string
	for _, base := range []string{stdoutName, stderrName} {
		matches, err := filepath.Glob(filepath.Join(dir, base+"*"))
		if err != nil {
			return nil, fmt.Errorf("logcap: export glob: %w", err)
		}
		for _, m := range matches {
			names = append(names, filepath.Base(m))
		}
	}
	sort.Strings(names)
	return names, nil
}

// existing filters bases down to those that exist under dir.
func existing(dir string, bases []string) []string {
	out := make([]string, 0, len(bases))
	for _, b := range bases {
		if _, err := os.Stat(filepath.Join(dir, b)); err == nil {
			out = append(out, b)
		}
	}
	return out
}

func writeTarBytes(tw *tar.Writer, name string, data []byte) error {
	hdr := &tar.Header{Name: name, Mode: 0o600, Size: int64(len(data)), ModTime: time.Now().UTC()}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("logcap: export header %s: %w", name, err)
	}
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("logcap: export write %s: %w", name, err)
	}
	return nil
}

func addFileToTar(tw *tar.Writer, dir, name string) error {
	path := filepath.Join(dir, name)
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("logcap: export stat: %w", err)
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("logcap: export open: %w", err)
	}
	defer func() { _ = f.Close() }()
	hdr, err := tar.FileInfoHeader(fi, "")
	if err != nil {
		return fmt.Errorf("logcap: export header %s: %w", name, err)
	}
	hdr.Name = name
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("logcap: export header %s: %w", name, err)
	}
	if _, err := io.Copy(tw, f); err != nil {
		return fmt.Errorf("logcap: export copy %s: %w", name, err)
	}
	return nil
}
