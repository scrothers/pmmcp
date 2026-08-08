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

package daemoncmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/scrothers/pmmcp/internal/config"
	"github.com/scrothers/pmmcp/internal/daemon"
	"github.com/scrothers/pmmcp/internal/version"
)

// Execute builds the root command and runs it with ctx as the command context, so
// signal cancellation from main propagates into the daemon run loop.
func Execute(ctx context.Context) error {
	return newRootCmd().ExecuteContext(ctx)
}

// newRootCmd assembles the pmmcpd command tree. The root itself runs the daemon so
// bare `pmmcpd` behaves as before; `run` is the explicit form and `version` prints
// the build version. Errors and usage are silenced so main owns error reporting.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "pmmcpd",
		Short:         "pmmcp process supervisor daemon",
		Long:          "pmmcpd is the long-lived pmmcp process supervisor daemon.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE:          runDaemon,
	}
	config.RegisterDaemonFlags(root.PersistentFlags())

	runCmd := &cobra.Command{
		Use:           "run",
		Short:         "Run the daemon (default when no subcommand is given)",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE:          runDaemon,
	}
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the pmmcpd version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "pmmcpd %s\n", version.String())
			return nil
		},
	}
	root.AddCommand(runCmd, versionCmd)
	return root
}

// runDaemon loads config (flag > env > file > default) and serves until the
// command context is cancelled.
func runDaemon(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	fs := cmd.Root().PersistentFlags()
	cfgPath, _ := fs.GetString("config")
	if cfgPath == "" {
		cfgPath = os.Getenv("PMMCP_CONFIG")
	}
	cfg, err := config.Load(config.LoadOptions{Path: cfgPath, Flags: fs})
	if err != nil {
		return err
	}
	srv, err := daemon.New(ctx, daemon.Options{Config: cfg})
	if err != nil {
		return err
	}
	defer func() { _ = srv.Close() }()
	fmt.Fprintf(os.Stderr, "pmmcpd listening on %s (state %s)\n", cfg.IPC.Endpoint, cfg.StateDir)
	return srv.ListenAndServe(ctx)
}
