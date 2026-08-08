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

package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/scrothers/pmmcp/internal/api"
	"github.com/scrothers/pmmcp/internal/audit"
	"github.com/scrothers/pmmcp/internal/authz"
	"github.com/scrothers/pmmcp/internal/config"
	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/event"
	"github.com/scrothers/pmmcp/internal/group"
	"github.com/scrothers/pmmcp/internal/id"
	ipcpkg "github.com/scrothers/pmmcp/internal/ipc"
	"github.com/scrothers/pmmcp/internal/logcap"
	"github.com/scrothers/pmmcp/internal/observability"
	"github.com/scrothers/pmmcp/internal/ports"
	"github.com/scrothers/pmmcp/internal/process"
	"github.com/scrothers/pmmcp/internal/process/drivers"
	"github.com/scrothers/pmmcp/internal/process/local"
	"github.com/scrothers/pmmcp/internal/profile"
	"github.com/scrothers/pmmcp/internal/project"
	"github.com/scrothers/pmmcp/internal/sandbox"
	sandboxlinux "github.com/scrothers/pmmcp/internal/sandbox/linux"
	"github.com/scrothers/pmmcp/internal/secret"
	"github.com/scrothers/pmmcp/internal/session"
	"github.com/scrothers/pmmcp/internal/store"
	"github.com/scrothers/pmmcp/internal/store/sqlite"
	"github.com/scrothers/pmmcp/internal/supervise"
	"github.com/scrothers/pmmcp/internal/version"
	"github.com/scrothers/pmmcp/internal/watch"
	"github.com/scrothers/pmmcp/internal/webhook"
	"google.golang.org/grpc"
)

// Server is the pmmcpd control plane.
type Server struct {
	cfg *config.Config
	// store is the process record store; normally the same underlying handle
	// as dbStore, but Options.Store (tests) can swap it independently.
	store store.ProcessStore
	// dbStore is non-nil only when Options.Store overrode store above, so
	// Close still releases the real SQLite handle opened for audit/events.
	dbStore  *sqlite.Store
	mgr      process.Manager
	events   *event.Bus
	audit    *audit.Log
	sessions *session.Registry
	started  time.Time
	uid      string

	groups   *group.Registry
	profiles *profile.Store
	hooks    *webhook.Registry
	shares   *authz.ShareBook

	mu                  sync.Mutex
	byName              map[string]string // project\0name -> id
	projects            map[string]string // key -> root
	secrets             map[string]string // ref name -> path (not values)
	watches             map[string]string // process id -> path
	subs                map[string]subInfo
	healthURL           map[string]string // process id -> url
	autoRestart         map[string]bool
	ports               map[string][]string // process id -> declared ports
	procEnv             map[string][]string // process id -> KEY=VAL overlays for restart
	memLimit            map[string]uint64   // process id -> memory bytes
	startTimes          map[string]uint64   // process id -> /proc start-time captured at spawn (PID-reuse guard)
	stopOnDisconnect    map[string]string   // process id -> session id that owns SOD
	watchers            map[string]*watch.Watcher
	router              *process.Router
	keyring             *secret.FileBackend
	deliver             webhookDeliverFunc // webhook delivery seam (injectable for tests)
	webhookPoll         time.Duration      // webhook dispatch poll interval (0 → 2s)
	autoRestartMax      int                // runAutoRestartLoop's RestartPolicy.Max (0 → 20)
	autoRestartBackoff  time.Duration      // runAutoRestartLoop's RestartPolicy.Backoff (0 → 500ms)
	autoRestartTick     time.Duration      // runAutoRestartLoop ticker interval (0 → 500ms)
	webhookRetryBackoff time.Duration      // deliverWithRetry initial backoff (0 → 500ms)
	gracefulStopTimeout time.Duration      // GracefulStop → Stop fallback budget (0 → 5s)
	logsPreviewFollow   time.Duration      // doSubscribe logs-preview follow window (0 → 1.5s)
	ln                  net.Listener
	// runCancel stops daemon-scoped background work. Only the cancel func and
	// done channel are stored — not a context.Context (containedctx).
	runCancel context.CancelFunc
	runDoneCh <-chan struct{}
}

// Options configures the daemon.
type Options struct {
	Config *config.Config
	// DBPath overrides state_dir/pmmcp.db when set (tests).
	DBPath string
	// Manager overrides process manager (tests).
	Manager process.Manager
	// WebhookDeliver overrides the webhook delivery function (tests); nil uses
	// the production SSRF-guarded DeliverEvent path.
	WebhookDeliver func(ctx context.Context, hook webhook.Hook, ev webhook.Event) error
	// WebhookPoll overrides the webhook dispatch poll interval (tests); 0 → 2s.
	WebhookPoll time.Duration
	// AutoRestartMax overrides runAutoRestartLoop's RestartPolicy.Max (tests); 0 → 20.
	AutoRestartMax int
	// AutoRestartBackoff overrides runAutoRestartLoop's RestartPolicy.Backoff (tests); 0 → 500ms.
	AutoRestartBackoff time.Duration
	// AutoRestartTick overrides runAutoRestartLoop's health-scan ticker interval (tests); 0 → 500ms.
	AutoRestartTick time.Duration
	// WebhookRetryBackoff overrides deliverWithRetry's initial backoff (tests); 0 → 500ms.
	// The backoff still doubles between attempts.
	WebhookRetryBackoff time.Duration
	// GracefulStopTimeout overrides how long ListenAndServe lets GracefulStop
	// drain before forcing Stop on shutdown (tests); 0 → 5s.
	GracefulStopTimeout time.Duration
	// LogsPreviewFollow overrides doSubscribe's logs-preview follow window
	// (tests); 0 → 1.5s.
	LogsPreviewFollow time.Duration
	// Store overrides the process store (tests); nil uses the real SQLite-backed
	// store opened from DBPath/StateDir. The SQLite handle is still opened and
	// migrated normally (audit/events always persist to it); this only replaces
	// what New wires up as the process-record store, e.g. to inject a fault on
	// Update/Delete/List without a real database fault.
	Store store.ProcessStore
}

