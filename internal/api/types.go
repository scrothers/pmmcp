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

// Package api holds shared private IPC request/response types.
package api

import "time"

// APIVersion is the IPC major.minor negotiated at handshake.
const APIVersion = "1.0"

// Request is a single framed JSON-RPC-style call over private IPC.
type Request struct {
	APIVersion string `json:"api_version"`
	Method     string `json:"method"`
	// Session is the client session id (sess- or harness id).
	Session string `json:"session,omitempty"`
	// Role is the authz role pack (agent, full, …).
	Role string `json:"role,omitempty"`
	// Payload is method-specific JSON.
	Payload []byte `json:"payload,omitempty"`
}

// Response is a framed result.
type Response struct {
	OK        bool   `json:"ok"`
	ErrorCode string `json:"error_code,omitempty"`
	Error     string `json:"error,omitempty"`
	Retryable bool   `json:"retryable,omitempty"`
	Payload   []byte `json:"payload,omitempty"`
}

// Methods — names map 1:1 to MCP tools via registry (see cli/mcp).
const (
	MethodHello           = "hello"
	MethodStart           = "process.start"
	MethodStop            = "process.stop"
	MethodRestart         = "process.restart"
	MethodUpdate          = "process.update"
	MethodList            = "process.list"
	MethodStatus          = "process.status"
	MethodRemove          = "process.remove"
	MethodRun             = "process.run"
	MethodWait            = "process.wait"
	MethodEnable          = "process.enable"
	MethodDisable         = "process.disable"
	MethodHealthCheck     = "process.health_check"
	MethodLogs            = "logs.tail"
	MethodGrep            = "logs.grep"
	MethodErrors          = "logs.errors"
	MethodLogsExport      = "logs.export"
	MethodLogsShip        = "logs.ship"
	MethodLogsSubscribe   = "logs.subscribe"
	MethodLogsUnsub       = "logs.unsubscribe"
	MethodEvents          = "events.query"
	MethodEventsSub       = "events.subscribe"
	MethodEventsUnsub     = "events.unsubscribe"
	MethodEventsSubs      = "events.subscriptions"
	MethodAudit           = "audit.query"
	MethodDaemonInfo      = "daemon.info"
	MethodDaemonReload    = "daemon.reload"
	MethodProjectCurrent  = "project.current"
	MethodProjectList     = "project.list"
	MethodWhoami          = "whoami"
	MethodGroupCreate     = "group.create"
	MethodGroupRemove     = "group.remove"
	MethodGroupList       = "group.list"
	MethodGroupStatus     = "group.status"
	MethodGroupStart      = "group.start"
	MethodGroupStop       = "group.stop"
	MethodGroupRestart    = "group.restart"
	MethodProfileList     = "profile.list"
	MethodProfileGet      = "profile.get"
	MethodProfileCreate   = "profile.create"
	MethodProfileUpdate   = "profile.update"
	MethodProfileDelete   = "profile.delete"
	MethodProfileUse      = "profile.use"
	MethodSessionInfo     = "session.info"
	MethodSessionEnd      = "session.end"
	MethodShare           = "session.share"
	MethodUnshare         = "session.unshare"
	MethodValidate        = "declare.validate"
	MethodDiff            = "declare.diff"
	MethodApply           = "declare.apply"
	MethodDeclareShow     = "declare.show"
	MethodImport          = "declare.import"
	MethodPorts           = "process.ports"
	MethodRuntimeInfo     = "runtime.info"
	MethodSandboxProfiles = "sandbox.profiles"
	MethodSecretList      = "secret.list"
	MethodSecretRefCheck  = "secret.ref_check"
	MethodSecretSet       = "secret.set"
	MethodWatchSet        = "watch.set"
	MethodWatchStatus     = "watch.status"
	MethodWebhookCreate   = "webhook.create"
	MethodWebhookUpdate   = "webhook.update"
	MethodWebhookDelete   = "webhook.delete"
	MethodWebhookList     = "webhook.list"
	MethodWebhookTest     = "webhook.test"
	MethodMetrics         = "metrics.snapshot"
)

