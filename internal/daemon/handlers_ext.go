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
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/scrothers/pmmcp/internal/api"
	"github.com/scrothers/pmmcp/internal/audit"
	"github.com/scrothers/pmmcp/internal/authz"
	"github.com/scrothers/pmmcp/internal/config"
	"github.com/scrothers/pmmcp/internal/declare"
	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/engine/docker"
	"github.com/scrothers/pmmcp/internal/engine/podman"
	"github.com/scrothers/pmmcp/internal/event"
	"github.com/scrothers/pmmcp/internal/group"
	"github.com/scrothers/pmmcp/internal/id"
	"github.com/scrothers/pmmcp/internal/logcap"
	"github.com/scrothers/pmmcp/internal/observability"
	portsd "github.com/scrothers/pmmcp/internal/ports"
	"github.com/scrothers/pmmcp/internal/profile"
	"github.com/scrothers/pmmcp/internal/sandbox"
	"github.com/scrothers/pmmcp/internal/secret"
	"github.com/scrothers/pmmcp/internal/session"
	"github.com/scrothers/pmmcp/internal/store"
	"github.com/scrothers/pmmcp/internal/supervise"
	"github.com/scrothers/pmmcp/internal/webhook"
)

// dispatchExtra handles product-path methods not covered by the core switch.
func (s *Server) dispatchExtra(ctx context.Context, p authz.Principal, req api.Request) api.Response {
	switch req.Method {
	case api.MethodDaemonReload:
		if r := s.require(ctx, p, req.Method, authz.CapDaemonReload); r != nil {
			return *r
		}
		return s.doDaemonReload(ctx, p)
	case api.MethodProjectList:
		return s.doProjectList(ctx, p)
	case api.MethodUpdate:
		if r := s.require(ctx, p, req.Method, authz.CapProcessRestart); r != nil {
			return *r
		}
		return s.doUpdate(ctx, p, req.Payload)
	case api.MethodRun:
		if r := s.require(ctx, p, req.Method, authz.CapProcessStart); r != nil {
			return *r
		}
		return s.doRun(ctx, p, req.Payload)
	case api.MethodWait:
		if r := s.require(ctx, p, req.Method, authz.CapProcessRead); r != nil {
			return *r
		}
		return s.doWait(ctx, p, req.Payload)
	case api.MethodEnable:
		if r := s.require(ctx, p, req.Method, authz.CapProcessStart); r != nil {
			return *r
		}
		return s.doEnable(ctx, p, req.Payload, true)
	case api.MethodDisable:
		if r := s.require(ctx, p, req.Method, authz.CapProcessStop); r != nil {
			return *r
		}
		return s.doEnable(ctx, p, req.Payload, false)
	case api.MethodHealthCheck:
		if r := s.require(ctx, p, req.Method, authz.CapProcessRead); r != nil {
			return *r
		}
		return s.doHealthCheck(ctx, p, req.Payload)
	case api.MethodGroupCreate:
		if r := s.require(ctx, p, req.Method, authz.CapGroupManage); r != nil {
			return *r
		}
		return s.doGroupCreate(ctx, p, req.Payload)
	case api.MethodGroupRemove:
		if r := s.require(ctx, p, req.Method, authz.CapGroupManage); r != nil {
			return *r
		}
		return s.doGroupRemove(ctx, p, req.Payload)
	case api.MethodGroupList:
		return s.doGroupList(ctx, p, req.Payload)
	case api.MethodGroupStatus:
		return s.doGroupStatus(ctx, p, req.Payload)
	case api.MethodGroupStart:
		return s.doGroupLifecycle(ctx, p, req.Payload, "start")
	case api.MethodGroupStop:
		return s.doGroupLifecycle(ctx, p, req.Payload, "stop")
	case api.MethodGroupRestart:
		return s.doGroupLifecycle(ctx, p, req.Payload, "restart")
	case api.MethodProfileList:
		if r := s.require(ctx, p, req.Method, authz.CapProcessRead); r != nil {
			return *r
		}
		return s.doProfileList(ctx, p, req.Payload)
	case api.MethodProfileGet:
		if r := s.require(ctx, p, req.Method, authz.CapProcessRead); r != nil {
			return *r
		}
		return s.doProfileGet(ctx, p, req.Payload)
	case api.MethodProfileCreate:
		if r := s.require(ctx, p, req.Method, authz.CapProfileManage); r != nil {
			return *r
		}
		return s.doProfileCreate(ctx, p, req.Payload)
	case api.MethodProfileUpdate:
		if r := s.require(ctx, p, req.Method, authz.CapProfileManage); r != nil {
			return *r
		}
		return s.doProfileUpdate(ctx, p, req.Payload)
	case api.MethodProfileDelete:
		if r := s.require(ctx, p, req.Method, authz.CapProfileManage); r != nil {
			return *r
		}
		return s.doProfileDelete(ctx, p, req.Payload)
	case api.MethodProfileUse:
		if r := s.require(ctx, p, req.Method, authz.CapProfileManage); r != nil {
			return *r
		}
		return s.doProfileUse(ctx, p, req)
	case api.MethodSessionInfo:
		return s.doSessionInfo(ctx, p, req)
	case api.MethodSessionEnd:
		if r := s.require(ctx, p, req.Method, authz.CapSessionEnd); r != nil {
			return *r
		}
		return s.doSessionEnd(ctx, p, req)
	case api.MethodShare:
		if r := s.require(ctx, p, req.Method, authz.CapSessionShare); r != nil {
			return *r
		}
		return s.doShare(ctx, p, req.Payload, true)
	case api.MethodUnshare:
		if r := s.require(ctx, p, req.Method, authz.CapSessionShare); r != nil {
			return *r
		}
		return s.doShare(ctx, p, req.Payload, false)
	case api.MethodValidate:
		return s.doDeclareValidate(ctx, p, req.Payload)
	case api.MethodDiff:
		return s.doDeclareDiff(ctx, p, req.Payload)
	case api.MethodApply:
		if r := s.require(ctx, p, req.Method, authz.CapDeclareApply); r != nil {
			return *r
		}
		return s.doDeclareApply(ctx, p, req.Payload)
	case api.MethodDeclareShow:
		return s.doDeclareShow(ctx, p, req.Payload)
	case api.MethodImport:
		return s.doDeclareImport(ctx, p, req.Payload)
	case api.MethodPorts:
		if r := s.require(ctx, p, req.Method, authz.CapProcessRead); r != nil {
			return *r
		}
		return s.doPorts(ctx, req.Payload)
	case api.MethodRuntimeInfo:
		return s.doRuntimeInfo(ctx, p)
	case api.MethodSandboxProfiles:
		return s.doSandboxProfiles(ctx, p)
	case api.MethodSecretList:
		return s.doSecretList(ctx, p)
	case api.MethodSecretRefCheck:
		return s.doSecretRefCheck(ctx, p, req.Payload)
	case api.MethodSecretSet:
		if r := s.require(ctx, p, req.Method, authz.CapSecretSet); r != nil {
			return *r
		}
		return s.doSecretSet(ctx, p, req.Payload)
	case api.MethodWatchSet:
		if r := s.require(ctx, p, req.Method, authz.CapWatchSet); r != nil {
			return *r
		}
		return s.doWatchSet(ctx, p, req.Payload)
	case api.MethodWatchStatus:
		return s.doWatchStatus(ctx, p)
	case api.MethodWebhookCreate:
		if r := s.require(ctx, p, req.Method, authz.CapWebhookManage); r != nil {
			return *r
		}
		return s.doWebhookCreate(ctx, p, req.Payload)
	case api.MethodWebhookUpdate:
		if r := s.require(ctx, p, req.Method, authz.CapWebhookManage); r != nil {
			return *r
		}
		return s.doWebhookUpdate(ctx, p, req.Payload)
	case api.MethodWebhookDelete:
		if r := s.require(ctx, p, req.Method, authz.CapWebhookManage); r != nil {
			return *r
		}
		return s.doWebhookDelete(ctx, p, req.Payload)
	case api.MethodWebhookList:
		return s.doWebhookList(ctx, p)
	case api.MethodWebhookTest:
		if r := s.require(ctx, p, req.Method, authz.CapWebhookManage); r != nil {
			return *r
		}
		return s.doWebhookTest(ctx, p, req.Payload)
	case api.MethodMetrics:
		if r := s.require(ctx, p, req.Method, authz.CapDaemonInfo); r != nil {
			return *r
		}
		return s.doMetrics(ctx)
	case api.MethodLogsExport:
		if r := s.require(ctx, p, req.Method, authz.CapLogsExport); r != nil {
			return *r
		}
		return s.doLogsExport(ctx, p, req.Payload)
	case api.MethodLogsShip:
		if r := s.require(ctx, p, req.Method, authz.CapLogsExport); r != nil {
			return *r
		}
		return s.doLogsShip(ctx, p, req.Payload)
	case api.MethodLogsSubscribe:
		if r := s.require(ctx, p, req.Method, authz.CapLogsRead); r != nil {
			return *r
		}
		return s.doSubscribe(ctx, "logs", req.Payload)
	case api.MethodLogsUnsub:
		return s.doUnsubscribe(ctx, req.Payload)
	case api.MethodEventsSub:
		if r := s.require(ctx, p, req.Method, authz.CapEventsRead); r != nil {
			return *r
		}
		return s.doSubscribe(ctx, "events", req.Payload)
	case api.MethodEventsUnsub:
		return s.doUnsubscribe(ctx, req.Payload)
	case api.MethodEventsSubs:
		return s.doListSubs(ctx)
	default:
		return errResp(domain.CodeUnimplemented, "unimplemented method: "+req.Method, false)
	}
}