// Test seams for New's rare initialization failures (a broken NSS lookup, a
// migration failure on an already-open handle, a SQLite log constructor
// failure) that can't be forced without root or a second OS user. Each
// defaults to the real implementation; only an internal (package daemon) test
// may reassign them, and must restore the original via t.Cleanup.
var (
	userCurrent       = user.Current
	migrateStore      = func(ctx context.Context, st *sqlite.Store) error { return st.Migrate(ctx) }
	newAuditSQLiteLog = audit.NewSQLiteLog
	newEventSQLiteLog = event.NewSQLiteLog
)

// New creates a Server, opens the store, and prepares the local process manager.
func New(ctx context.Context, opts Options) (*Server, error) {
	if opts.Config == nil {
		return nil, fmt.Errorf("daemon: nil config")
	}
	cfg := opts.Config
	if err := os.MkdirAll(cfg.StateDir, 0o700); err != nil {
		return nil, fmt.Errorf("daemon: state dir: %w", err)
	}
	dbPath := opts.DBPath
	if dbPath == "" {
		dbPath = filepath.Join(cfg.StateDir, "pmmcp.db")
	}
	st, err := sqlite.OpenContext(ctx, dbPath)
	if err != nil {
		return nil, err
	}
	if err := migrateStore(ctx, st); err != nil {
		_ = st.Close()
		return nil, err
	}
	localMgr := local.New()
	router := process.NewRouter(localMgr, drivers.Open)
	mgr := opts.Manager
	if mgr == nil {
		mgr = router
	}
	u, err := userCurrent()
	if err != nil {
		_ = st.Close()
		return nil, fmt.Errorf("daemon: user: %w", err)
	}
	groups, profiles, hooks, shares, projects, secrets, watches, subs, healthURL, autoRestart, ports, procEnv := newProductState(cfg.Webhook.Allowlist)
	kr, err := secret.NewFileBackend(filepath.Join(cfg.StateDir, "keyring"))
	if err != nil {
		_ = st.Close()
		return nil, err
	}
	//: audit + events persist to the shared SQLite handle so the
	// forensic trail survives daemon restarts (not an in-memory ring).
	auditLog, err := newAuditSQLiteLog(st.DB())
	if err != nil {
		_ = st.Close()
		return nil, err
	}
	eventBus, err := newEventSQLiteLog(st.DB())
	if err != nil {
		_ = st.Close()
		return nil, err
	}
	deliverFn := webhookDeliverFunc(defaultWebhookDeliver)
	if opts.WebhookDeliver != nil {
		deliverFn = opts.WebhookDeliver
	}
	autoRestartMax := opts.AutoRestartMax
	if autoRestartMax == 0 {
		autoRestartMax = 20
	}
	autoRestartBackoff := opts.AutoRestartBackoff
	if autoRestartBackoff == 0 {
		autoRestartBackoff = 500 * time.Millisecond
	}
	autoRestartTick := opts.AutoRestartTick
	if autoRestartTick == 0 {
		autoRestartTick = 500 * time.Millisecond
	}
	webhookRetryBackoff := opts.WebhookRetryBackoff
	if webhookRetryBackoff == 0 {
		webhookRetryBackoff = 500 * time.Millisecond
	}
	gracefulStopTimeout := opts.GracefulStopTimeout
	if gracefulStopTimeout == 0 {
		gracefulStopTimeout = 5 * time.Second
	}
	logsPreviewFollow := opts.LogsPreviewFollow
	if logsPreviewFollow == 0 {
		logsPreviewFollow = 1500 * time.Millisecond
	}
	procStore := store.ProcessStore(st)
	var dbStore *sqlite.Store
	if opts.Store != nil {
		procStore = opts.Store
		// st is no longer reachable via s.store (Close only closes that), so
		// track it separately to still close the real handle backing audit/events.
		dbStore = st
	}
	return &Server{
		cfg:                 cfg,
		store:               procStore,
		dbStore:             dbStore,
		mgr:                 mgr,
		router:              router,
		keyring:             kr,
		deliver:             deliverFn,
		webhookPoll:         opts.WebhookPoll,
		autoRestartMax:      autoRestartMax,
		autoRestartBackoff:  autoRestartBackoff,
		autoRestartTick:     autoRestartTick,
		webhookRetryBackoff: webhookRetryBackoff,
		gracefulStopTimeout: gracefulStopTimeout,
		logsPreviewFollow:   logsPreviewFollow,
		events:              eventBus,
		audit:               auditLog,
		sessions:            session.NewRegistry(),
		started:             time.Now().UTC(),
		uid:                 u.Uid,
		groups:              groups,
		profiles:            profiles,
		hooks:               hooks,
		shares:              shares,
		byName:              make(map[string]string),
		projects:            projects,
		secrets:             secrets,
		watches:             watches,
		subs:                subs,
		healthURL:           healthURL,
		autoRestart:         autoRestart,
		ports:               ports,
		procEnv:             procEnv,
		memLimit:            make(map[string]uint64),
		startTimes:          make(map[string]uint64),
		stopOnDisconnect:    make(map[string]string),
		watchers:            make(map[string]*watch.Watcher),
	}, nil
}

// Close releases resources. It cancels the run context so background loops
// (auto-restart, watch dispatch) stop before the store closes, avoiding a
// goroutine leak on New+Close without a ctx cancellation.
func (s *Server) Close() error {
	if s.runCancel != nil {
		s.runCancel()
	}
	if s.ln != nil {
		_ = s.ln.Close()
	}
	err := s.store.Close()
	if s.dbStore != nil {
		if cerr := s.dbStore.Close(); err == nil {
			err = cerr
		}
	}
	return err
}

