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

package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// CLIRunner shells out to a container CLI (podman or docker).
// It is shared by engine/podman and engine/docker drivers.
type CLIRunner struct {
	// Binary is the CLI name (podman or docker).
	Binary string
	// LookPath resolves Binary; nil uses exec.LookPath.
	LookPath func(file string) (string, error)
	// Command constructs an *exec.Cmd; nil uses exec.CommandContext.
	Command func(ctx context.Context, name string, arg ...string) *exec.Cmd
	// Env holds extra environment variables (KEY=VALUE) appended to the process
	// environment for every invocation — used, for example, to pass DOCKER_HOST
	// to the docker CLI. Empty means the command inherits the daemon's env.
	Env []string
}

// path resolves the CLI binary path.
func (r *CLIRunner) path() (string, error) {
	look := r.LookPath
	if look == nil {
		look = exec.LookPath
	}
	p, err := look(r.Binary)
	if err != nil {
		return "", fmt.Errorf("%w: %s not found: %w", ErrUnavailable, r.Binary, err)
	}
	return p, nil
}

// Available reports whether the binary is on PATH.
func (r *CLIRunner) Available(ctx context.Context) bool {
	if err := ctx.Err(); err != nil {
		return false
	}
	_, err := r.path()
	return err == nil
}

// Run starts a detached container and returns its ID.
func (r *CLIRunner) Run(ctx context.Context, spec RunSpec) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if spec.Image == "" {
		return "", fmt.Errorf("%w: empty image", ErrInvalidSpec)
	}
	bin, err := r.path()
	if err != nil {
		return "", err
	}

	args := runArgs(spec)

	out, err := r.run(ctx, bin, args...)
	if err != nil {
		return "", fmt.Errorf("engine/%s: run: %w", r.Binary, err)
	}
	id := strings.TrimSpace(out)
	if id == "" {
		return "", fmt.Errorf("engine/%s: run: empty container id", r.Binary)
	}
	return id, nil
}

// Stop stops a container, optionally with a stop timeout in seconds.
func (r *CLIRunner) Stop(ctx context.Context, containerID string, timeout time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if containerID == "" {
		return fmt.Errorf("%w: empty container id", ErrNotFound)
	}
	bin, err := r.path()
	if err != nil {
		return err
	}
	args := []string{"stop"}
	if timeout > 0 {
		// Sub-second timeouts round to 0 ("-t 0" = immediate SIGKILL) so a
		// force-now stop is honored rather than floored to a full second.
		secs := int(timeout.Round(time.Second) / time.Second)
		args = append(args, "-t", fmt.Sprintf("%d", secs))
	}
	args = append(args, containerID)
	if _, err := r.run(ctx, bin, args...); err != nil {
		return fmt.Errorf("engine/%s: stop: %w", r.Binary, err)
	}
	return nil
}

// Logs returns container logs, optionally tailed.
func (r *CLIRunner) Logs(ctx context.Context, containerID string, tail int) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if containerID == "" {
		return "", fmt.Errorf("%w: empty container id", ErrNotFound)
	}
	bin, err := r.path()
	if err != nil {
		return "", err
	}
	args := []string{"logs"}
	if tail > 0 {
		args = append(args, "--tail", fmt.Sprintf("%d", tail))
	}
	args = append(args, containerID)
	out, err := r.run(ctx, bin, args...)
	if err != nil {
		return "", fmt.Errorf("engine/%s: logs: %w", r.Binary, err)
	}
	return out, nil
}

// runArgs builds the fully tokenized `run` argv for spec. Env, label, and
// security-opt entries are emitted in a stable order so the argv is
// deterministic (and thus assertable in tests). No shell is involved: every
// value is a distinct argv element, so metacharacters and leading dashes in
// image/env/port strings are inert.
func runArgs(spec RunSpec) []string {
	args := []string{"run", "-d"}
	if !spec.NoRemove {
		args = append(args, "--rm")
	}
	if spec.Name != "" {
		args = append(args, "--name", spec.Name)
	}
	if spec.User != "" {
		args = append(args, "--user", spec.User)
	}
	if spec.Network != "" {
		args = append(args, "--network", spec.Network)
	}
	if spec.ReadOnlyRootfs {
		args = append(args, "--read-only")
	}
	if spec.Privileged {
		args = append(args, "--privileged")
	}
	for _, c := range spec.CapDrop {
		if c != "" {
			args = append(args, "--cap-drop", c)
		}
	}
	for _, c := range spec.CapAdd {
		if c != "" {
			args = append(args, "--cap-add", c)
		}
	}
	for _, o := range spec.SecurityOpt {
		if o != "" {
			args = append(args, "--security-opt", o)
		}
	}
	for _, k := range sortedKeys(spec.Labels) {
		args = append(args, "--label", k+"="+spec.Labels[k])
	}
	for _, k := range sortedKeys(spec.Env) {
		args = append(args, "-e", k+"="+spec.Env[k])
	}
	for _, v := range spec.Volumes {
		if v.Source == "" || v.Target == "" {
			continue
		}
		m := v.Source + ":" + v.Target
		if v.ReadOnly {
			m += ":ro"
		}
		args = append(args, "-v", m)
	}
	for _, p := range spec.Ports {
		if p == "" {
			continue
		}
		if !spec.PublishAllInterfaces {
			p = loopbackPortSpec(p)
		}
		args = append(args, "-p", p)
	}
	args = append(args, spec.Image)
	args = append(args, spec.Command...)
	return args
}