// AllMethods is the complete IPC method surface (for catalog parity tests).
var AllMethods = []string{
	MethodHello, MethodStart, MethodStop, MethodRestart, MethodUpdate, MethodList, MethodStatus, MethodRemove,
	MethodRun, MethodWait, MethodEnable, MethodDisable, MethodHealthCheck,
	MethodLogs, MethodGrep, MethodErrors, MethodLogsExport, MethodLogsShip, MethodLogsSubscribe, MethodLogsUnsub,
	MethodEvents, MethodEventsSub, MethodEventsUnsub, MethodEventsSubs, MethodAudit,
	MethodDaemonInfo, MethodDaemonReload, MethodProjectCurrent, MethodProjectList, MethodWhoami,
	MethodGroupCreate, MethodGroupRemove, MethodGroupList, MethodGroupStatus, MethodGroupStart, MethodGroupStop, MethodGroupRestart,
	MethodProfileList, MethodProfileGet, MethodProfileCreate, MethodProfileUpdate, MethodProfileDelete, MethodProfileUse,
	MethodSessionInfo, MethodSessionEnd, MethodShare, MethodUnshare,
	MethodValidate, MethodDiff, MethodApply, MethodDeclareShow, MethodImport,
	MethodPorts, MethodRuntimeInfo, MethodSandboxProfiles,
	MethodSecretList, MethodSecretRefCheck, MethodSecretSet,
	MethodWatchSet, MethodWatchStatus,
	MethodWebhookCreate, MethodWebhookUpdate, MethodWebhookDelete, MethodWebhookList, MethodWebhookTest,
	MethodMetrics,
}

// HelloResult is returned by MethodHello.
type HelloResult struct {
	APIVersion    string `json:"api_version"`
	DaemonVersion string `json:"daemon_version"`
	UID           string `json:"uid"`
}

// StartPayload starts a process.
type StartPayload struct {
	Name             string            `json:"name"`
	Command          []string          `json:"command"`
	Cwd              string            `json:"cwd,omitempty"`
	Env              map[string]string `json:"env,omitempty"`
	EnvFiles         []string          `json:"env_files,omitempty"`
	Sandbox          string            `json:"sandbox,omitempty"`
	Project          string            `json:"project,omitempty"`
	StopOnDisconnect bool              `json:"stop_on_disconnect,omitempty"`
	HealthURL        string            `json:"health_url,omitempty"`
	AutoRestart      bool              `json:"auto_restart,omitempty"`
	Ports            []string          `json:"ports,omitempty"`
	Runtime          string            `json:"runtime,omitempty"` // local|container
	Image            string            `json:"image,omitempty"`
	Profile          string            `json:"profile,omitempty"`
	MemoryBytes      uint64            `json:"memory_bytes,omitempty"`
	// Replace stops an existing same-scope name before start.
	Replace bool `json:"replace,omitempty"`
}

// StartResult is the started process summary.
type StartResult struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	PID           int    `json:"pid"`
	Status        string `json:"status"`
	LogDir        string `json:"log_dir"`
	PredecessorID string `json:"predecessor_id,omitempty"`
	SuccessorID   string `json:"successor_id,omitempty"`
}

// IDPayload identifies a process by id or name.
type IDPayload struct {
	ID         string `json:"id,omitempty"`
	Name       string `json:"name,omitempty"`
	Project    string `json:"project,omitempty"`
	TimeoutSec int    `json:"timeout_sec,omitempty"`
	// Force skips graceful stop (immediate kill).
	Force bool `json:"force,omitempty"`
	// PurgeLogs deletes the process log directory on remove.
	PurgeLogs bool `json:"purge_logs,omitempty"`
}

// ListPayload filters process list.
type ListPayload struct {
	Project string `json:"project,omitempty"`
	// Cwd when set and Project empty: detect current project and filter to it.
	Cwd    string `json:"cwd,omitempty"`
	Status string `json:"status,omitempty"`
	// IncludeExited when true returns exited/failed; default false.
	IncludeExited bool   `json:"include_exited,omitempty"`
	Runtime       string `json:"runtime,omitempty"`
	Profile       string `json:"profile,omitempty"`
	// Cursor is a best-effort pagination token (process id); empty = start.
	// Pagination is best-effort under churn (documented).
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	// All when true lists all projects (explicit opt-in to cross-project list).
	All bool `json:"all,omitempty"`
}

