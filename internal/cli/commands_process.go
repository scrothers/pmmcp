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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/scrothers/pmmcp/internal/api"
	"github.com/scrothers/pmmcp/internal/domain"
)

// processCommands builds the process-lifecycle command set.
func (st *rootState) processCommands() []*cobra.Command {
	return []*cobra.Command{
		st.newStartCmd(),
		st.newRunCmd(),
		st.newListCmd(),
		st.newStatusCmd(),
		st.idCmd("stop", "Stop a process", api.MethodStop),
		st.idCmd("restart", "Restart a process", api.MethodRestart),
		st.idCmd("remove", "Remove a process", api.MethodRemove, "rm"),
		st.idCmd("wait", "Wait for a process to exit", api.MethodWait),
		st.idCmd("enable", "Enable autostart for a process", api.MethodEnable),
		st.idCmd("disable", "Disable autostart for a process", api.MethodDisable),
		st.idCmd("health", "Run a process health check", api.MethodHealthCheck, "health-check"),
		st.newLogsCmd(),
		st.newLogLikeCmd("grep", "Grep a process's logs", api.MethodGrep),
		st.newLogLikeCmd("errors", "Show a process's error lines", api.MethodErrors),
		st.newEventsCmd(),
		st.newPortsCmd(),
	}
}

// startPayload assembles the start request from parsed flags and command argv.
func startPayload(name, cwd, sandbox, project string, command []string) api.StartPayload {
	return api.StartPayload{Name: name, Command: command, Cwd: cwd, Sandbox: sandbox, Project: project}
}

func (st *rootState) newStartCmd() *cobra.Command {
	var name, cwd, sandbox, project string
	c := &cobra.Command{
		Use:   "start [flags] -- command [args...]",
		Short: "Start a supervised process",
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return domain.NewError(domain.CodeInvalidArgument, "--name required", false)
			}
			if len(args) == 0 {
				return domain.NewError(domain.CodeInvalidArgument, "command argv required after flags (use -- cmd...)", false)
			}
			c2, err := st.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer func() { _ = c2.Close() }()
			var out api.StartResult
			if err := c2.Call(cmd.Context(), api.MethodStart, startPayload(name, cwd, sandbox, project, args), &out); err != nil {
				return err
			}
			fmt.Printf("started %s name=%s pid=%d status=%s logs=%s\n", out.ID, out.Name, out.PID, out.Status, out.LogDir)
			return nil
		},
	}
	// Stop flag parsing at the first positional so the child's own flags (before a
	// -- separator) are never read as pmmcp flags.
	c.Flags().SetInterspersed(false)
	c.Flags().StringVarP(&name, "name", "n", "", "process name (required)")
	c.Flags().StringVar(&cwd, "cwd", "", "working directory")
	c.Flags().StringVar(&sandbox, "sandbox", "", "sandbox profile (strict, standard, permissive, off)")
	c.Flags().StringVarP(&project, "project", "C", "", "project directory or id")
	return c
}

// runPayload assembles the one-shot run request, omitting empty fields.
func runPayload(name, cwd string, command []string) map[string]any {
	pl := map[string]any{}
	if name != "" {
		pl["name"] = name
	}
	if cwd != "" {
		pl["cwd"] = cwd
	}
	if len(command) > 0 {
		pl["command"] = command
	}
	return pl
}

func (st *rootState) newRunCmd() *cobra.Command {
	var name, cwd string
	c := &cobra.Command{
		Use:   "run [flags] -- command [args...]",
		Short: "Run a one-shot process",
		RunE: func(cmd *cobra.Command, args []string) error {
			return st.callJSON(cmd.Context(), api.MethodRun, runPayload(name, cwd, args))
		},
	}
	c.Flags().SetInterspersed(false)
	c.Flags().StringVarP(&name, "name", "n", "", "process name")
	c.Flags().StringVar(&cwd, "cwd", "", "working directory")
	return c
}