// loopbackPortSpec prefixes 127.0.0.1 to a publish spec that lacks an explicit
// host IP, so strict/standard containers do not expose ports on all interfaces
// (sandbox-strict "Ports bound to loopback by default"). Specs that already
// carry a host IP (bracketed IPv6, or ip:host:container) are returned verbatim.
func loopbackPortSpec(p string) string {
	spec, proto := p, ""
	if i := strings.LastIndex(p, "/"); i >= 0 {
		spec, proto = p[:i], p[i:]
	}
	if strings.HasPrefix(spec, "[") || strings.Count(spec, ":") >= 2 {
		return p
	}
	if strings.Contains(spec, ":") {
		// hostPort:containerPort
		return "127.0.0.1:" + spec + proto
	}
	// Bare containerPort: bind loopback with an engine-chosen host port.
	return "127.0.0.1::" + spec + proto
}

func sortedKeys(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (r *CLIRunner) run(ctx context.Context, bin string, args ...string) (string, error) {
	cmdFn := r.Command
	if cmdFn == nil {
		cmdFn = exec.CommandContext
	}
	cmd := cmdFn(ctx, bin, args...)
	if len(r.Env) > 0 {
		cmd.Env = append(os.Environ(), r.Env...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s %s: %s: %w", r.Binary, strings.Join(args, " "), msg, err)
	}
	return stdout.String(), nil
}

// Version returns the CLI's client and (when the daemon is reachable) server
// version. A populated Server confirms the engine daemon is up — the docker
// driver uses this to tell "binary present" apart from "daemon reachable".
func (r *CLIRunner) Version(ctx context.Context) (VersionInfo, error) {
	if err := ctx.Err(); err != nil {
		return VersionInfo{}, err
	}
	bin, err := r.path()
	if err != nil {
		return VersionInfo{}, err
	}
	out, err := r.run(ctx, bin, "version", "--format", "{{json .}}")
	if err != nil {
		return VersionInfo{}, fmt.Errorf("engine/%s: version: %w", r.Binary, err)
	}
	var v struct {
		Client struct {
			Version string `json:"Version"`
		} `json:"Client"`
		Server struct {
			Version string `json:"Version"`
		} `json:"Server"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &v); err != nil {
		return VersionInfo{}, fmt.Errorf("engine/%s: version: parse: %w", r.Binary, err)
	}
	return VersionInfo{Client: v.Client.Version, Server: v.Server.Version}, nil
}

// Inspect returns the current state of a container; a missing container maps to
// ErrNotFound.
func (r *CLIRunner) Inspect(ctx context.Context, containerID string) (Status, error) {
	if err := ctx.Err(); err != nil {
		return Status{}, err
	}
	if containerID == "" {
		return Status{}, fmt.Errorf("%w: empty container id", ErrNotFound)
	}
	bin, err := r.path()
	if err != nil {
		return Status{}, err
	}
	out, err := r.run(ctx, bin, "inspect", containerID)
	if err != nil {
		return Status{}, r.mapRunErr("inspect", err)
	}
	return parseInspect(out)
}

// Wait blocks until the container exits and returns its exit code.
func (r *CLIRunner) Wait(ctx context.Context, containerID string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if containerID == "" {
		return 0, fmt.Errorf("%w: empty container id", ErrNotFound)
	}
	bin, err := r.path()
	if err != nil {
		return 0, err
	}
	out, err := r.run(ctx, bin, "wait", containerID)
	if err != nil {
		return 0, r.mapRunErr("wait", err)
	}
	code, convErr := strconv.Atoi(strings.TrimSpace(out))
	if convErr != nil {
		return 0, fmt.Errorf("engine/%s: wait: parse exit code %q: %w", r.Binary, strings.TrimSpace(out), convErr)
	}
	return code, nil
}

// Remove deletes a container; force removes a running one (rm -f).
func (r *CLIRunner) Remove(ctx context.Context, containerID string, force bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if containerID == "" {
		return fmt.Errorf("%w: empty container id", ErrNotFound)
	}
	bin, err := r.path()
	if err != nil {
		return err
	}
	args := []string{"rm"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, containerID)
	if _, err := r.run(ctx, bin, args...); err != nil {
		return r.mapRunErr("rm", err)
	}
	return nil
}

// PullImage pre-pulls an image so a later Run does not block on the transfer.
func (r *CLIRunner) PullImage(ctx context.Context, image string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if image == "" {
		return fmt.Errorf("%w: empty image", ErrInvalidSpec)
	}
	bin, err := r.path()
	if err != nil {
		return err
	}
	if _, err := r.run(ctx, bin, "pull", image); err != nil {
		return fmt.Errorf("engine/%s: pull: %w", r.Binary, err)
	}
	return nil
}

// List returns containers whose labels include every entry in labels (running
// or not). Each result is fully populated via Inspect so state and labels are
// available for reconcile.
func (r *CLIRunner) List(ctx context.Context, labels map[string]string) ([]Container, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	bin, err := r.path()
	if err != nil {
		return nil, err
	}
	args := []string{"ps", "--all", "--no-trunc", "--format", "{{.ID}}"}
	for _, k := range sortedKeys(labels) {
		args = append(args, "--filter", "label="+k+"="+labels[k])
	}
	out, err := r.run(ctx, bin, args...)
	if err != nil {
		return nil, fmt.Errorf("engine/%s: ps: %w", r.Binary, err)
	}
	var result []Container
	for id := range strings.FieldsSeq(out) {
		st, ierr := r.Inspect(ctx, id)
		if ierr != nil {
			// A container can vanish between ps and inspect; skip it rather than
			// fail the whole listing.
			continue
		}
		result = append(result, Container{ID: st.ID, Name: st.Name, Image: st.Image, State: st.State, Labels: st.Labels})
	}
	return result, nil
}

// mapRunErr wraps a run error, mapping "no such container/object" to
// ErrNotFound so callers can errors.Is it.
func (r *CLIRunner) mapRunErr(op string, err error) error {
	if isNotFound(err) {
		return fmt.Errorf("engine/%s: %s: %w", r.Binary, op, ErrNotFound)
	}
	return fmt.Errorf("engine/%s: %s: %w", r.Binary, op, err)
}

// isNotFound reports whether err is a docker/podman "no such container" error.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such container") ||
		strings.Contains(msg, "no such object") ||
		strings.Contains(msg, "no container with name or id")
}

// inspectDoc is the docker/podman `inspect` JSON subset pmmcp consumes.
type inspectDoc struct {
	ID    string `json:"Id"`
	Name  string `json:"Name"`
	Image string `json:"Image"`
	State struct {
		Status     string `json:"Status"`
		Running    bool   `json:"Running"`
		ExitCode   int    `json:"ExitCode"`
		OOMKilled  bool   `json:"OOMKilled"`
		StartedAt  string `json:"StartedAt"`
		FinishedAt string `json:"FinishedAt"`
		Health     *struct {
			Status string `json:"Status"`
		} `json:"Health"`
	} `json:"State"`
	Config struct {
		Image  string            `json:"Image"`
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
}

// parseInspect turns `inspect` output (a JSON array) into a Status.
func parseInspect(out string) (Status, error) {
	var docs []inspectDoc
	if err := json.Unmarshal([]byte(out), &docs); err != nil {
		return Status{}, fmt.Errorf("engine: inspect: parse: %w", err)
	}
	if len(docs) == 0 {
		return Status{}, fmt.Errorf("%w: inspect returned no objects", ErrNotFound)
	}
	d := docs[0]
	image := d.Config.Image
	if image == "" {
		image = d.Image
	}
	health := ""
	if d.State.Health != nil {
		health = d.State.Health.Status
	}
	return Status{
		ID:         d.ID,
		Name:       strings.TrimPrefix(d.Name, "/"),
		Image:      image,
		State:      normalizeState(d.State.Status),
		Running:    d.State.Running,
		ExitCode:   d.State.ExitCode,
		OOMKilled:  d.State.OOMKilled,
		Health:     health,
		StartedAt:  parseTime(d.State.StartedAt),
		FinishedAt: parseTime(d.State.FinishedAt),
		Labels:     d.Config.Labels,
	}, nil
}

// normalizeState maps a raw engine state string to a modeled State.
func normalizeState(s string) State {
	switch State(strings.ToLower(strings.TrimSpace(s))) {
	case StateCreated, StateRunning, StatePaused, StateRestarting, StateRemoving, StateExited, StateDead:
		return State(strings.ToLower(strings.TrimSpace(s)))
	default:
		return StateUnknown
	}
}

// parseTime parses an engine RFC3339Nano timestamp; a blank, zero, or invalid
// value yields the zero time.
func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil || t.Year() <= 1 {
		return time.Time{}
	}
	return t
}