// ProcessView is a list/status item.
type ProcessView struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Status        string   `json:"status"`
	Desired       string   `json:"desired"`
	PID           int      `json:"pid"`
	Cwd           string   `json:"cwd"`
	Command       []string `json:"command"`
	LogDir        string   `json:"log_dir"`
	Project       string   `json:"project_id"`
	Profile       string   `json:"profile,omitempty"`
	Sandbox       string   `json:"sandbox"`
	Runtime       string   `json:"runtime,omitempty"`
	EnvKeys       []string `json:"env_keys,omitempty"`
	ExitCode      *int     `json:"exit_code,omitempty"`
	Error         string   `json:"error,omitempty"`
	Ports         []string `json:"ports,omitempty"`
	Discovered    []string `json:"discovered,omitempty"`
	PredecessorID string   `json:"predecessor_id,omitempty"`
	SuccessorID   string   `json:"successor_id,omitempty"`
}

// LogsPayload requests log content.
type LogsPayload struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Project  string `json:"project,omitempty"`
	Stream   string `json:"stream,omitempty"`
	Lines    int    `json:"lines,omitempty"`
	Pattern  string `json:"pattern,omitempty"`
	MinLevel string `json:"min_level,omitempty"` // structured/JSON level filter
}

// LogsResult returns log text.
type LogsResult struct {
	Text string `json:"text"`
}

// DaemonInfoResult describes the running daemon.
type DaemonInfoResult struct {
	Version        string    `json:"version"`
	APIVersion     string    `json:"api_version"`
	StateDir       string    `json:"state_dir"`
	Endpoint       string    `json:"endpoint"`
	UptimeSec      int64     `json:"uptime_sec"`
	SandboxDefault string    `json:"sandbox_default"`
	StartedAt      time.Time `json:"started_at"`
	// TokenFile is redacted path hint only (never secret material).
	TokenFile string `json:"token_file,omitempty"`
	// LogLevel is the effective log level.
	LogLevel string `json:"log_level,omitempty"`
}

// WhoamiResult returns principal info.
type WhoamiResult struct {
	UID      string `json:"uid"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Session  string `json:"session"`
}

// ProjectResult is the current project.
type ProjectResult struct {
	Root string `json:"root"`
	Key  string `json:"key"`
}

// EventsPayload queries events.
type EventsPayload struct {
	ProcessID string `json:"process_id,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

// EventView is one event.
type EventView struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	ProcessID string    `json:"process_id"`
	Message   string    `json:"message"`
	At        time.Time `json:"at"`
}

// UpdatePayload mutates a process spec; optional restart applies changes.
type UpdatePayload struct {
	ID      string            `json:"id,omitempty"`
	Name    string            `json:"name,omitempty"`
	Project string            `json:"project,omitempty"`
	Command []string          `json:"command,omitempty"`
	Cwd     string            `json:"cwd,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Restart bool              `json:"restart,omitempty"`
}

// RunPayload starts a oneshot process and optionally waits.
type RunPayload struct {
	StartPayload
	Wait       bool `json:"wait,omitempty"`
	TimeoutSec int  `json:"timeout_sec,omitempty"`
	// Oneshot defaults true for MethodRun when omitted (always oneshot).
	Oneshot *bool `json:"oneshot,omitempty"`
}

// WaitResult is the outcome of process.wait / process.run with wait.
type WaitResult struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	ExitCode *int   `json:"exit_code,omitempty"`
	TimedOut bool   `json:"timed_out,omitempty"`
}

// HealthCheckResult is the outcome of process.health_check.
type HealthCheckResult struct {
	ID      string `json:"id"`
	OK      bool   `json:"ok"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// ProjectListResult lists known projects.
type ProjectListResult struct {
	Projects []ProjectEntry `json:"projects"`
}

// ProjectEntry is one known project key → root.
type ProjectEntry struct {
	Key  string `json:"key"`
	Root string `json:"root"`
}

// GroupPayload creates or identifies a group.
type GroupPayload struct {
	ID        string               `json:"id,omitempty"`
	Name      string               `json:"name,omitempty"`
	ProjectID string               `json:"project_id,omitempty"`
	Project   string               `json:"project,omitempty"`
	Members   []GroupMemberPayload `json:"members,omitempty"`
}

// GroupMemberPayload is a group member in IPC payloads.
type GroupMemberPayload struct {
	Name      string   `json:"name"`
	ProcessID string   `json:"process_id,omitempty"`
	DependsOn []string `json:"depends_on,omitempty"`
}

// GroupView is a group list/status item.
type GroupView struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	ProjectID string            `json:"project_id"`
	Phase     string            `json:"phase,omitempty"`
	Ready     int               `json:"ready,omitempty"`
	Desired   int               `json:"desired,omitempty"`
	Members   []GroupMemberView `json:"members,omitempty"`
}