func (s *Server) doDaemonReload(ctx context.Context, p authz.Principal) api.Response {
	// Capability is enforced at dispatch (CapDaemonReload) with audited denial.
	//: safe subset — re-read config for log.level and sandbox.default (new starts only).
	path := os.Getenv("PMMCP_CONFIG")
	cfg, err := config.Load(config.LoadOptions{Path: path})
	if err != nil {
		detail := "reload failed: " + err.Error()
		_, _ = s.audit.Append(ctx, audit.Record{
			Action: "daemon.reload", Actor: string(p.Role), SessionID: p.Session, Detail: detail,
		})
		return errResp(domain.CodeInvalidArgument, detail, false)
	}
	s.cfg.Log.Level = cfg.Log.Level
	s.cfg.Sandbox.Default = cfg.Sandbox.Default
	// Apply slog level when possible.
	switch strings.ToLower(cfg.Log.Level) {
	case "debug":
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
	case "warn", "warning":
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
	case "error":
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	default:
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))
	}
	detail := "log.level=" + cfg.Log.Level + " sandbox.default=" + cfg.Sandbox.Default
	_, _ = s.audit.Append(ctx, audit.Record{
		Action: "daemon.reload", Actor: string(p.Role), SessionID: p.Session, Detail: detail,
	})
	return s.jsonOK(map[string]string{"status": "ok", "detail": detail})
}

func (s *Server) doProjectList(ctx context.Context, p authz.Principal) api.Response {
	_ = ctx
	if err := authz.Require(p, authz.CapDaemonInfo); err != nil {
		return errResp(domain.CodePermissionDenied, err.Error(), false)
	}
	s.mu.Lock()
	out := make([]api.ProjectEntry, 0, len(s.projects))
	for k, root := range s.projects {
		out = append(out, api.ProjectEntry{Key: k, Root: root})
	}
	s.mu.Unlock()
	return s.jsonOK(api.ProjectListResult{Projects: out})
}

func (s *Server) doUpdate(ctx context.Context, p authz.Principal, raw []byte) api.Response {
	var pl api.UpdatePayload
	if err := json.Unmarshal(raw, &pl); err != nil {
		return errResp(domain.CodeInvalidArgument, "bad update payload", false)
	}
	pid, rec, err := s.resolveID(ctx, pl.ID, pl.Name, pl.Project)
	if err != nil {
		return errFrom(err)
	}
	if len(pl.Command) > 0 {
		if err := domain.ValidateCommand(pl.Command); err != nil {
			return errResp(domain.CodeInvalidArgument, err.Error(), false)
		}
		rec.Command = pl.Command
	}
	if pl.Cwd != "" {
		rec.Cwd = pl.Cwd
	}
	if pl.Env != nil {
		var env []string
		for k, v := range pl.Env {
			env = append(env, k+"="+v)
		}
		s.mu.Lock()
		s.procEnv[pid] = env
		s.mu.Unlock()
	}
	if err := s.store.Update(ctx, rec); err != nil {
		return errFrom(err)
	}
	_, _ = s.audit.Append(ctx, audit.Record{
		Action: "process.update", Actor: string(p.Role), SessionID: p.Session, Target: pid,
	})
	if pl.Restart {
		return s.doRestart(ctx, p, mustJSON(api.IDPayload{ID: pid}))
	}
	return s.jsonOK(toView(rec))
}

func (s *Server) doRun(ctx context.Context, p authz.Principal, raw []byte) api.Response {
	var pl api.RunPayload
	if err := json.Unmarshal(raw, &pl); err != nil {
		// Include the decode detail: MCP agents iterate on this message, and
		// a bare "bad payload" forces guesswork (command must be an argv
		// array, wait a boolean, …).
		return errResp(domain.CodeInvalidArgument, "bad run payload: "+err.Error(), false)
	}
	if pl.Name == "" {
		// The CLI documents --name as optional for one-shot runs, and agents
		// should not have to invent identities for fire-and-forget jobs —
		// derive one so doStart's name-required invariant holds.
		var b [4]byte
		if _, err := rand.Read(b[:]); err != nil {
			return errResp(domain.CodeInternal, "generate run name: "+err.Error(), false)
		}
		pl.Name = "run-" + hex.EncodeToString(b[:])
	}
	// MethodRun is always oneshot: desired stays running during wait then observed exit.
	startRaw, err := json.Marshal(pl.StartPayload)
	if err != nil {
		return errResp(domain.CodeInternal, err.Error(), false)
	}
	resp := s.doStart(ctx, p, startRaw)
	if !resp.OK {
		return resp
	}
	var start api.StartResult
	if err := json.Unmarshal(resp.Payload, &start); err != nil {
		return errResp(domain.CodeInternal, err.Error(), false)
	}
	// Mark oneshot desired stopped so boot relaunch skips after exit.
	if rec, err := s.store.Get(ctx, start.ID); err == nil && rec != nil {
		rec.Desired = domain.DesiredStopped
		_ = s.store.Update(ctx, rec)
	}
	if !pl.Wait {
		return resp
	}
	waitPl := api.IDPayload{ID: start.ID, TimeoutSec: pl.TimeoutSec}
	return s.doWait(ctx, p, mustJSON(waitPl))
}