// ListenAndServe serves private IPC until ctx is cancelled.
// Transport is gRPC over UDS (Linux/macOS) or named pipe (Windows) —.
// Accept path enforces same-UID peer credentials.
func (s *Server) ListenAndServe(ctx context.Context) error {
	endpoint := s.cfg.IPC.Endpoint
	ln, err := ipcpkg.Listen(endpoint)
	if err != nil {
		return fmt.Errorf("daemon: listen: %w", err)
	}
	s.ln = ln
	runCtx, cancel := context.WithCancel(ctx)
	s.runCancel = cancel
	s.runDoneCh = runCtx.Done()

	if s.cfg.Relaunch.Enabled {
		if err := s.RelaunchEligible(runCtx); err != nil {
			// Boot relaunch failures are non-fatal; processes can be started manually.
			_, _ = s.audit.Append(runCtx, audit.Record{Action: "daemon.relaunch", Detail: err.Error()})
		}
	}
	// Background supervision: crash/unhealthy auto-restart for opted-in processes.
	go s.runAutoRestartLoop(runCtx)
	// Resume any watches that were set (in-memory only for this process lifetime).
	go s.runWatchDispatchers(runCtx)
	// Deliver domain events to registered webhooks.
	go s.runWebhookDispatch(runCtx)

	gs := grpc.NewServer()
	s.RegisterGRPC(gs)

	go func() {
		<-ctx.Done()
		if s.runCancel != nil {
			s.runCancel()
		}
		// Streams derive their cancellation from runCtx (see runDone), so they
		// return promptly; a bounded fallback hard-stops if a drain stalls so
		// shutdown never hangs on a live stream.
		stopped := make(chan struct{})
		go func() {
			gs.GracefulStop()
			close(stopped)
		}()
		select {
		case <-stopped:
		case <-time.After(s.gracefulStopTimeout):
			gs.Stop()
		}
		_ = ln.Close()
		s.stopAllWatchers()
	}()

	// gRPC over private transport; peer-cred filtered listener.
	if err := gs.Serve(ln); err != nil {
		select {
		case <-ctx.Done():
			return nil
		default:
			return fmt.Errorf("daemon: grpc serve: %w", err)
		}
	}
	return nil
}

// RelaunchEligible lists store records with desired=running and starts them.
func (s *Server) RelaunchEligible(ctx context.Context) error {
	list, err := s.store.List(ctx, store.ProcessFilter{})
	if err != nil {
		return fmt.Errorf("daemon: relaunch list: %w", err)
	}
	var first error
	for _, rec := range list {
		if !supervise.EligibleForRelaunch(rec.Desired, supervise.RestartPolicy{}) {
			continue
		}
		s.mu.Lock()
		s.byName[nameScopeKey(rec.ProjectID, rec.Profile, rec.Name)] = rec.ID
		if rec.ProjectID != "" {
			if _, ok := s.projects[rec.ProjectID]; !ok {
				s.projects[rec.ProjectID] = rec.ProjectID
			}
		}
		env := append([]string(nil), s.procEnv[rec.ID]...)
		s.mu.Unlock()

		// Skip if already running under the manager.
		if h, err := s.mgr.Inspect(ctx, rec.ID); err == nil && h != nil && h.Status == domain.StatusRunning {
			continue
		}
		// Children spawned with Setpgid survive daemon exit. If the persisted
		// PID is still alive, adopt it rather than start a second instance
		// (double writers to the same LogDir, port conflicts).
		if rec.PID > 0 && pidAlive(rec.PID) {
			continue
		}
		h, err := s.mgr.Start(ctx, process.StartSpec{
			ID: rec.ID, Name: rec.Name, Command: rec.Command, Cwd: rec.Cwd,
			Env: env, LogDir: rec.LogDir, Sandbox: rec.Sandbox,
		})
		if err != nil {
			if first == nil {
				first = err
			}
			rec.Status = domain.StatusFailed
			rec.LastError = err.Error()
			_ = s.store.Update(ctx, rec)
			continue
		}
		rec.Status = domain.StatusRunning
		rec.PID = h.PID
		rec.ExitCode = nil
		now := time.Now().UTC()
		rec.StartedAt = &now
		rec.ExitedAt = nil
		rec.LastError = ""
		_ = s.store.Update(ctx, rec)
		s.recordStartTime(rec.ID, h.PID)
		_, _ = s.events.Append(ctx, event.Event{Type: "process.relaunched", ProcessID: rec.ID, Message: rec.Name})
	}
	return first
}

func (s *Server) handle(ctx context.Context, req api.Request) api.Response {
	//: fail closed on major mismatch or client-minor-newer, with both
	// versions in the message. An empty client version is a mismatch (the real
	// client always sends one) — not a bypass.
	if err := api.Compatible(req.APIVersion, api.APIVersion); err != nil {
		var de *domain.Error
		if errors.As(err, &de) {
			return errResp(de.Code, de.Message, false)
		}
		return errResp(domain.CodeIPCVersionMismatch, err.Error(), false)
	}
	// Peer UID is enforced at the Accept path (ipc.peerFilterListener). Role
	// packs apply on top of same-user identity.
	p := s.principal(req.Role, req.Session)
	role := p.Role

	switch req.Method {
	case api.MethodHello:
		return s.jsonOK(api.HelloResult{
			APIVersion:    api.APIVersion,
			DaemonVersion: version.Version,
			UID:           s.uid,
		})
	case api.MethodWhoami:
		u, _ := user.Current()
		return s.jsonOK(api.WhoamiResult{
			UID: s.uid, Username: u.Username, Role: string(role), Session: req.Session,
		})
	case api.MethodDaemonInfo:
		if r := s.require(ctx, p, req.Method, authz.CapDaemonInfo); r != nil {
			return *r
		}
		tokenHint := s.cfg.TokenFile
		if tokenHint != "" {
			tokenHint = "[redacted]"
		}
		return s.jsonOK(api.DaemonInfoResult{
			Version:        version.Version,
			APIVersion:     api.APIVersion,
			StateDir:       s.cfg.StateDir,
			Endpoint:       s.cfg.IPC.Endpoint,
			UptimeSec:      int64(time.Since(s.started).Seconds()),
			SandboxDefault: s.cfg.Sandbox.Default,
			StartedAt:      s.started,
			TokenFile:      tokenHint,
			LogLevel:       s.cfg.Log.Level,
		})
	case api.MethodProjectCurrent:
		var pl struct {
			Cwd string `json:"cwd"`
		}
		_ = json.Unmarshal(req.Payload, &pl)
		if pl.Cwd == "" {
			pl.Cwd, _ = os.Getwd()
		}
		root, key, err := project.Detect(ctx, pl.Cwd)
		if err != nil {
			return errFrom(err)
		}
		return s.jsonOK(api.ProjectResult{Root: root, Key: key})
	case api.MethodStart:
		if r := s.require(ctx, p, req.Method, authz.CapProcessStart); r != nil {
			return *r
		}
		return s.doStart(ctx, p, req.Payload)
	case api.MethodStop:
		if r := s.require(ctx, p, req.Method, authz.CapProcessStop); r != nil {
			return *r
		}
		return s.doStop(ctx, p, req.Payload)
	case api.MethodRestart:
		if r := s.require(ctx, p, req.Method, authz.CapProcessRestart); r != nil {
			return *r
		}
		return s.doRestart(ctx, p, req.Payload)
	case api.MethodList:
		if r := s.require(ctx, p, req.Method, authz.CapProcessList); r != nil {
			return *r
		}
		return s.doList(ctx, req.Payload)
	case api.MethodStatus:
		if r := s.require(ctx, p, req.Method, authz.CapProcessRead); r != nil {
			return *r
		}
		return s.doStatus(ctx, req.Payload)
	case api.MethodRemove:
		if r := s.require(ctx, p, req.Method, authz.CapProcessRemove); r != nil {
			return *r
		}
		return s.doRemove(ctx, p, req.Payload)
	case api.MethodLogs:
		if r := s.require(ctx, p, req.Method, authz.CapLogsRead); r != nil {
			return *r
		}
		return s.doLogs(ctx, req.Payload, "tail")
	case api.MethodGrep:
		if r := s.require(ctx, p, req.Method, authz.CapLogsRead); r != nil {
			return *r
		}
		return s.doLogs(ctx, req.Payload, "grep")
	case api.MethodErrors:
		if r := s.require(ctx, p, req.Method, authz.CapLogsRead); r != nil {
			return *r
		}
		return s.doLogs(ctx, req.Payload, "errors")
	case api.MethodEvents:
		if r := s.require(ctx, p, req.Method, authz.CapEventsRead); r != nil {
			return *r
		}
		return s.doEvents(ctx, req.Payload)
	case api.MethodAudit:
		if r := s.require(ctx, p, req.Method, authz.CapAuditRead); r != nil {
			return *r
		}
		return s.doAudit(ctx, req.Payload)
	default:
		if req.Method == "" {
			return errResp(domain.CodeInvalidArgument, "method required", false)
		}
		// Unknown method → unimplemented (error-model AC).
		if !isKnownMethod(req.Method) {
			return errResp(domain.CodeUnimplemented, "unimplemented method: "+req.Method, false)
		}
		return s.dispatchExtra(ctx, p, req)
	}
}

