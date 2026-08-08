// Copyright 2026 Steven Crothers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package domain_test

import (
	"errors"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/scrothers/pmmcp/internal/domain"
)

func TestStatusValid(t *testing.T) {
	t.Parallel()
	for _, s := range domain.AllStatuses {
		if !s.Valid() {
			t.Fatalf("%q should be valid", s)
		}
		parsed, err := domain.ParseStatus(string(s))
		if err != nil || parsed != s {
			t.Fatalf("ParseStatus(%q) = %q, %v", s, parsed, err)
		}
	}
	if domain.Status("bogus").Valid() {
		t.Fatal("bogus status valid")
	}
	if _, err := domain.ParseStatus("runningg"); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateCommand(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		cmd     []string
		wantErr bool
	}{
		{"ok", []string{"npm", "run", "dev"}, false},
		{"single", []string{"sleep"}, false},
		{"empty_list", nil, true},
		{"empty_slice", []string{}, true},
		{"empty_arg", []string{"echo", ""}, true},
		{"leading_empty", []string{"", "x"}, true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := domain.ValidateCommand(tc.cmd)
			if tc.wantErr {
				if !errors.Is(err, domain.ErrInvalidCommand) {
					t.Fatalf("err = %v, want ErrInvalidCommand", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProcessValidate(t *testing.T) {
	t.Parallel()
	p := &domain.Process{
		Name:    "web",
		Command: []string{"true"},
		Status:  domain.StatusRunning,
		Desired: domain.DesiredRunning,
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := &domain.Process{Name: "x", Command: nil}
	if err := bad.Validate(); !errors.Is(err, domain.ErrInvalidCommand) {
		t.Fatalf("err = %v", err)
	}
	noName := &domain.Process{Command: []string{"true"}}
	if err := noName.Validate(); !errors.Is(err, domain.ErrInvalidProcess) {
		t.Fatalf("err = %v", err)
	}
	if err := (*domain.Process)(nil).Validate(); !errors.Is(err, domain.ErrInvalidProcess) {
		t.Fatalf("nil: %v", err)
	}
}

func TestDomainHasNoIOSImports(t *testing.T) {
	t.Parallel()
	// Structural: parse package source and reject OS/network imports.
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	dir := filepath.Dir(file)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	// Allow-list: the pure domain layer may import only these stdlib
	// packages. Anything else — a third-party dep, os/net/io, crypto/rand — must
	// fail closed rather than slip through a deny-list gap.
	allowed := map[string]struct{}{
		"errors":  {},
		"fmt":     {},
		"time":    {},
		"strings": {},
		"slices":  {},
		"cmp":     {},
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imp := range f.Imports {
			imported := strings.Trim(imp.Path.Value, `"`)
			if _, ok := allowed[imported]; !ok {
				t.Errorf("%s imports %q which is not on the domain allow-list", e.Name(), imported)
			}
		}
	}
}