func (st *rootState) newListCmd() *cobra.Command {
	var includeExited, all bool
	var project string
	c := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List processes",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pl := api.ListPayload{IncludeExited: includeExited, All: all, Project: project}
			if pl.Project == "" && !pl.All {
				pl.Cwd, _ = os.Getwd()
			}
			c2, err := st.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer func() { _ = c2.Close() }()
			var out []api.ProcessView
			if err := c2.Call(cmd.Context(), api.MethodList, pl, &out); err != nil {
				return err
			}
			if st.json {
				b, _ := json.MarshalIndent(out, "", "  ")
				fmt.Println(string(b))
				return nil
			}
			if len(out) == 0 {
				fmt.Println("no processes")
				return nil
			}
			for _, p := range out {
				fmt.Printf("%s  %-12s  %-10s  pid=%d  %s\n", p.ID, p.Name, p.Status, p.PID, strings.Join(p.Command, " "))
			}
			return nil
		},
	}
	c.Flags().BoolVar(&includeExited, "include-exited", false, "include exited processes")
	c.Flags().BoolVar(&all, "all", false, "list across all projects")
	c.Flags().StringVar(&project, "project", "", "filter by project")
	return c
}

func (st *rootState) newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <id-or-name>",
		Short: "Show process detail",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c2, err := st.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer func() { _ = c2.Close() }()
			pl := api.IDPayload{}
			if strings.HasPrefix(args[0], "proc-") {
				pl.ID = args[0]
			} else {
				pl.Name = args[0]
			}
			var out api.ProcessView
			if err := c2.Call(cmd.Context(), api.MethodStatus, pl, &out); err != nil {
				return err
			}
			b, _ := json.MarshalIndent(out, "", "  ")
			fmt.Println(string(b))
			return nil
		},
	}
}

// logs runs a logs-family read (logs/grep/errors) for the given method.
func (st *rootState) logs(ctx context.Context, method string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: pmmcp logs|grep|errors <id-or-name> [pattern]")
	}
	c, err := st.dial(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	pl := api.LogsPayload{Lines: 100}
	if strings.HasPrefix(args[0], "proc-") {
		pl.ID = args[0]
	} else {
		pl.Name = args[0]
	}
	if method == api.MethodGrep {
		if len(args) < 2 {
			return fmt.Errorf("grep requires pattern")
		}
		pl.Pattern = args[1]
	}
	var out api.LogsResult
	if err := c.Call(ctx, method, pl, &out); err != nil {
		return err
	}
	fmt.Print(out.Text)
	if out.Text != "" && !strings.HasSuffix(out.Text, "\n") {
		fmt.Println()
	}
	return nil
}

func (st *rootState) newLogsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "logs <id-or-name>",
		Short: "Tail a process's logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return st.logs(cmd.Context(), api.MethodLogs, args)
		},
	}
	c.AddCommand(
		st.dslCmd("export", "Export logs", api.MethodLogsExport),
		st.dslCmd("ship", "Ship logs to a sink", api.MethodLogsShip),
	)
	return c
}

func (st *rootState) newLogLikeCmd(name, short, method string) *cobra.Command {
	return &cobra.Command{
		Use:   name + " <id-or-name>",
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			return st.logs(cmd.Context(), method, args)
		},
	}
}

func (st *rootState) newEventsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "events [process-id]",
		Short: "Show recent events",
		RunE: func(cmd *cobra.Command, args []string) error {
			c2, err := st.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer func() { _ = c2.Close() }()
			pl := api.EventsPayload{Limit: 50}
			if len(args) > 0 {
				pl.ProcessID = args[0]
			}
			var out []api.EventView
			if err := c2.Call(cmd.Context(), api.MethodEvents, pl, &out); err != nil {
				return err
			}
			for _, e := range out {
				fmt.Printf("%s  %s  %s  %s\n", e.At.Format(time.RFC3339), e.ID, e.Type, e.Message)
			}
			return nil
		},
	}
}

func (st *rootState) newPortsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ports [id-or-name]",
		Short: "Show port allocations",
		RunE: func(cmd *cobra.Command, args []string) error {
			pl := map[string]any{}
			if len(args) > 0 {
				if strings.HasPrefix(args[0], "proc-") {
					pl["id"] = args[0]
				} else {
					pl["name"] = args[0]
				}
			}
			return st.callJSON(cmd.Context(), api.MethodPorts, pl)
		},
	}
}
