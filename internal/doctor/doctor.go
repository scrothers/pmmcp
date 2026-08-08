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

package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"runtime"
	"strings"
	"time"

	"github.com/scrothers/pmmcp/internal/api"
	pmmcpv1 "github.com/scrothers/pmmcp/internal/api/gen/pmmcp/v1"
	"github.com/scrothers/pmmcp/internal/ipc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Report is the result of a local connectivity/config diagnosis.
type Report struct {
	OK    bool
	Lines []string
}

// Check probes the daemon endpoint without starting the daemon.
// It dials via ipc.Dial (unix socket path) or gRPC over TCP when the endpoint
// looks like a host:port address, and only reports OK after a successful Hello
// handshake — so a stray listener squatting the endpoint is not mistaken for a
// healthy daemon. On failure it appends remediation lines.
func Check(ctx context.Context, endpoint string) Report {
	r := Report{OK: false}
	if endpoint == "" {
		r.Lines = append(r.Lines, "endpoint: empty")
		r.Lines = append(r.Lines, remediation()...)
		return r
	}
	r.Lines = append(r.Lines, fmt.Sprintf("endpoint: %s", endpoint))

	if isTCPEndpoint(endpoint) {
		return checkTCP(ctx, endpoint, r)
	}
	return checkUnix(ctx, endpoint, r)
}

func checkUnix(ctx context.Context, endpoint string, r Report) Report {
	c, err := ipc.Dial(ctx, endpoint)
	if err != nil {
		r.Lines = append(r.Lines, fmt.Sprintf("dial: %v", err))
		r.Lines = append(r.Lines, remediation()...)
		return r
	}
	defer func() { _ = c.Close() }()

	h, err := c.Hello(ctx)
	if err != nil {
		r.Lines = append(r.Lines, fmt.Sprintf("hello: %v", err))
		r.Lines = append(r.Lines, remediation()...)
		return r
	}
	r.OK = true
	r.Lines = append(r.Lines, fmt.Sprintf("daemon: ok api=%s version=%s", h.APIVersion, h.DaemonVersion))
	return r
}

func checkTCP(ctx context.Context, endpoint string, r Report) Report {
	h, err := tcpHello(ctx, endpoint)
	if err != nil {
		r.Lines = append(r.Lines, fmt.Sprintf("hello (tcp): %v", err))
		r.Lines = append(r.Lines, remediation()...)
		return r
	}
	if skew := versionSkew(h.APIVersion); skew != "" {
		r.Lines = append(r.Lines, fmt.Sprintf("dial: ok (tcp) but %s", skew))
		r.Lines = append(r.Lines, remediation()...)
		return r
	}
	r.OK = true
	r.Lines = append(r.Lines, fmt.Sprintf("daemon: ok (tcp) api=%s version=%s", h.APIVersion, h.DaemonVersion))
	return r
}

// tcpHello performs the gRPC Hello handshake over a TCP endpoint. ipc.Dial only
// speaks the unix transport, so the TCP path builds its own client here.
func tcpHello(ctx context.Context, endpoint string) (*api.HelloResult, error) {
	conn, err := grpc.NewClient("passthrough:///"+endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "tcp", endpoint)
		}),
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()

	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	resp, err := pmmcpv1.NewDaemonClient(conn).Call(cctx, &pmmcpv1.CallRequest{
		ApiVersion: api.APIVersion,
		Method:     api.MethodHello,
	})
	if err != nil {
		return nil, err
	}
	if !resp.GetOk() {
		return nil, fmt.Errorf("daemon refused hello: %s", resp.GetError())
	}
	var out api.HelloResult
	if len(resp.GetPayload()) > 0 {
		if err := json.Unmarshal(resp.GetPayload(), &out); err != nil {
			return nil, fmt.Errorf("decode hello: %w", err)
		}
	}
	return &out, nil
}

// versionSkew returns a description when the daemon's major API version differs
// from the client's, or "" when compatible.
func versionSkew(daemonAPI string) string {
	if daemonAPI == "" {
		return ""
	}
	if daemonAPI[:1] != api.APIVersion[:1] {
		return fmt.Sprintf("api version mismatch (client %s, daemon %s)", api.APIVersion, daemonAPI)
	}
	return ""
}

func remediation() []string {
	return remediationFor(runtime.GOOS)
}

// remediationFor implements remediation for an explicit goos value. It is the
// seam tests use to exercise every OS-specific hint (including the
// linux/other-OS default) regardless of which platform runs the test binary;
// remediation always calls it with runtime.GOOS.
func remediationFor(goos string) []string {
	lines := []string{"remediation: start pmmcpd (pmmcpd run) or pmmcp install-service"}
	switch goos {
	case "darwin":
		lines = append(lines, "remediation: load the LaunchAgent (launchctl load ~/Library/LaunchAgents/com.scrothers.pmmcpd.plist)")
	case "windows":
		lines = append(lines, `remediation: register the logon task (see %LOCALAPPDATA%\pmmcp\INSTALL.txt)`)
	default:
		lines = append(lines, "remediation: ensure the user service is enabled (systemctl --user enable --now pmmcpd.service)")
	}
	return lines
}

// isTCPEndpoint reports whether endpoint looks like host:port (not a path).
func isTCPEndpoint(endpoint string) bool {
	if strings.Contains(endpoint, "/") || strings.HasPrefix(endpoint, `\\`) {
		return false
	}
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		return false
	}
	return host != "" && port != ""
}