func (s *Server) doWait(ctx context.Context, p authz.Principal, raw []byte) api.Response {
	var pl api.IDPayload
	_ = json.Unmarshal(raw, &pl)
	pid, rec, err := s.resolveID(ctx, pl.ID, pl.Name, pl.Project)
	if err != nil {
		return errFrom(err)
	}
	waitCtx := ctx
	var cancel context.CancelFunc
	if pl.TimeoutSec > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, time.Duration(pl.TimeoutSec)*time.Second)
		defer cancel()
	}
	h, err := s.mgr.Wait(waitCtx, pid)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || waitCtx.Err() == context.DeadlineExceeded {
			return s.jsonOK(api.WaitResult{ID: pid, Status: string(rec.Status), TimedOut: true})
		}
		return errFrom(err)
	}
	if h != nil {
		rec.Status = h.Status
		if rec.Status == "" {
			rec.Status = domain.StatusExited
		}
		rec.ExitCode = h.ExitCode
		rec.PID = h.PID
		now := time.Now().UTC()
		rec.ExitedAt = &now
		_ = s.store.Update(ctx, rec)
	}
	_, _ = s.events.Append(ctx, event.Event{Type: "process.exited", ProcessID: pid, SessionID: p.Session})
	return s.jsonOK(api.WaitResult{
		ID: pid, Status: string(rec.Status), ExitCode: rec.ExitCode,
	})
}

func (s *Server) doEnable(ctx context.Context, p authz.Principal, raw []byte, enable bool) api.Response {
	var pl api.IDPayload
	_ = json.Unmarshal(raw, &pl)
	pid, rec, err := s.resolveID(ctx, pl.ID, pl.Name, pl.Project)
	if err != nil {
		return errFrom(err)
	}
	if enable {
		rec.Desired = domain.DesiredRunning
		if err := s.store.Update(ctx, rec); err != nil {
			return errFrom(err)
		}
		_, _ = s.audit.Append(ctx, audit.Record{
			Action: "process.enable", Actor: string(p.Role), SessionID: p.Session, Target: pid,
		})
		return s.jsonOK(toView(rec))
	}
	rec.Desired = domain.DesiredStopped
	if err := s.store.Update(ctx, rec); err != nil {
		return errFrom(err)
	}
	// Stop if still running.
	if h, err := s.mgr.Inspect(ctx, pid); err == nil && h != nil && h.Status == domain.StatusRunning {
		_ = s.mgr.Stop(ctx, pid, 10*time.Second)
		rec.Status = domain.StatusExited
		now := time.Now().UTC()
		rec.ExitedAt = &now
		_ = s.store.Update(ctx, rec)
	}
	_, _ = s.audit.Append(ctx, audit.Record{
		Action: "process.disable", Actor: string(p.Role), SessionID: p.Session, Target: pid,
	})
	return s.jsonOK(toView(rec))
}

func (s *Server) doHealthCheck(ctx context.Context, p authz.Principal, raw []byte) api.Response {
	var pl api.IDPayload
	_ = json.Unmarshal(raw, &pl)
	pid, rec, err := s.resolveID(ctx, pl.ID, pl.Name, pl.Project)
	if err != nil {
		return errFrom(err)
	}
	s.mu.Lock()
	url := s.healthURL[pid]
	s.mu.Unlock()

	ok := true
	var msg string
	if url != "" {
		res := supervise.ProbeHTTP(ctx, url, 2*time.Second)
		ok = res.OK
		msg = res.Message
	} else {
		h, err := s.mgr.Inspect(ctx, pid)
		if err != nil || h == nil {
			ok = false
			msg = "process not found in manager"
		} else if h.Status != domain.StatusRunning {
			ok = false
			msg = "status " + string(h.Status)
			if h.Status != "" {
				rec.Status = h.Status
			}
		} else {
			rec.Status = domain.StatusRunning
			rec.PID = h.PID
			msg = "running"
		}
	}
	if !ok {
		rec.Status = domain.StatusUnhealthy
		rec.LastError = msg
	} else if rec.Status == domain.StatusUnhealthy {
		rec.Status = domain.StatusRunning
		rec.LastError = ""
	}
	_ = s.store.Update(ctx, rec)
	_, _ = s.audit.Append(ctx, audit.Record{
		Action: "process.health_check", Actor: string(p.Role), SessionID: p.Session, Target: pid, Detail: msg,
	})
	return s.jsonOK(api.HealthCheckResult{
		ID: pid, OK: ok, Status: string(rec.Status), Message: msg,
	})
}

func (s *Server) doGroupCreate(ctx context.Context, p authz.Principal, raw []byte) api.Response {
	var pl api.GroupPayload
	if err := json.Unmarshal(raw, &pl); err != nil {
		return errResp(domain.CodeInvalidArgument, "bad group payload", false)
	}
	proj := pl.ProjectID
	if proj == "" {
		proj = pl.Project
	}
	members := make([]group.Member, 0, len(pl.Members))
	for i, m := range pl.Members {
		members = append(members, group.Member{
			Name: m.Name, ProcessID: m.ProcessID, Order: i, DependsOn: m.DependsOn,
		})
	}
	g, err := s.groups.Create(group.Group{
		ID: pl.ID, Name: pl.Name, ProjectID: proj, Members: members,
	})
	if err != nil {
		if errors.Is(err, group.ErrExists) {
			return errResp(domain.CodeConflict, err.Error(), false)
		}
		if errors.Is(err, group.ErrCycle) {
			return errResp(domain.CodeInvalidArgument, err.Error(), false)
		}
		return errResp(domain.CodeInvalidArgument, err.Error(), false)
	}
	_, _ = s.audit.Append(ctx, audit.Record{
		Action: "group.create", Actor: string(p.Role), SessionID: p.Session, Target: g.ID, Detail: g.Name,
	})
	return s.jsonOK(groupToView(g, nil, 0, 0, ""))
}

func (s *Server) doGroupRemove(ctx context.Context, p authz.Principal, raw []byte) api.Response {
	var pl api.GroupPayload
	_ = json.Unmarshal(raw, &pl)
	gid := pl.ID
	if gid == "" {
		return errResp(domain.CodeInvalidArgument, "id required", false)
	}
	if err := s.groups.Remove(gid); err != nil {
		if errors.Is(err, group.ErrNotFound) {
			return errResp(domain.CodeNotFound, err.Error(), false)
		}
		return errFrom(err)
	}
	_, _ = s.audit.Append(ctx, audit.Record{
		Action: "group.remove", Actor: string(p.Role), SessionID: p.Session, Target: gid,
	})
	return s.jsonOK(map[string]string{"id": gid, "removed": "true"})
}

func (s *Server) doGroupList(ctx context.Context, p authz.Principal, raw []byte) api.Response {
	_ = p
	var pl api.GroupPayload
	_ = json.Unmarshal(raw, &pl)
	proj := pl.ProjectID
	if proj == "" {
		proj = pl.Project
	}
	list := s.groups.List(proj)
	out := make([]api.GroupView, 0, len(list))
	for i := range list {
		out = append(out, groupToView(&list[i], nil, 0, 0, ""))
	}
	_ = ctx
	return s.jsonOK(out)
}

func (s *Server) doGroupStatus(ctx context.Context, p authz.Principal, raw []byte) api.Response {
	_ = p
	var pl api.GroupPayload
	_ = json.Unmarshal(raw, &pl)
	g, err := s.resolveGroup(pl)
	if err != nil {
		return errFrom(err)
	}
	members := make([]api.GroupMemberView, 0, len(g.Members))
	ready, desired := 0, len(g.Members)
	for _, m := range g.Members {
		mv := api.GroupMemberView{Name: m.Name}
		_, rec, err := s.resolveID(ctx, m.ProcessID, m.Name, g.ProjectID)
		if err == nil && rec != nil {
			mv.Status = string(rec.Status)
			if rec.Status == domain.StatusRunning {
				mv.Ready = true
				ready++
			}
		} else {
			mv.Status = "unknown"
		}
		members = append(members, mv)
	}
	phase := "ready"
	switch {
	case ready == 0 && desired > 0:
		phase = "stopped"
	case ready < desired:
		phase = "degraded"
	}
	return s.jsonOK(groupToView(g, members, ready, desired, phase))
}