func isKnownMethod(m string) bool {
	for _, x := range api.AllMethods {
		if x == m {
			return true
		}
	}
	return false
}

func (s *Server) doStart(ctx context.Context, principal authz.Principal, raw []byte) api.Response {
	var pl api.StartPayload
	if err := json.Unmarshal(raw, &pl); err != nil {
		return errResp(domain.CodeInvalidArgument, "bad start payload", false)
	}
	if err := domain.ValidateCommand(pl.Command); err != nil {
		return errResp(domain.CodeInvalidArgument, err.Error(), false)
	}
	if pl.Name == "" {
		return errResp(domain.CodeInvalidArgument, "name required", false)
	}
	sandboxProfile := pl.Sandbox
	if sandboxProfile == "" {
		sandboxProfile = s.cfg.Sandbox.Default
	}
	if sandboxIsRelaxation(s.cfg.Sandbox.Default, sandboxProfile) {
		if r := s.require(ctx, principal, "process.start:sandbox_relax", authz.CapSandboxRelax); r != nil {
			return *r
		}
	}
	proj := pl.Project
	cwd := pl.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	if proj == "" {
		root, key, err := project.Detect(ctx, cwd)
		if err == nil {
			proj = key
			if pl.Cwd == "" {
				cwd = root
			}
		}
	}
	// Name uniqueness among non-terminal processes in (project, profile, name) —.
	nameKey := nameScopeKey(proj, pl.Profile, pl.Name)
	var existingID string
	s.mu.Lock()
	if id, ok := s.byName[nameKey]; ok {
		existingID = id
	}
	s.mu.Unlock()
	if existingID != "" {
		if rec, err := s.store.Get(ctx, existingID); err == nil && rec != nil {
			if rec.Status == domain.StatusExited || rec.Status == domain.StatusFailed {
				// Terminal map entry: free name for reuse without replace.
				s.mu.Lock()
				delete(s.byName, nameKey)
				s.mu.Unlock()
				existingID = ""
			}
		}
	}
	if existingID == "" {
		if list, err := s.store.List(ctx, store.ProcessFilter{ProjectID: proj, Name: pl.Name}); err == nil {
			for _, r := range list {
				if r.Profile != pl.Profile {
					continue
				}
				if r.Status == domain.StatusExited || r.Status == domain.StatusFailed {
					continue
				}
				existingID = r.ID
				break
			}
		}
	}
	if existingID != "" {
		if !pl.Replace {
			return errResp(domain.CodeNameConflict, "name already in use: "+pl.Name, false)
		}
		// replace: stop and remove existing, free name.
		_ = s.mgr.Stop(ctx, existingID, 5*time.Second)
		_ = s.store.Delete(ctx, existingID)
		s.mu.Lock()
		delete(s.byName, nameKey)
		delete(s.procEnv, existingID)
		delete(s.autoRestart, existingID)
		delete(s.ports, existingID)
		delete(s.healthURL, existingID)
		delete(s.stopOnDisconnect, existingID)
		delete(s.startTimes, existingID)
		s.mu.Unlock()
	}
	// Sandbox policy
	pol, err := sandbox.DefaultPolicy(sandbox.Profile(sandboxProfile), cwd)
	if err != nil {
		return errResp(domain.CodeSandboxFailed, err.Error(), false)
	}
	if _, err := sandboxlinux.Apply(ctx, pol); err != nil {
		return errResp(domain.CodeSandboxFailed, err.Error(), false)
	}

	pid, err := id.New(id.Proc)
	if err != nil {
		return errFrom(err)
	}
	logDir := filepath.Join(s.cfg.StateDir, "logs", pid)
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return errFrom(err)
	}
	envMap := map[string]string{}
	for _, f := range pl.EnvFiles {
		loaded, err := secret.LoadEnvFileMaybeSOPS(f)
		if err != nil {
			return errResp(domain.CodeInvalidArgument, err.Error(), false)
		}
		for k, v := range loaded {
			envMap[k] = v
		}
	}
	for k, v := range pl.Env {
		envMap[k] = v
	}
	// Resolve secret:// URIs in env values (never store resolved values in declare files).
	resolved, err := secret.ResolveEnvMap(envMap, secret.ResolveOptions{
		ProjectRoot: cwd,
		Keyring:     s.keyring,
	})
	if err != nil {
		return errResp(domain.CodeInvalidArgument, err.Error(), false)
	}
	// Register the cleartext of values that came from secret:// refs so
	// secret.RedactLine scrubs them from this process's captured logs. Only real
	// resolved secrets are registered — plain env (PATH, etc.) is left untouched,
	// which is why this happens here (the only layer that knows which keys were
	// refs) and not in the process driver.
	for k, orig := range envMap {
		if secret.LooksLikeRef(orig) {
			secret.RegisterNamedValue(k, resolved[k])
		}
	}
	envMap = resolved
	envKeys := make([]string, 0, len(envMap))
	var env []string
	for k, v := range envMap {
		envKeys = append(envKeys, k)
		env = append(env, k+"="+v)
	}
	runtime := pl.Runtime
	if runtime == "" {
		runtime = "local"
	}
	h, err := s.mgr.Start(ctx, process.StartSpec{
		ID:          pid,
		Name:        pl.Name,
		Command:     pl.Command,
		Cwd:         cwd,
		Env:         env,
		LogDir:      logDir,
		Sandbox:     sandboxProfile,
		Runtime:     runtime,
		Image:       pl.Image,
		Ports:       pl.Ports,
		MemoryBytes: pl.MemoryBytes,
	})
	if err != nil {
		return errFrom(err)
	}
	desired := domain.DesiredRunning
	rec := &domain.Process{
		ID: pid, Name: pl.Name, Command: pl.Command, Cwd: cwd,
		Status: domain.StatusRunning, Desired: desired,
		ProjectID: proj, SessionID: principal.Session, Sandbox: sandboxProfile,
		Runtime: runtime, PID: h.PID, LogDir: logDir, Profile: pl.Profile,
		EnvKeys: envKeys,
	}
	now := time.Now().UTC()
	rec.StartedAt = &now
	if err := s.store.Create(ctx, rec); err != nil {
		_ = s.mgr.Stop(ctx, pid, 2*time.Second)
		return errFrom(err)
	}
	s.mu.Lock()
	s.byName[nameKey] = pid
	if proj != "" {
		root := cwd
		if pl.Project != "" {
			root = proj
		}
		s.projects[proj] = root
	}
	if len(env) > 0 {
		s.procEnv[pid] = append([]string(nil), env...)
	}
	if pl.HealthURL != "" {
		s.healthURL[pid] = pl.HealthURL
	}
	if pl.AutoRestart {
		s.autoRestart[pid] = true
	}
	if len(pl.Ports) > 0 {
		s.ports[pid] = append([]string(nil), pl.Ports...)
	}
	if pl.MemoryBytes > 0 {
		s.memLimit[pid] = pl.MemoryBytes
	}
	if pl.StopOnDisconnect && principal.Session != "" {
		s.stopOnDisconnect[pid] = principal.Session
	}
	s.mu.Unlock()
	s.recordStartTime(pid, h.PID)
	_, _ = s.events.Append(ctx, event.Event{Type: "process.started", ProcessID: pid, SessionID: principal.Session, Message: pl.Name})
	_, _ = s.audit.Append(ctx, audit.Record{Action: "process.start", Actor: string(principal.Role), SessionID: principal.Session, Target: pid, Detail: pl.Name})
	return s.jsonOK(api.StartResult{ID: pid, Name: pl.Name, PID: h.PID, Status: string(domain.StatusRunning), LogDir: logDir})
}

