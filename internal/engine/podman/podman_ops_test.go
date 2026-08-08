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

package podman_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/scrothers/pmmcp/internal/engine"
	"github.com/scrothers/pmmcp/internal/engine/podman"
)

// TestHelperProcess is re-executed as the "podman" CLI by fakeCLI.
func TestHelperProcess(_ *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	fmt.Fprint(os.Stdout, os.Getenv("HELPER_STDOUT"))
	if m := os.Getenv("HELPER_STDERR"); m != "" {
		fmt.Fprint(os.Stderr, m)
	}
	code, _ := strconv.Atoi(os.Getenv("HELPER_EXIT"))
	os.Exit(code)
}

type resp struct {
	stdout string
	stderr string
	exit   int
}

func fakeCLI(byCmd map[string]resp) *podman.Engine {
	return podman.NewWithCLI(engine.CLIRunner{
		Binary:   "podman",
		LookPath: func(string) (string, error) { return "/usr/bin/podman", nil },
		Command: func(ctx context.Context, _ string, arg ...string) *exec.Cmd {
			sub := ""
			if len(arg) > 0 {
				sub = arg[0]
			}
			r := byCmd[sub]
			c := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestHelperProcess$")
			c.Env = append(os.Environ(),
				"GO_WANT_HELPER_PROCESS=1",
				"HELPER_STDOUT="+r.stdout,
				"HELPER_STDERR="+r.stderr,
				"HELPER_EXIT="+strconv.Itoa(r.exit),
			)
			return c
		},
	})
}

// TestCapabilities exercises podman's optional-capability delegates so the
// engine satisfies Inspector/Waiter/Remover/ImagePuller/Lister/Versioner and the
// one-line delegations are covered.
func TestCapabilities(t *testing.T) {
	t.Parallel()

	e := fakeCLI(map[string]resp{
		"version": {stdout: `{"Client":{"Version":"5.8.2"},"Server":{"Version":"5.8.2"}}`},
		"inspect": {stdout: `[{"Id":"cid","Name":"svc","State":{"Status":"running","Running":true},"Config":{"Image":"img"}}]`},
		"wait":    {stdout: "0\n"},
		"rm":      {},
		"pull":    {},
		"ps":      {stdout: "cid\n"},
	})
	ctx := context.Background()

	if v, err := e.Version(ctx); err != nil || v.Server != "5.8.2" {
		t.Fatalf("Version = %+v err=%v", v, err)
	}
	if st, err := e.Inspect(ctx, "cid"); err != nil || !st.Running {
		t.Fatalf("Inspect = %+v err=%v", st, err)
	}
	if code, err := e.Wait(ctx, "cid"); err != nil || code != 0 {
		t.Fatalf("Wait = %d err=%v", code, err)
	}
	if err := e.Remove(ctx, "cid", true); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := e.PullImage(ctx, "alpine"); err != nil {
		t.Fatalf("PullImage: %v", err)
	}
	if got, err := e.List(ctx, nil); err != nil || len(got) != 1 {
		t.Fatalf("List = %+v err=%v", got, err)
	}
}

func TestCapabilityErrors(t *testing.T) {
	t.Parallel()
	e := fakeCLI(map[string]resp{"inspect": {stderr: "no such container", exit: 1}})
	if _, err := e.Inspect(context.Background(), "gone"); !errors.Is(err, engine.ErrNotFound) {
		t.Fatalf("Inspect err = %v, want ErrNotFound", err)
	}
}