func (s *Server) doGroupLifecycle(ctx context.Context, p authz.Principal, raw []byte, op string) api.Response {
	var pl api.GroupPayload
	_ = json.Unmarshal(raw, &pl)
	g, err := s.resolveGroup(pl)
	if err != nil {
		return errFrom(err)
	}
	// Authorize once by operation before iterating — an empty group must still
	// be checked, and a large group must not re-check per member.
	var opCap authz.Capability
	switch op {
	case "stop":
		opCap = authz.CapProcessStop
	case "restart":
		opCap = authz.CapProcessRestart
	default:
		opCap = authz.CapProcessStart
	}
	if r := s.require(ctx, p, "group."+op, opCap); r != nil {
		return *r
	}
	var order []string
	switch op {
	case "stop":
		order, err = s.groups.StopOrder(g.ID)
	default:
		order, err = s.groups.StartOrder(g.ID)
	}
	if err != nil {
		return errFrom(err)
	}
	results := make([]map[string]string, 0, len(order))
	for _, name := range order {
		idPl := api.IDPayload{Name: name, Project: g.ProjectID}
		var resp api.Response
		switch op {
		case "stop":
			resp = s.doStop(ctx, p, mustJSON(idPl))
		case "restart":
			resp = s.doRestart(ctx, p, mustJSON(idPl))
		default: // start: restart if known (ensure running from store)
			resp = s.ensureMemberRunning(ctx, p, g.ProjectID, name)
		}
		entry := map[string]string{"name": name}
		if resp.OK {
			entry["status"] = "ok"
		} else {
			entry["status"] = "error"
			entry["error"] = resp.Error
		}
		results = append(results, entry)
	}
	_, _ = s.audit.Append(ctx, audit.Record{
		Action: "group." + op, Actor: string(p.Role), SessionID: p.Session, Target: g.ID,
	})
	return s.jsonOK(map[string]any{"id": g.ID, "op": op, "members": results})
}

func (s *Server) ensureMemberRunning(ctx context.Context, p authz.Principal, project, name string) api.Response {
	pid, rec, err := s.resolveID(ctx, "", name, project)
	if err != nil {
		return errFrom(err)
	}
	if h, err := s.mgr.Inspect(ctx, pid); err == nil && h != nil && h.Status == domain.StatusRunning {
		return s.jsonOK(api.StartResult{ID: pid, Name: rec.Name, PID: h.PID, Status: string(domain.StatusRunning), LogDir: rec.LogDir})
	}
	return s.doRestart(ctx, p, mustJSON(api.IDPayload{ID: pid}))
}

func (s *Server) resolveGroup(pl api.GroupPayload) (*group.Group, error) {
	if pl.ID != "" {
		return s.groups.Get(pl.ID)
	}
	if pl.Name == "" {
		return nil, domain.NewError(domain.CodeInvalidArgument, "group id or name required", false)
	}
	proj := pl.ProjectID
	if proj == "" {
		proj = pl.Project
	}
	for _, g := range s.groups.List(proj) {
		if g.Name == pl.Name {
			cp := g
			return &cp, nil
		}
	}
	// also scan all if project filter missed
	if proj != "" {
		for _, g := range s.groups.List("") {
			if g.Name == pl.Name {
				cp := g
				return &cp, nil
			}
		}
	}
	return nil, domain.NewError(domain.CodeNotFound, "group not found: "+pl.Name, false)
}

func groupToView(g *group.Group, members []api.GroupMemberView, ready, desired int, phase string) api.GroupView {
	if g == nil {
		return api.GroupView{}
	}
	v := api.GroupView{
		ID: g.ID, Name: g.Name, ProjectID: g.ProjectID,
		Phase: phase, Ready: ready, Desired: desired, Members: members,
	}
	if members == nil {
		v.Members = make([]api.GroupMemberView, 0, len(g.Members))
		for _, m := range g.Members {
			v.Members = append(v.Members, api.GroupMemberView{Name: m.Name})
		}
		v.Desired = len(g.Members)
	}
	return v
}

func (s *Server) doProfileList(ctx context.Context, p authz.Principal, raw []byte) api.Response {
	var pl api.ProfilePayload
	_ = json.Unmarshal(raw, &pl)
	proj := pl.ProjectID
	if proj == "" {
		proj = pl.Project
	}
	list, err := s.profiles.List(ctx, proj)
	if err != nil {
		return errFrom(err)
	}
	showValues := authz.Allow(p, authz.CapSecretsReadValues)
	out := make([]api.ProfileView, 0, len(list))
	for _, pr := range list {
		out = append(out, profileView(pr, showValues))
	}
	return s.jsonOK(out)
}

func (s *Server) doProfileGet(ctx context.Context, p authz.Principal, raw []byte) api.Response {
	var pl api.ProfilePayload
	_ = json.Unmarshal(raw, &pl)
	if pl.ID == "" {
		return errResp(domain.CodeInvalidArgument, "id required", false)
	}
	pr, err := s.profiles.Get(ctx, pl.ID)
	if err != nil {
		return errFrom(err)
	}
	return s.jsonOK(profileView(pr, authz.Allow(p, authz.CapSecretsReadValues)))
}

func (s *Server) doProfileCreate(ctx context.Context, p authz.Principal, raw []byte) api.Response {
	var pl api.ProfilePayload
	if err := json.Unmarshal(raw, &pl); err != nil {
		return errResp(domain.CodeInvalidArgument, "bad profile payload", false)
	}
	proj := pl.ProjectID
	if proj == "" {
		proj = pl.Project
	}
	pr, err := s.profiles.Create(ctx, profile.Profile{
		ID: pl.ID, Name: pl.Name, ProjectID: proj, Env: pl.Env,
	})
	if err != nil {
		return errFrom(err)
	}
	_, _ = s.audit.Append(ctx, audit.Record{
		Action: "profile.create", Actor: string(p.Role), SessionID: p.Session, Target: pr.ID, Detail: pr.Name,
	})
	return s.jsonOK(profileToView(pr))
}

func (s *Server) doProfileUpdate(ctx context.Context, p authz.Principal, raw []byte) api.Response {
	var pl api.ProfilePayload
	if err := json.Unmarshal(raw, &pl); err != nil {
		return errResp(domain.CodeInvalidArgument, "bad profile payload", false)
	}
	pr, err := s.profiles.Update(ctx, profile.Profile{
		ID: pl.ID, Name: pl.Name, Env: pl.Env,
	})
	if err != nil {
		return errFrom(err)
	}
	_, _ = s.audit.Append(ctx, audit.Record{
		Action: "profile.update", Actor: string(p.Role), SessionID: p.Session, Target: pr.ID,
	})
	return s.jsonOK(profileToView(pr))
}

func (s *Server) doProfileDelete(ctx context.Context, p authz.Principal, raw []byte) api.Response {
	var pl api.ProfilePayload
	_ = json.Unmarshal(raw, &pl)
	if pl.ID == "" {
		return errResp(domain.CodeInvalidArgument, "id required", false)
	}
	if err := s.profiles.Delete(ctx, pl.ID); err != nil {
		return errFrom(err)
	}
	_, _ = s.audit.Append(ctx, audit.Record{
		Action: "profile.delete", Actor: string(p.Role), SessionID: p.Session, Target: pl.ID,
	})
	return s.jsonOK(map[string]string{"id": pl.ID, "removed": "true"})
}