func nameScopeKey(project, profile, name string) string {
	return project + "\x00" + profile + "\x00" + name
}

// sandboxIsRelaxation reports whether requested is looser than the configured default.
func sandboxIsRelaxation(cfgDefault, requested string) bool {
	rank := func(p string) int {
		switch sandbox.Profile(strings.ToLower(p)) {
		case sandbox.Strict:
			return 0
		case sandbox.Standard:
			return 1
		case sandbox.Permissive:
			return 2
		case sandbox.Off:
			return 3
		default:
			return 0
		}
	}
	return rank(requested) > rank(cfgDefault)
}

func (s *Server) resolveID(ctx context.Context, idOrName, name, project string) (string, *domain.Process, error) {
	if idOrName != "" {
		p, err := s.store.Get(ctx, idOrName)
		if err != nil {
			return idOrName, p, err
		}
		return idOrName, p, nil
	}
	if name == "" {
		return "", nil, domain.NewError(domain.CodeInvalidArgument, "id or name required", false)
	}
	// Prefer live map for default profile empty.
	s.mu.Lock()
	pid := s.byName[nameScopeKey(project, "", name)]
	s.mu.Unlock()
	if pid != "" {
		p, err := s.store.Get(ctx, pid)
		return pid, p, err
	}
	list, err := s.store.List(ctx, store.ProcessFilter{ProjectID: project, Name: name})
	if err != nil {
		return "", nil, err
	}
	if len(list) == 0 {
		list, err = s.store.List(ctx, store.ProcessFilter{Name: name})
		if err != nil {
			return "", nil, err
		}
	}
	if len(list) == 0 {
		return "", nil, store.ErrNotFound
	}
	// Prefer non-terminal; if multiple live, ambiguous.
	var live []*domain.Process
	for _, r := range list {
		if r.Status != domain.StatusExited && r.Status != domain.StatusFailed {
			live = append(live, r)
		}
	}
	if len(live) > 1 {
		return "", nil, domain.NewError(domain.CodeInvalidArgument,
			"ambiguous name "+name+": multiple processes; specify id", false)
	}
	if len(live) == 1 {
		return live[0].ID, live[0], nil
	}
	return list[0].ID, list[0], nil
}