// GroupMemberView summarizes one group member.
type GroupMemberView struct {
	Name   string `json:"name"`
	Status string `json:"status,omitempty"`
	Ready  bool   `json:"ready"`
}

// ProfilePayload is profile CRUD input.
type ProfilePayload struct {
	ID        string            `json:"id,omitempty"`
	Name      string            `json:"name,omitempty"`
	ProjectID string            `json:"project_id,omitempty"`
	Project   string            `json:"project,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Session   string            `json:"session,omitempty"`
}

// ProfileView is a profile list/get item.
type ProfileView struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	ProjectID string            `json:"project_id"`
	Env       map[string]string `json:"env,omitempty"`
}

// SessionInfoResult describes the current session.
type SessionInfoResult struct {
	ID        string     `json:"id"`
	HarnessID string     `json:"harness_id,omitempty"`
	Role      string     `json:"role"`
	CreatedAt time.Time  `json:"created_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

// SharePayload grants or revokes a capability on a target.
type SharePayload struct {
	Target    string `json:"target"`
	ToSession string `json:"to_session"`
	Cap       string `json:"cap,omitempty"`
}

// DeclarePayload carries YAML or path for declare methods.
type DeclarePayload struct {
	YAML         string   `json:"yaml,omitempty"`
	Path         string   `json:"path,omitempty"`
	RunningNames []string `json:"running_names,omitempty"`
	// Format for import: "procfile" (default).
	Format string `json:"format,omitempty"`
	Data   string `json:"data,omitempty"`
}

// PortsResult returns declared and discovered ports for a process.
type PortsResult struct {
	ID         string   `json:"id"`
	Ports      []string `json:"ports"`
	Discovered []string `json:"discovered"`
}

// RuntimeInfoResult describes local and container engine availability.
type RuntimeInfoResult struct {
	Local   bool            `json:"local"`
	Engines map[string]bool `json:"engines"`
}

// SandboxProfilesResult lists known sandbox profiles.
type SandboxProfilesResult struct {
	Profiles []string `json:"profiles"`
}

// SecretPayload sets or checks a secret path ref (or stores Value in file keyring).
type SecretPayload struct {
	Name  string `json:"name,omitempty"`
	Ref   string `json:"ref,omitempty"` // secret:// URI for ref_check
	Path  string `json:"path,omitempty"`
	Value string `json:"value,omitempty"` // stored via FileBackend; never returned by list
}

// SecretListResult lists secret ref names and paths (not values).
type SecretListResult struct {
	Refs []SecretRefView `json:"refs"`
}

// SecretRefView is one secret path reference.
type SecretRefView struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// WatchPayload configures a path watch for a process.
type WatchPayload struct {
	ID      string `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
	Project string `json:"project,omitempty"`
	Path    string `json:"path,omitempty"`
}

// WatchStatusResult reports configured watches.
type WatchStatusResult struct {
	Watches []WatchView `json:"watches"`
}

// WatchView is one process → path watch.
type WatchView struct {
	ProcessID string `json:"process_id"`
	Path      string `json:"path"`
}

// WebhookPayload creates or updates a webhook.
type WebhookPayload struct {
	ID     string   `json:"id,omitempty"`
	URL    string   `json:"url,omitempty"`
	Events []string `json:"events,omitempty"`
}

// WebhookView is a webhook list item.
type WebhookView struct {
	ID     string   `json:"id"`
	URL    string   `json:"url"`
	Events []string `json:"events,omitempty"`
}

// SubPayload creates or removes a logs/events subscription.
type SubPayload struct {
	ID        string `json:"id,omitempty"`
	ProcessID string `json:"process_id,omitempty"`
}

// SubResult is a subscription id.
type SubResult struct {
	ID string `json:"id"`
}

// LogsExportResult is the path of a written export archive.
type LogsExportResult struct {
	Path string `json:"path"`
}

// LogsShipPayload copies an export to a sink path.
type LogsShipPayload struct {
	ExportPath string `json:"export_path"`
	SinkPath   string `json:"sink_path"`
	// Path is an alias accepted for sink_path.
	Path string `json:"path,omitempty"`
}