func (s *Server) doProfileUse(ctx context.Context, p authz.Principal, req api.Request) api.Response {
	var pl api.ProfilePayload
	_ = json.Unmarshal(req.Payload, &pl)
	sess := pl.Session
	if sess == "" {
		sess = req.Session
	}
	if sess == "" {
		// open a session so Use has a key
		opened, err := s.sessions.Open("", string(p.Role))
		if err != nil {
			return errFrom(err)
		}
		sess = opened.ID
	}
	if err := s.profiles.Use(ctx, sess, pl.Name); err != nil {
		return errFrom(err)
	}
	return s.jsonOK(map[string]string{"session": sess, "profile": s.profiles.Active(sess)})
}

func profileToView(p profile.Profile) api.ProfileView {
	return api.ProfileView{ID: p.ID, Name: p.Name, ProjectID: p.ProjectID, Env: p.Env}
}

// profileView renders a profile for read paths, redacting env values (keeping
// keys) unless the caller may read secret values (secrets:read_values). Profiles
// are the natural place users park API keys, so list/get must not leak values to
// lower roles (capability matrix; mirrors ProcessView's env-keys-only rule).
func profileView(pr profile.Profile, showValues bool) api.ProfileView {
	v := api.ProfileView{ID: pr.ID, Name: pr.Name, ProjectID: pr.ProjectID}
	if len(pr.Env) == 0 {
		return v
	}
	env := make(map[string]string, len(pr.Env))
	for k, val := range pr.Env {
		if showValues {
			env[k] = val
		} else {
			env[k] = "[redacted]"
		}
	}
	v.Env = env
	return v
}

func (s *Server) ensureSession(req api.Request, role authz.Role) (*session.Session, error) {
	if req.Session != "" {
		if sess, ok := s.sessions.Get(req.Session); ok {
			return sess, nil
		}
		// treat request session as harness id
		return s.sessions.Open(req.Session, string(role))
	}
	return s.sessions.Open("", string(role))
}

func (s *Server) doSessionInfo(ctx context.Context, p authz.Principal, req api.Request) api.Response {
	_ = ctx
	sess, err := s.ensureSession(req, p.Role)
	if err != nil {
		return errFrom(err)
	}
	return s.jsonOK(api.SessionInfoResult{
		ID: sess.ID, HarnessID: sess.HarnessID, Role: sess.Role,
		CreatedAt: sess.CreatedAt, EndedAt: sess.EndedAt,
	})
}

func (s *Server) doSessionEnd(ctx context.Context, p authz.Principal, req api.Request) api.Response {
	sid := req.Session
	if sid == "" {
		var pl struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(req.Payload, &pl)
		sid = pl.ID
	}
	if sid == "" {
		return errResp(domain.CodeInvalidArgument, "session required", false)
	}
	// Capture harness/request id before resolving to internal ULID.
	sessionKeys := []string{sid}
	if p.Session != "" && p.Session != sid {
		sessionKeys = append(sessionKeys, p.Session)
	}
	// resolve harness id to internal if needed
	if sess, ok := s.sessions.Get(sid); ok {
		sessionKeys = append(sessionKeys, sess.ID, sess.HarnessID)
		sid = sess.ID
	}
	// Registry has no list-by-harness; if sid is still a harness id, End fails
	// and we fall through to ensureSession below.
	if !s.sessions.End(sid) {
		// try ensure then end when session was harness-only
		opened, err := s.ensureSession(req, p.Role)
		if err != nil {
			return errResp(domain.CodeNotFound, "session not found", false)
		}
		sessionKeys = append(sessionKeys, opened.ID, opened.HarnessID)
		sid = opened.ID
		_ = s.sessions.End(sid)
	}
	// Stopping this session's stop-on-disconnect processes is a de-facto
	// process:stop and requires that capability beyond session:end (which alone
	// only permits self-management). Since the session id is client-asserted, we
	// cannot trust "own vs other" — we gate on whether real processes would be
	// stopped..
	if len(s.sodIDsForSession(sessionKeys)) > 0 {
		if r := s.require(ctx, p, "session.end", authz.CapProcessStop); r != nil {
			return *r
		}
	}
	// stop_on_disconnect: stop processes started with that flag for this session.
	stopped := s.stopSODForSession(ctx, sessionKeys)
	// Release the session's active-profile entry so sessionUse does not grow
	// unbounded (security group's RemoveSession).
	s.profiles.RemoveSession(sid)
	_, _ = s.audit.Append(ctx, audit.Record{
		Action: "session.end", Actor: string(p.Role), SessionID: sid,
		Detail: fmt.Sprintf("stopped_sod=%d", stopped),
	})
	return s.jsonOK(map[string]any{"id": sid, "ended": "true", "stopped": stopped})
}

// sodIDsForSession returns the process ids registered with stop_on_disconnect
// for any of the given session key aliases (harness id, internal id, request
// session). Used both to authorize and to perform session-end teardown.
func (s *Server) sodIDsForSession(sessionKeys []string) []string {
	keySet := make(map[string]struct{}, len(sessionKeys))
	for _, k := range sessionKeys {
		k = strings.TrimSpace(k)
		if k != "" {
			keySet[k] = struct{}{}
		}
	}
	if len(keySet) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var ids []string
	for pid, sess := range s.stopOnDisconnect {
		if _, ok := keySet[sess]; ok {
			ids = append(ids, pid)
		}
	}
	return ids
}

// stopSODForSession stops processes registered with stop_on_disconnect for any
// of the given session key aliases (harness id, internal id, request session).
func (s *Server) stopSODForSession(ctx context.Context, sessionKeys []string) int {
	ids := s.sodIDsForSession(sessionKeys)
	n := 0
	for _, pid := range ids {
		if err := s.mgr.Stop(ctx, pid, 5*time.Second); err != nil {
			continue
		}
		if rec, err := s.store.Get(ctx, pid); err == nil && rec != nil {
			rec.Status = domain.StatusExited
			rec.Desired = domain.DesiredStopped
			now := time.Now().UTC()
			rec.ExitedAt = &now
			_ = s.store.Update(ctx, rec)
		}
		s.mu.Lock()
		delete(s.stopOnDisconnect, pid)
		s.mu.Unlock()
		_, _ = s.events.Append(ctx, event.Event{
			Type: "process.stopped", ProcessID: pid, Message: "stop_on_disconnect",
		})
		n++
	}
	return n
}

func (s *Server) doShare(ctx context.Context, p authz.Principal, raw []byte, grant bool) api.Response {
	var pl api.SharePayload
	if err := json.Unmarshal(raw, &pl); err != nil {
		return errResp(domain.CodeInvalidArgument, "bad share payload", false)
	}
	if pl.Target == "" || pl.ToSession == "" {
		return errResp(domain.CodeInvalidArgument, "target and to_session required", false)
	}
	capName := authz.Capability(pl.Cap)
	if grant {
		s.shares.Share(authz.Grant{Target: pl.Target, Cap: capName, ToSession: pl.ToSession})
		_, _ = s.audit.Append(ctx, audit.Record{
			Action: "session.share", Actor: string(p.Role), SessionID: p.Session,
			Target: pl.Target, Detail: pl.ToSession,
		})
		return s.jsonOK(map[string]string{"shared": "true", "target": pl.Target, "to_session": pl.ToSession})
	}
	n := s.shares.Unshare(pl.Target, pl.ToSession, capName)
	_, _ = s.audit.Append(ctx, audit.Record{
		Action: "session.unshare", Actor: string(p.Role), SessionID: p.Session,
		Target: pl.Target, Detail: pl.ToSession,
	})
	return s.jsonOK(map[string]any{"removed": n, "target": pl.Target, "to_session": pl.ToSession})
}