func (s *Server) doStop(ctx context.Context, principal authz.Principal, raw []byte) api.Response {
	var pl api.IDPayload
	_ = json.Unmarshal(raw, &pl)
	pid, rec, err := s.resolveID(ctx, pl.ID, pl.Name, pl.Project)
	if err != nil {
		return errFrom(err)
	}
	if r := s.authorizeTarget(ctx, principal, "process.stop", authz.CapProcessStop, rec); r != nil {
		return *r
	}
	// Idempotent: already terminal → already_stopped.
	if rec.Status == domain.StatusExited || rec.Status == domain.StatusFailed {
		return s.jsonOK(map[string]any{"id": pid, "status": string(rec.Status), "already_stopped": true})
	}
	timeout := 10 * time.Second
	if pl.TimeoutSec > 0 {
		timeout = time.Duration(pl.TimeoutSec) * time.Second
	}
	if pl.Force {
		// Immediate force path (skip long grace).
		timeout = time.Millisecond
	}
	if err := s.mgr.Stop(ctx, pid, timeout); err != nil {
		return errFrom(err)
	}
	h, _ := s.mgr.Inspect(ctx, pid)
	rec.Status = domain.StatusExited
	rec.Desired = domain.DesiredStopped
	if h != nil {
		rec.ExitCode = h.ExitCode
		if h.Status != "" {
			rec.Status = h.Status
		}
	}
	now := time.Now().UTC()
	rec.ExitedAt = &now
	_ = s.store.Update(ctx, rec)
	_, _ = s.events.Append(ctx, event.Event{Type: "process.stopped", ProcessID: pid, SessionID: principal.Session})
	_, _ = s.audit.Append(ctx, audit.Record{Action: "process.stop", Actor: string(principal.Role), SessionID: principal.Session, Target: pid})
	return s.jsonOK(map[string]any{"id": pid, "status": string(rec.Status), "forced": pl.Force})
}

func (s *Server) doRestart(ctx context.Context, principal authz.Principal, raw []byte) api.Response {
	var pl api.IDPayload
	_ = json.Unmarshal(raw, &pl)
	oldID, rec, err := s.resolveID(ctx, pl.ID, pl.Name, pl.Project)
	if err != nil {
		return errFrom(err)
	}
	if r := s.authorizeTarget(ctx, principal, "process.restart", authz.CapProcessRestart, rec); r != nil {
		return *r
	}
	timeout := 10 * time.Second
	if pl.Force {
		timeout = 100 * time.Millisecond
	}
	_ = s.mgr.Stop(ctx, oldID, timeout)
	s.mu.Lock()
	env := append([]string(nil), s.procEnv[oldID]...)
	ports := append([]string(nil), s.ports[oldID]...)
	mem := s.memLimit[oldID]
	auto := s.autoRestart[oldID]
	health := s.healthURL[oldID]
	sod := s.stopOnDisconnect[oldID]
	s.mu.Unlock()

	newID, err := id.New(id.Proc)
	if err != nil {
		return errFrom(err)
	}
	logDir := filepath.Join(s.cfg.StateDir, "logs", newID)
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return errFrom(err)
	}
	// Prefer prior log dir only if empty command path; always new dir for new ID.
	h, err := s.mgr.Start(ctx, process.StartSpec{
		ID: newID, Name: rec.Name, Command: rec.Command, Cwd: rec.Cwd,
		Env: env, LogDir: logDir, Sandbox: rec.Sandbox, Ports: ports, MemoryBytes: mem,
		Runtime: rec.Runtime,
	})
	if err != nil {
		return errFrom(err)
	}
	now := time.Now().UTC()
	// Mark predecessor terminal + linked.
	rec.Status = domain.StatusExited
	rec.Desired = domain.DesiredStopped
	rec.SuccessorID = newID
	rec.ExitedAt = &now
	_ = s.store.Update(ctx, rec)

	succ := &domain.Process{
		ID: newID, Name: rec.Name, Command: rec.Command, Cwd: rec.Cwd,
		Status: domain.StatusRunning, Desired: domain.DesiredRunning,
		ProjectID: rec.ProjectID, Profile: rec.Profile, SessionID: principal.Session,
		Sandbox: rec.Sandbox, Runtime: rec.Runtime, PID: h.PID, LogDir: logDir,
		EnvKeys: append([]string(nil), rec.EnvKeys...), PredecessorID: oldID,
		StartedAt: &now,
	}
	if err := s.store.Create(ctx, succ); err != nil {
		_ = s.mgr.Stop(ctx, newID, 2*time.Second)
		return errFrom(err)
	}
	nameKey := nameScopeKey(rec.ProjectID, rec.Profile, rec.Name)
	s.mu.Lock()
	s.byName[nameKey] = newID
	if len(env) > 0 {
		s.procEnv[newID] = env
	}
	delete(s.procEnv, oldID)
	if len(ports) > 0 {
		s.ports[newID] = ports
	}
	delete(s.ports, oldID)
	if mem > 0 {
		s.memLimit[newID] = mem
	}
	delete(s.memLimit, oldID)
	if auto {
		s.autoRestart[newID] = true
	}
	delete(s.autoRestart, oldID)
	if health != "" {
		s.healthURL[newID] = health
	}
	delete(s.healthURL, oldID)
	if sod != "" {
		s.stopOnDisconnect[newID] = sod
	}
	delete(s.stopOnDisconnect, oldID)
	delete(s.startTimes, oldID)
	s.mu.Unlock()
	s.recordStartTime(newID, h.PID)
	_, _ = s.events.Append(ctx, event.Event{
		Type: "process.restarted", ProcessID: newID, SessionID: principal.Session,
		Message: "predecessor=" + oldID,
	})
	//: every mutation writes an audit record — restart included.
	_, _ = s.audit.Append(ctx, audit.Record{
		Action: "process.restart", Actor: string(principal.Role), SessionID: principal.Session,
		Target: oldID, Detail: "successor=" + newID,
	})
	return s.jsonOK(api.StartResult{
		ID: newID, Name: rec.Name, PID: h.PID, Status: string(domain.StatusRunning),
		LogDir: logDir, PredecessorID: oldID,
	})
}

