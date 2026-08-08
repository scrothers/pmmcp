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

package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/scrothers/pmmcp/internal/api"
	"github.com/scrothers/pmmcp/internal/doctor"
	"github.com/scrothers/pmmcp/internal/domain"
)

// adminCommands builds daemon/system administration and utility commands.
func (st *rootState) adminCommands() []*cobra.Command {
	return []*cobra.Command{
		st.versionCmd(),
		st.newDoctorCmd(),
		st.jsonCmd("metrics", "Show metrics snapshot", api.MethodMetrics),
		st.jsonCmd("sandbox-profiles", "List sandbox profiles", api.MethodSandboxProfiles),
		st.jsonCmd("whoami", "Show caller identity", api.MethodWhoami),
		st.jsonCmd("reload", "Reload the daemon configuration", api.MethodDaemonReload),
		st.jsonCmd("daemon-info", "Show daemon info", api.MethodDaemonInfo, "info"),
		st.newMCPCmd(),
		st.newInstallServiceCmd(),
		st.newUninstallServiceCmd(),
	}
}

func (st *rootState) newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check config and daemon health",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := st.loadCfg()
			if err != nil {
				return err
			}
			fmt.Print(cfg.DoctorView())
			rep := doctor.Check(cmd.Context(), cfg.IPC.Endpoint)
			for _, line := range rep.Lines {
				fmt.Println(line)
			}
			if !rep.OK {
				return domain.NewError(domain.CodeDaemonUnavailable, "daemon not running", true)
			}
			return nil
		},
	}
}

func (st *rootState) newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve MCP over stdio",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return st.runMCPSDK(cmd.Context())
		},
	}
}

func (st *rootState) newInstallServiceCmd() *cobra.Command {
	var bin string
	c := &cobra.Command{
		Use:   "install-service",
		Short: "Install the user-level pmmcpd service",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return installService(cmd.Context(), bin)
		},
	}
	c.Flags().StringVar(&bin, "bin", "", "path to the pmmcpd binary")
	return c
}

func (st *rootState) newUninstallServiceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall-service",
		Short: "Remove the user-level pmmcpd service",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return uninstallService(cmd.Context())
		},
	}
}