func (s *Server) loadDeclare(pl api.DeclarePayload) (*declare.Document, error) {
	data := []byte(pl.YAML)
	if len(data) == 0 && pl.Data != "" {
		data = []byte(pl.Data)
	}
	if len(data) == 0 && pl.Path != "" {
		b, err := os.ReadFile(pl.Path)
		if err != nil {
			return nil, domain.WrapError(domain.CodeInvalidArgument, "read declare path", false, err)
		}
		data = b
	}
	if len(data) == 0 {
		return nil, domain.NewError(domain.CodeInvalidArgument, "yaml, data, or path required", false)
	}
	doc, err := declare.Parse(data)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func (s *Server) doDeclareValidate(ctx context.Context, p authz.Principal, raw []byte) api.Response {
	_ = ctx
	_ = p
	var pl api.DeclarePayload
	_ = json.Unmarshal(raw, &pl)
	doc, err := s.loadDeclare(pl)
	if err != nil {
		return errFrom(err)
	}
	if err := doc.Validate(); err != nil {
		return errFrom(err)
	}
	return s.jsonOK(map[string]any{"valid": true, "services": len(doc.Services)})
}

func (s *Server) doDeclareDiff(ctx context.Context, p authz.Principal, raw []byte) api.Response {
	_ = p
	var pl api.DeclarePayload
	_ = json.Unmarshal(raw, &pl)
	doc, err := s.loadDeclare(pl)
	if err != nil {
		return errFrom(err)
	}
	running := pl.RunningNames
	if running == nil {
		list, err := s.store.List(ctx, store.ProcessFilter{})
		if err != nil {
			return errFrom(err)
		}
		running = make([]string, 0, len(list))
		for _, rec := range list {
			running = append(running, rec.Name)
		}
	}
	diff := declare.DiffServices(doc, running)
	return s.jsonOK(diff)
}

func (s *Server) doDeclareApply(ctx context.Context, p authz.Principal, raw []byte) api.Response {
	var pl api.DeclarePayload
	_ = json.Unmarshal(raw, &pl)
	doc, err := s.loadDeclare(pl)
	if err != nil {
		return errFrom(err)
	}
	if err := doc.Validate(); err != nil {
		return errFrom(err)
	}
	list, err := s.store.List(ctx, store.ProcessFilter{})
	if err != nil {
		return errFrom(err)
	}
	running := make([]string, 0, len(list))
	for _, rec := range list {
		running = append(running, rec.Name)
	}
	diff := declare.DiffServices(doc, running)
	created := make([]string, 0)
	for _, d := range diff {
		if d.Action != "create" {
			continue
		}
		spec, ok := doc.Services[d.Name]
		if !ok {
			continue
		}
		argv := spec.Argv
		if len(argv) == 0 {
			continue
		}
		//: an unset sandbox must fall through to doStart, which applies
		// cfg.Sandbox.Default (shipped strict) and gates relaxation on
		// CapSandboxRelax. Never substitute "off" here — that is the forbidden
		// silent unsandboxed start.
		startPl := api.StartPayload{
			Name: d.Name, Command: argv, Sandbox: spec.Sandbox,
			Runtime: spec.Runtime, Image: spec.Image,
		}
		resp := s.doStart(ctx, p, mustJSON(startPl))
		if !resp.OK {
			return resp
		}
		created = append(created, d.Name)
	}
	_, _ = s.audit.Append(ctx, audit.Record{
		Action: "declare.apply", Actor: string(p.Role), SessionID: p.Session,
		Detail: fmt.Sprintf("created=%d", len(created)),
	})
	_, _ = s.events.Append(ctx, event.Event{Type: "declare.applied", SessionID: p.Session, Message: strings.Join(created, ",")})
	return s.jsonOK(map[string]any{"created": created, "diff": diff})
}

func (s *Server) doDeclareShow(ctx context.Context, p authz.Principal, raw []byte) api.Response {
	_ = ctx
	_ = p
	var pl api.DeclarePayload
	_ = json.Unmarshal(raw, &pl)
	doc, err := s.loadDeclare(pl)
	if err != nil {
		return errFrom(err)
	}
	return s.jsonOK(doc)
}

func (s *Server) doDeclareImport(ctx context.Context, p authz.Principal, raw []byte) api.Response {
	_ = ctx
	_ = p
	var pl api.DeclarePayload
	_ = json.Unmarshal(raw, &pl)
	data := []byte(pl.Data)
	if len(data) == 0 && pl.Path != "" {
		b, err := os.ReadFile(pl.Path)
		if err != nil {
			return errFrom(err)
		}
		data = b
	}
	if len(data) == 0 {
		return errResp(domain.CodeInvalidArgument, "data or path required", false)
	}
	format := strings.ToLower(pl.Format)
	if format == "" || format == "procfile" {
		doc, err := declare.ImportProcfile(data)
		if err != nil {
			return errFrom(err)
		}
		return s.jsonOK(doc)
	}
	return errResp(domain.CodeInvalidArgument, "unsupported import format: "+pl.Format, false)
}

func (s *Server) doPorts(ctx context.Context, raw []byte) api.Response {
	var pl api.IDPayload
	_ = json.Unmarshal(raw, &pl)
	pid, rec, err := s.resolveID(ctx, pl.ID, pl.Name, pl.Project)
	if err != nil {
		return errFrom(err)
	}
	s.mu.Lock()
	declared := append([]string(nil), s.ports[pid]...)
	s.mu.Unlock()
	if declared == nil {
		declared = []string{}
	}
	osPID := 0
	if rec != nil {
		osPID = rec.PID
	}
	if h, err := s.mgr.Inspect(ctx, pid); err == nil && h != nil && h.PID > 0 {
		osPID = h.PID
	}
	discovered := portsd.DiscoverListeningPorts(osPID)
	if discovered == nil {
		discovered = []string{}
	}
	return s.jsonOK(api.PortsResult{ID: pid, Ports: declared, Discovered: discovered})
}

func (s *Server) doRuntimeInfo(ctx context.Context, p authz.Principal) api.Response {
	_ = p
	engines := map[string]bool{
		"podman": podman.New().Available(ctx),
		"docker": docker.New().Available(ctx),
	}
	return s.jsonOK(api.RuntimeInfoResult{Local: true, Engines: engines})
}

func (s *Server) doSandboxProfiles(ctx context.Context, p authz.Principal) api.Response {
	_ = ctx
	_ = p
	return s.jsonOK(api.SandboxProfilesResult{
		Profiles: []string{
			string(sandbox.Strict),
			string(sandbox.Standard),
			string(sandbox.Permissive),
			string(sandbox.Off),
		},
	})
}

func (s *Server) doSecretList(ctx context.Context, p authz.Principal) api.Response {
	_ = ctx
	_ = p
	s.mu.Lock()
	out := make([]api.SecretRefView, 0, len(s.secrets))
	for name, path := range s.secrets {
		out = append(out, api.SecretRefView{Name: name, Path: path})
	}
	s.mu.Unlock()
	return s.jsonOK(api.SecretListResult{Refs: out})
}

func (s *Server) doSecretRefCheck(ctx context.Context, p authz.Principal, raw []byte) api.Response {
	_ = ctx
	_ = p
	var pl api.SecretPayload
	_ = json.Unmarshal(raw, &pl)
	// secret:// URI form: check resolves without returning secret bytes.
	ref := pl.Ref
	if ref == "" {
		ref = pl.Name
	}
	if secret.LooksLikeRef(ref) {
		ok, errMsg := secret.Check(ref, secret.ResolveOptions{
			ProjectRoot: pl.Path, // optional project root
			Keyring:     s.keyring,
		})
		return s.jsonOK(map[string]any{
			"ref": ref, "ok": ok, "error": errMsg,
		})
	}
	if pl.Name == "" {
		return errResp(domain.CodeInvalidArgument, "name or ref required", false)
	}
	s.mu.Lock()
	path, ok := s.secrets[pl.Name]
	s.mu.Unlock()
	if !ok {
		// Fall back to keyring-by-name check without returning value.
		if s.keyring != nil {
			if _, err := s.keyring.Get(pl.Name); err == nil {
				return s.jsonOK(map[string]any{"name": pl.Name, "ok": true})
			}
		}
		return s.jsonOK(map[string]any{"name": pl.Name, "ok": false, "error": "ref not found"})
	}
	if _, err := os.Stat(path); err != nil {
		return s.jsonOK(map[string]any{"name": pl.Name, "path": path, "ok": false, "error": errString(err)})
	}
	// SOPS-encrypted refs: attempt decrypt without returning cleartext.
	if secret.LooksLikeSOPS(path) {
		if _, err := secret.DecryptFile(path); err != nil {
			return s.jsonOK(map[string]any{
				"name": pl.Name, "path": path, "ok": false,
				"error": errString(err), "sops": true,
			})
		}
		return s.jsonOK(map[string]any{"name": pl.Name, "path": path, "ok": true, "sops": true})
	}
	return s.jsonOK(map[string]any{"name": pl.Name, "path": path, "ok": true})
}

func (s *Server) doSecretSet(ctx context.Context, p authz.Principal, raw []byte) api.Response {
	var pl api.SecretPayload
	if err := json.Unmarshal(raw, &pl); err != nil {
		return errResp(domain.CodeInvalidArgument, "bad secret payload", false)
	}
	if pl.Name == "" {
		return errResp(domain.CodeInvalidArgument, "name required", false)
	}
	// Prefer storing value in file keyring when Value provided; else register path ref.
	path := pl.Path
	if pl.Value != "" && s.keyring != nil {
		p2, err := s.keyring.Set(pl.Name, pl.Value)
		if err != nil {
			return errFrom(err)
		}
		path = p2
	}
	if path == "" {
		return errResp(domain.CodeInvalidArgument, "path or value required", false)
	}
	s.mu.Lock()
	s.secrets[pl.Name] = path
	s.mu.Unlock()
	_, _ = s.audit.Append(ctx, audit.Record{
		Action: "secret.set", Actor: string(p.Role), SessionID: p.Session, Target: pl.Name, Detail: path,
	})
	return s.jsonOK(api.SecretRefView{Name: pl.Name, Path: path})
}

func (s *Server) doWatchSet(ctx context.Context, p authz.Principal, raw []byte) api.Response {
	var pl api.WatchPayload
	_ = json.Unmarshal(raw, &pl)
	if pl.Path == "" {
		return errResp(domain.CodeInvalidArgument, "path required", false)
	}
	pid, _, err := s.resolveID(ctx, pl.ID, pl.Name, pl.Project)
	if err != nil {
		return errFrom(err)
	}
	// Use daemon lifetime context — not the per-RPC ctx (which ends when Call returns).
	if err := s.startWatchForProcess(s.daemonCtx(ctx), pid, pl.Path); err != nil {
		return errFrom(err)
	}
	_, _ = s.audit.Append(ctx, audit.Record{
		Action: "watch.set", Actor: string(p.Role), SessionID: p.Session, Target: pid, Detail: pl.Path,
	})
	return s.jsonOK(api.WatchView{ProcessID: pid, Path: pl.Path})
}

func (s *Server) doWatchStatus(ctx context.Context, p authz.Principal) api.Response {
	_ = ctx
	_ = p
	s.mu.Lock()
	out := make([]api.WatchView, 0, len(s.watches))
	for id, path := range s.watches {
		out = append(out, api.WatchView{ProcessID: id, Path: path})
	}
	s.mu.Unlock()
	return s.jsonOK(api.WatchStatusResult{Watches: out})
}

func (s *Server) doWebhookCreate(ctx context.Context, p authz.Principal, raw []byte) api.Response {
	var pl api.WebhookPayload
	if err := json.Unmarshal(raw, &pl); err != nil {
		return errResp(domain.CodeInvalidArgument, "bad webhook payload", false)
	}
	if pl.URL == "" {
		return errResp(domain.CodeInvalidArgument, "url required", false)
	}
	hid := pl.ID
	if hid == "" {
		// wh- prefix is product vocabulary; reuse ULID body from session generator.
		tmp, err := id.New(id.Event)
		if err != nil {
			return errFrom(err)
		}
		hid = "wh-" + strings.TrimPrefix(tmp, "evt-")
	}
	h := webhook.Hook{ID: hid, URL: pl.URL, Events: pl.Events}
	if err := s.hooks.Create(h); err != nil {
		if errors.Is(err, webhook.ErrSSRF) || errors.Is(err, webhook.ErrInvalid) {
			return errResp(domain.CodeInvalidArgument, err.Error(), false)
		}
		return errFrom(err)
	}
	_, _ = s.audit.Append(ctx, audit.Record{
		Action: "webhook.create", Actor: string(p.Role), SessionID: p.Session, Target: hid,
	})
	return s.jsonOK(api.WebhookView{ID: h.ID, URL: h.URL, Events: h.Events})
}

func (s *Server) doWebhookUpdate(ctx context.Context, p authz.Principal, raw []byte) api.Response {
	var pl api.WebhookPayload
	if err := json.Unmarshal(raw, &pl); err != nil {
		return errResp(domain.CodeInvalidArgument, "bad webhook payload", false)
	}
	if pl.ID == "" {
		return errResp(domain.CodeInvalidArgument, "id required", false)
	}
	cur, err := s.hooks.Get(pl.ID)
	if err != nil {
		if errors.Is(err, webhook.ErrNotFound) {
			return errResp(domain.CodeNotFound, err.Error(), false)
		}
		return errFrom(err)
	}
	if pl.URL != "" {
		cur.URL = pl.URL
	}
	if pl.Events != nil {
		cur.Events = pl.Events
	}
	if err := s.hooks.Create(cur); err != nil {
		if errors.Is(err, webhook.ErrSSRF) || errors.Is(err, webhook.ErrInvalid) {
			return errResp(domain.CodeInvalidArgument, err.Error(), false)
		}
		return errFrom(err)
	}
	_, _ = s.audit.Append(ctx, audit.Record{
		Action: "webhook.update", Actor: string(p.Role), SessionID: p.Session, Target: cur.ID,
	})
	return s.jsonOK(api.WebhookView{ID: cur.ID, URL: cur.URL, Events: cur.Events})
}

func (s *Server) doWebhookDelete(ctx context.Context, p authz.Principal, raw []byte) api.Response {
	var pl api.WebhookPayload
	_ = json.Unmarshal(raw, &pl)
	if pl.ID == "" {
		return errResp(domain.CodeInvalidArgument, "id required", false)
	}
	if err := s.hooks.Delete(pl.ID); err != nil {
		if errors.Is(err, webhook.ErrNotFound) {
			return errResp(domain.CodeNotFound, err.Error(), false)
		}
		return errFrom(err)
	}
	_, _ = s.audit.Append(ctx, audit.Record{
		Action: "webhook.delete", Actor: string(p.Role), SessionID: p.Session, Target: pl.ID,
	})
	return s.jsonOK(map[string]string{"id": pl.ID, "removed": "true"})
}

func (s *Server) doWebhookList(ctx context.Context, p authz.Principal) api.Response {
	_ = ctx
	_ = p
	list := s.hooks.List()
	out := make([]api.WebhookView, 0, len(list))
	for _, h := range list {
		out = append(out, api.WebhookView{ID: h.ID, URL: h.URL, Events: h.Events})
	}
	return s.jsonOK(out)
}

func (s *Server) doWebhookTest(ctx context.Context, p authz.Principal, raw []byte) api.Response {
	var pl api.WebhookPayload
	_ = json.Unmarshal(raw, &pl)
	if pl.ID == "" {
		return errResp(domain.CodeInvalidArgument, "id required", false)
	}
	h, err := s.hooks.Get(pl.ID)
	if err != nil {
		if errors.Is(err, webhook.ErrNotFound) {
			return errResp(domain.CodeNotFound, err.Error(), false)
		}
		return errFrom(err)
	}
	payload := map[string]any{
		"type": "webhook.test",
		"at":   time.Now().UTC(),
	}
	if err := webhook.Deliver(ctx, h, payload); err != nil {
		return errResp(domain.CodeInternal, err.Error(), true)
	}
	_, _ = s.audit.Append(ctx, audit.Record{
		Action: "webhook.test", Actor: string(p.Role), SessionID: p.Session, Target: h.ID,
	})
	return s.jsonOK(map[string]string{"id": h.ID, "delivered": "true"})
}

func (s *Server) doMetrics(ctx context.Context) api.Response {
	list, err := s.store.List(ctx, store.ProcessFilter{})
	if err != nil {
		return errFrom(err)
	}
	// Use start-time-verified sampling so a recycled PID zeroes out instead of
	// misattributing another process's /proc counters (telemetry PID-reuse fix).
	refs := make(map[string]observability.ProcRef, len(list))
	for _, rec := range list {
		pid := rec.PID
		if h, err := s.mgr.Inspect(ctx, rec.ID); err == nil && h != nil {
			pid = h.PID
		}
		s.mu.Lock()
		st := s.startTimes[rec.ID]
		s.mu.Unlock()
		refs[rec.ID] = observability.ProcRef{PID: pid, StartTime: st}
	}
	snap := observability.SnapshotVerified(refs)
	return s.jsonOK(snap)
}

func (s *Server) doLogsExport(ctx context.Context, p authz.Principal, raw []byte) api.Response {
	var pl api.LogsPayload
	_ = json.Unmarshal(raw, &pl)
	pid, rec, err := s.resolveID(ctx, pl.ID, pl.Name, pl.Project)
	if err != nil {
		return errFrom(err)
	}
	exportDir := filepath.Join(s.cfg.StateDir, "exports")
	if err := os.MkdirAll(exportDir, 0o700); err != nil {
		return errFrom(err)
	}
	outPath := filepath.Join(exportDir, pid+"-logs.tar.gz")
	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return errFrom(err)
	}
	err = logcap.ExportTarGz(rec.LogDir, f)
	_ = f.Close()
	if err != nil {
		return errFrom(err)
	}
	_, _ = s.audit.Append(ctx, audit.Record{
		Action: "logs.export", Actor: string(p.Role), SessionID: p.Session, Target: pid, Detail: outPath,
	})
	return s.jsonOK(api.LogsExportResult{Path: outPath})
}