func (s *Server) doList(ctx context.Context, raw []byte) api.Response {
	var pl api.ListPayload
	_ = json.Unmarshal(raw, &pl)
	proj := pl.Project
	if proj == "" && !pl.All {
		cwd := pl.Cwd
		if cwd == "" {
			cwd, _ = os.Getwd()
		}
		if _, key, err := project.Detect(ctx, cwd); err == nil {
			proj = key
		}
	}
	f := store.ProcessFilter{ProjectID: proj}
	if pl.Status != "" {
		f.Status = domain.Status(pl.Status)
	}
	list, err := s.store.List(ctx, f)
	if err != nil {
		return errFrom(err)
	}
	// Default: hide exited/failed unless include_exited.
	views := make([]api.ProcessView, 0, len(list))
	pastCursor := pl.Cursor == ""
	limit := pl.Limit
	if limit <= 0 {
		limit = 1000
	}
	for _, p := range list {
		if !pastCursor {
			if p.ID == pl.Cursor {
				pastCursor = true
			}
			continue
		}
		if !pl.IncludeExited && pl.Status == "" {
			if p.Status == domain.StatusExited || p.Status == domain.StatusFailed {
				continue
			}
		}
		if pl.Runtime != "" && p.Runtime != pl.Runtime {
			continue
		}
		if pl.Profile != "" && p.Profile != pl.Profile {
			continue
		}
		views = append(views, toView(p))
		if len(views) >= limit {
			break
		}
	}
	return s.jsonOK(views)
}

func (s *Server) doStatus(ctx context.Context, raw []byte) api.Response {
	var pl api.IDPayload
	_ = json.Unmarshal(raw, &pl)
	_, rec, err := s.resolveID(ctx, pl.ID, pl.Name, pl.Project)
	if err != nil {
		return errFrom(err)
	}
	if h, err := s.mgr.Inspect(ctx, rec.ID); err == nil && h != nil {
		rec.PID = h.PID
		if h.Status != "" {
			rec.Status = h.Status
		}
		rec.ExitCode = h.ExitCode
	}
	view := toView(rec)
	s.mu.Lock()
	view.Ports = append([]string(nil), s.ports[rec.ID]...)
	s.mu.Unlock()
	if rec.PID > 0 {
		view.Discovered = ports.DiscoverListeningPorts(rec.PID)
	}
	if view.Discovered == nil {
		view.Discovered = []string{}
	}
	if view.Ports == nil {
		view.Ports = []string{}
	}
	return s.jsonOK(view)
}

func (s *Server) doRemove(ctx context.Context, principal authz.Principal, raw []byte) api.Response {
	var pl api.IDPayload
	_ = json.Unmarshal(raw, &pl)
	pid, rec, err := s.resolveID(ctx, pl.ID, pl.Name, pl.Project)
	if err != nil {
		return errFrom(err)
	}
	if r := s.authorizeTarget(ctx, principal, "process.remove", authz.CapProcessRemove, rec); r != nil {
		return *r
	}
	_ = s.mgr.Stop(ctx, pid, 5*time.Second)
	if err := s.store.Delete(ctx, pid); err != nil {
		return errFrom(err)
	}
	purged := false
	if pl.PurgeLogs && rec.LogDir != "" {
		if err := os.RemoveAll(rec.LogDir); err == nil {
			purged = true
		}
	}
	// Warning when name appears in an applied declare unit ( coupling).
	declareWarn := ""
	s.mu.Lock()
	delete(s.byName, nameScopeKey(rec.ProjectID, rec.Profile, rec.Name))
	delete(s.healthURL, pid)
	delete(s.autoRestart, pid)
	delete(s.ports, pid)
	delete(s.procEnv, pid)
	delete(s.watches, pid)
	delete(s.stopOnDisconnect, pid)
	delete(s.startTimes, pid)
	// Best-effort: if a declare path is known under project, warn.
	if root, ok := s.projects[rec.ProjectID]; ok {
		for _, name := range []string{"pmmcp.yaml", "pmmcp.yml"} {
			if _, err := os.Stat(filepath.Join(root, name)); err == nil {
				declareWarn = "unit may be recreated by pmmcp apply if still declared"
				break
			}
		}
	}
	s.mu.Unlock()
	detail := rec.Name
	if pl.PurgeLogs {
		detail += " purge_logs=true"
	}
	_, _ = s.audit.Append(ctx, audit.Record{
		Action: "process.remove", Actor: string(principal.Role), SessionID: principal.Session,
		Target: pid, Detail: detail,
	})
	out := map[string]any{"id": pid, "name": rec.Name, "removed": true, "purge_logs": purged}
	if declareWarn != "" {
		out["warning"] = declareWarn
	}
	return s.jsonOK(out)
}

func (s *Server) doLogs(ctx context.Context, raw []byte, mode string) api.Response {
	var pl api.LogsPayload
	_ = json.Unmarshal(raw, &pl)
	_, rec, err := s.resolveID(ctx, pl.ID, pl.Name, pl.Project)
	if err != nil {
		return errFrom(err)
	}
	stream := pl.Stream
	if stream == "" {
		stream = "both"
	}
	lines := pl.Lines
	if lines <= 0 {
		lines = 100
	}
	var text string
	switch {
	case pl.MinLevel != "":
		text, err = logcap.FilterLevel(rec.LogDir, logcap.StructuredOptions{Stream: stream, MinLevel: pl.MinLevel, Lines: lines})
	case mode == "grep":
		text, err = logcap.Grep(rec.LogDir, logcap.GrepOptions{Stream: stream, Pattern: pl.Pattern, MaxMatches: 50})
	case mode == "errors":
		text, err = logcap.Errors(rec.LogDir, logcap.ErrorsOptions{Stream: stream, Lines: lines})
	default:
		text, err = logcap.Tail(rec.LogDir, logcap.TailOptions{Stream: stream, Lines: lines})
	}
	if err != nil {
		return errFrom(err)
	}
	// Defense in depth: redact on read as well as capture.
	text = redactText(text)
	return s.jsonOK(api.LogsResult{Text: text})
}

func (s *Server) doEvents(ctx context.Context, raw []byte) api.Response {
	var pl api.EventsPayload
	_ = json.Unmarshal(raw, &pl)
	evs := s.events.Query(ctx, pl.ProcessID, pl.Limit)
	out := make([]api.EventView, 0, len(evs))
	for _, e := range evs {
		out = append(out, api.EventView{ID: e.ID, Type: e.Type, ProcessID: e.ProcessID, Message: e.Message, At: e.At})
	}
	return s.jsonOK(out)
}

func (s *Server) doAudit(ctx context.Context, raw []byte) api.Response {
	var pl struct {
		Target string `json:"target"`
		Limit  int    `json:"limit"`
	}
	_ = json.Unmarshal(raw, &pl)
	recs := s.audit.Query(ctx, pl.Target, pl.Limit)
	return s.jsonOK(recs)
}