func (s *Server) doLogsShip(ctx context.Context, p authz.Principal, raw []byte) api.Response {
	var pl api.LogsShipPayload
	if err := json.Unmarshal(raw, &pl); err != nil {
		return errResp(domain.CodeInvalidArgument, "bad ship payload", false)
	}
	sink := pl.SinkPath
	if sink == "" {
		sink = pl.Path
	}
	if pl.ExportPath == "" || sink == "" {
		return errResp(domain.CodeInvalidArgument, "export_path and sink_path required", false)
	}
	src, err := os.Open(pl.ExportPath)
	if err != nil {
		return errFrom(err)
	}
	defer func() { _ = src.Close() }()
	if err := os.MkdirAll(filepath.Dir(sink), 0o700); err != nil {
		return errFrom(err)
	}
	// Create-new only (O_EXCL): never truncate a pre-existing file. Combined
	// with the operator+ logs:export gate this closes the arbitrary-overwrite
	// hole (the catalog's export path-jail intent) without a config root.
	dst, err := os.OpenFile(sink, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return errResp(domain.CodeInvalidArgument, "sink path already exists: "+sink, false)
		}
		return errFrom(err)
	}
	_, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil {
		return errFrom(copyErr)
	}
	if closeErr != nil {
		return errFrom(closeErr)
	}
	_, _ = s.audit.Append(ctx, audit.Record{
		Action: "logs.ship", Actor: string(p.Role), SessionID: p.Session, Detail: sink,
	})
	return s.jsonOK(map[string]string{"path": sink, "shipped": "true"})
}