func toView(p *domain.Process) api.ProcessView {
	keys := p.EnvKeys
	if keys == nil {
		keys = []string{}
	}
	return api.ProcessView{
		ID: p.ID, Name: p.Name, Status: string(p.Status), Desired: string(p.Desired),
		PID: p.PID, Cwd: p.Cwd, Command: p.Command, LogDir: p.LogDir,
		Project: p.ProjectID, Profile: p.Profile, Sandbox: p.Sandbox, Runtime: p.Runtime,
		EnvKeys: keys, ExitCode: p.ExitCode, Error: p.LastError,
		PredecessorID: p.PredecessorID, SuccessorID: p.SuccessorID,
	}
}

func (s *Server) jsonOK(v any) api.Response {
	b, err := json.Marshal(v)
	if err != nil {
		return errResp(domain.CodeInternal, err.Error(), false)
	}
	return api.Response{OK: true, Payload: b}
}

func errResp(code domain.Code, msg string, retry bool) api.Response {
	return api.Response{OK: false, ErrorCode: string(code), Error: msg, Retryable: retry}
}

// principal builds an authz principal for a client-asserted role/session on top
// of the daemon's same-UID identity. An empty role defaults to full (same-user
// cooperative model); tightening the default is tracked separately.
func (s *Server) principal(role, session string) authz.Principal {
	r := authz.Role(role)
	if r == "" {
		r = authz.RoleFull
	}
	return authz.Principal{UID: s.uid, Role: r, Session: session}
}

// recordStartTime captures the OS start-time of a freshly spawned PID so metrics
// can detect PID reuse before attributing /proc counters (a zero on failure just
// skips the reuse check for that process).
func (s *Server) recordStartTime(id string, pid int) {
	if pid <= 0 {
		return
	}
	st, err := observability.ReadStartTime(pid)
	if err != nil {
		return
	}
	s.mu.Lock()
	s.startTimes[id] = st
	s.mu.Unlock()
}

// runDone returns the daemon run-context's done channel, or a nil channel
// (which never fires in a select) when the run context is not yet set. Long
// streams select on this so a daemon shutdown cancels them promptly.
func (s *Server) runDone() <-chan struct{} {
	return s.runDoneCh
}

// daemonCtx returns a context that inherits values from parent and is canceled
// when the daemon stops (or when parent is already done-linked via WithoutCancel
// for the RPC end). Background work (watches) must outlive individual calls.
func (s *Server) daemonCtx(parent context.Context) context.Context {
	base := context.WithoutCancel(parent)
	done := s.runDone()
	if done == nil {
		return base
	}
	ctx, cancel := context.WithCancel(base)
	go func() {
		defer cancel()
		select {
		case <-done:
		case <-ctx.Done():
		}
	}()
	return ctx
}

// auditDeny records an authorization denial.
func (s *Server) auditDeny(ctx context.Context, p authz.Principal, method string, capNeeded authz.Capability) {
	_, _ = s.audit.Append(ctx, audit.Record{
		Action: method, Actor: string(p.Role), Role: string(p.Role), SessionID: p.Session,
		Target: method, Outcome: audit.OutcomeDenied, Capability: string(capNeeded),
		Detail: string(capNeeded),
	})
}

// deny audits an authorization denial and returns the permission_denied response.
func (s *Server) deny(ctx context.Context, p authz.Principal, method string, capNeeded authz.Capability) api.Response {
	s.auditDeny(ctx, p, method, capNeeded)
	return errResp(domain.CodePermissionDenied,
		fmt.Sprintf("permission_denied: %s lacks %s", p.Role, capNeeded), false)
}

// require checks a capability, auditing and returning a response on denial.
// A nil response means the check passed.
func (s *Server) require(ctx context.Context, p authz.Principal, method string, capNeeded authz.Capability) *api.Response {
	if err := authz.Require(p, capNeeded); err != nil {
		r := s.deny(ctx, p, method, capNeeded)
		return &r
	}
	return nil
}

// authorizeTarget enforces cross-session tenancy: a
// principal holding the role capability may still only act on a process owned by
// another session if it is operator/full or holds an explicit ShareBook grant
// for that capability on the target. Returns a non-nil audited denial otherwise.
func (s *Server) authorizeTarget(ctx context.Context, p authz.Principal, method string, capNeeded authz.Capability, rec *domain.Process) *api.Response {
	if rec == nil || rec.SessionID == "" || rec.SessionID == p.Session {
		return nil
	}
	if p.Role == authz.RoleFull || p.Role == authz.RoleOperator {
		return nil
	}
	if s.shares.Allowed(rec.ID, p.Session, capNeeded) {
		return nil
	}
	r := s.deny(ctx, p, method, capNeeded)
	return &r
}

// redactText applies defense-in-depth secret redaction line-by-line to log text.
func redactText(text string) string {
	if text == "" {
		return text
	}
	var b strings.Builder
	for i, line := range strings.Split(text, "\n") {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(secret.RedactLine(line))
	}
	return b.String()
}

func errFrom(err error) api.Response {
	if err == nil {
		return errResp(domain.CodeInternal, "nil error", false)
	}
	if errors.Is(err, process.ErrSandboxFailed) {
		return errResp(domain.CodeSandboxFailed, err.Error(), false)
	}
	if errors.Is(err, process.ErrInvalidSpec) {
		return errResp(domain.CodeInvalidArgument, err.Error(), false)
	}
	if errors.Is(err, process.ErrAlreadyExists) {
		return errResp(domain.CodeConflict, err.Error(), false)
	}
	if errors.Is(err, process.ErrNotFound) {
		return errResp(domain.CodeNotFound, err.Error(), false)
	}
	var de *domain.Error
	if errors.As(err, &de) {
		return errResp(de.Code, de.Message, de.Retryable)
	}
	if errors.Is(err, store.ErrNotFound) {
		return errResp(domain.CodeNotFound, err.Error(), false)
	}
	if errors.Is(err, store.ErrConflict) {
		return errResp(domain.CodeConflict, err.Error(), false)
	}
	return errResp(domain.CodeInternal, err.Error(), false)
}