func (s *Server) doSubscribe(ctx context.Context, kind string, raw []byte) api.Response {
	var pl api.SubPayload
	_ = json.Unmarshal(raw, &pl)
	sid, err := id.New(id.Event)
	if err != nil {
		return errFrom(err)
	}
	// use sub- prefix vocabulary via rewrite
	sid = "sub-" + strings.TrimPrefix(sid, "evt-")
	info := subInfo{
		ID: sid, Kind: kind, ProcessID: pl.ProcessID, CreatedAt: time.Now().UTC(),
	}
	s.mu.Lock()
	s.subs[sid] = info
	s.mu.Unlock()

	// Product path for logs.subscribe: short follow of log growth (backpressure via timeout).
	followText := ""
	if kind == "logs" && pl.ProcessID != "" {
		if rec, err := s.store.Get(ctx, pl.ProcessID); err == nil && rec.LogDir != "" {
			followText = followLogDir(ctx, rec.LogDir, s.logsPreviewFollow)
		}
	}
	if kind == "events" {
		evs := s.events.Query(ctx, pl.ProcessID, 20)
		b, _ := json.Marshal(evs)
		followText = string(b)
	}
	return s.jsonOK(map[string]any{
		"id":      sid,
		"kind":    kind,
		"preview": followText,
		"note":    "subscription registered; preview is a short follow window (poll logs/events for more)",
	})
}

func (s *Server) doUnsubscribe(ctx context.Context, raw []byte) api.Response {
	_ = ctx
	var pl api.SubPayload
	_ = json.Unmarshal(raw, &pl)
	if pl.ID == "" {
		return errResp(domain.CodeInvalidArgument, "id required", false)
	}
	s.mu.Lock()
	_, ok := s.subs[pl.ID]
	delete(s.subs, pl.ID)
	s.mu.Unlock()
	if !ok {
		return errResp(domain.CodeNotFound, "subscription not found", false)
	}
	return s.jsonOK(map[string]string{"id": pl.ID, "removed": "true"})
}

func (s *Server) doListSubs(ctx context.Context) api.Response {
	_ = ctx
	s.mu.Lock()
	out := make([]subInfo, 0, len(s.subs))
	for _, info := range s.subs {
		out = append(out, info)
	}
	s.mu.Unlock()
	return s.jsonOK(out)
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
