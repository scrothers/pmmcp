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
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/scrothers/pmmcp/internal/config"
	"github.com/scrothers/pmmcp/internal/domain"
	"github.com/scrothers/pmmcp/internal/ipc"
	"github.com/scrothers/pmmcp/internal/version"
)

// rootState holds per-invocation CLI state bound to persistent root flags. It
// replaces the former package-level jsonOutput global: every command that needs
// the flags closes over the same *rootState built in NewRootCmd.
type rootState struct {
	// json requests JSON output where a command supports both JSON and a human
	// table (currently list); bound to the persistent --json flag.
	json bool
	// cfgPath is the --config override; empty falls back to PMMCP_CONFIG.
	cfgPath string
}

// Execute builds the root command and runs it with ctx as the command context.
// cobra propagates ctx to every RunE via cmd.Context(), so contextcheck's
// heuristic (which cannot see that flow) is a false positive here.
//
//nolint:contextcheck // ctx is threaded through ExecuteContext → cmd.Context().
func Execute(ctx context.Context) error {
	return NewRootCmd().ExecuteContext(ctx)
}

// Run builds a fresh command tree and executes it with the given args. It is a
// thin convenience over NewRootCmd used by the binary entry point and tests.
//
//nolint:contextcheck // ctx is threaded through ExecuteContext → cmd.Context().
func Run(ctx context.Context, args []string) error {
	root := NewRootCmd()
	root.SetArgs(args)
	return root.ExecuteContext(ctx)
}

// NewRootCmd assembles the pmmcp client command tree. Errors and usage are
// silenced so main owns error reporting and exit-code mapping.
func NewRootCmd() *cobra.Command {
	st := &rootState{}
	root := &cobra.Command{
		Use:           "pmmcp",
		Short:         "pmmcp is the CLI and MCP client for the pmmcp daemon",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version.String(),
		// cobra reports unknown commands itself; RunE only fires for a bare `pmmcp`.
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = cmd.Help()
			return domain.NewError(domain.CodeInvalidArgument, "a subcommand is required", false)
		},
	}
	root.SetVersionTemplate("pmmcp {{.Version}}\n")
	root.PersistentFlags().BoolVar(&st.json, "json", false, "output JSON where a command supports it")
	root.PersistentFlags().StringVar(&st.cfgPath, "config", "", "config file path (overrides PMMCP_CONFIG)")
	root.AddCommand(st.commands()...)
	return root
}

// commands returns the full command set, grouped by concern to keep each builder
// small.
func (st *rootState) commands() []*cobra.Command {
	proc := st.processCommands()
	admin := st.adminCommands()
	decl := st.declarativeCommands()
	multi := st.multiVerbCommands()
	cmds := make([]*cobra.Command, 0, len(proc)+len(admin)+len(decl)+len(multi))
	cmds = append(cmds, proc...)
	cmds = append(cmds, admin...)
	cmds = append(cmds, decl...)
	cmds = append(cmds, multi...)
	return cmds
}

func (st *rootState) loadCfg() (*config.Config, error) {
	path := st.cfgPath
	if path == "" {
		path = os.Getenv("PMMCP_CONFIG")
	}
	return config.Load(config.LoadOptions{Path: path})
}

func (st *rootState) dial(ctx context.Context) (*ipc.Client, error) {
	cfg, err := st.loadCfg()
	if err != nil {
		return nil, err
	}
	return ipc.Dial(ctx, cfg.IPC.Endpoint)
}

// callJSON dials the daemon, calls method with payload, and pretty-prints the result.
func (st *rootState) callJSON(ctx context.Context, method string, payload any) error {
	c, err := st.dial(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	var out any
	if err := c.Call(ctx, method, payload, &out); err != nil {
		return err
	}
	if out == nil {
		fmt.Println("{}")
		return nil
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

// jsonCmd builds a leaf command that calls method with no payload and pretty-prints.
func (st *rootState) jsonCmd(name, short, method string, aliases ...string) *cobra.Command {
	return &cobra.Command{
		Use:     name,
		Short:   short,
		Aliases: aliases,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return st.callJSON(cmd.Context(), method, nil)
		},
	}
}

// idCmd builds a leaf command taking a single id-or-name argument.
func (st *rootState) idCmd(name, short, method string, aliases ...string) *cobra.Command {
	return &cobra.Command{
		Use:     name + " <id-or-name>",
		Short:   short,
		Aliases: aliases,
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return st.callJSON(cmd.Context(), method, idOrNamePayload(args))
		},
	}
}

// dslCmd builds a passthrough command that preserves the payload DSL: raw args
// (key=value, key:=json, --flag value, --json '{…}') are handed to payloadFromArgs
// untouched. Flag parsing is disabled so those tokens reach the handler verbatim;
// a bare -h/--help still prints help.
func (st *rootState) dslCmd(name, short, method string) *cobra.Command {
	return &cobra.Command{
		Use:                name + " [key=value | key:=json | --flag value | --json '{…}']...",
		Short:              short,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
				return cmd.Help()
			}
			return st.callJSON(cmd.Context(), method, payloadFromArgs(args))
		},
	}
}

// parentCmd builds a grouping command whose bare or unknown-subcommand invocation
// is an error (a subcommand is required), matching the pre-cobra dispatch.
func parentCmd(name, short string, subs ...*cobra.Command) *cobra.Command {
	c := &cobra.Command{
		Use:           name,
		Short:         short,
		SilenceUsage:  true,
		SilenceErrors: true,
		// cobra reports unknown subcommands itself; RunE only fires for a bare parent.
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = cmd.Help()
			return domain.NewError(domain.CodeInvalidArgument,
				fmt.Sprintf("usage: pmmcp %s <subcommand>", cmd.Name()), false)
		},
	}
	c.AddCommand(subs...)
	return c
}

// idOrNamePayload builds a minimal id/name payload from CLI args.
func idOrNamePayload(args []string) map[string]any {
	pl := map[string]any{}
	if len(args) == 0 {
		return pl
	}
	a := args[0]
	switch {
	case strings.HasPrefix(a, "proc-"), strings.HasPrefix(a, "grp-"),
		strings.HasPrefix(a, "prof-"), strings.HasPrefix(a, "wh-"):
		pl["id"] = a
	default:
		pl["name"] = a
	}
	return pl
}

// payloadFromArgs accepts raw JSON (--json '{…}'), typed key:=json pairs, plain
// key=value pairs, and --flag value pairs. Bare flag values are coerced to
// int or bool where they parse cleanly so int-typed daemon fields (lines,
// limit, timeout_sec) unmarshal correctly; use key:=json for exact typing.
func payloadFromArgs(args []string) map[string]any {
	pl := map[string]any{}
	for i := range args {
		a := args[i]
		switch {
		case a == "--json" || a == "-j":
			i++
			if i >= len(args) {
				continue
			}
			var m map[string]any
			if err := json.Unmarshal([]byte(args[i]), &m); err == nil {
				for k, v := range m {
					pl[k] = v
				}
			}
		case strings.HasPrefix(a, "--"):
			key := strings.ReplaceAll(strings.TrimPrefix(a, "--"), "-", "_")
			// A following token that is not itself a flag is this flag's value.
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				i++
				pl[key] = coerce(args[i])
			} else {
				pl[key] = true
			}
		case strings.Contains(a, ":="):
			k, v, _ := strings.Cut(a, ":=")
			var typed any
			if err := json.Unmarshal([]byte(v), &typed); err == nil {
				pl[k] = typed
			} else {
				pl[k] = v
			}
		case strings.Contains(a, "="):
			k, v, _ := strings.Cut(a, "=")
			pl[k] = coerce(v)
		default:
			_, hasName := pl["name"]
			_, hasID := pl["id"]
			if !hasName && !hasID {
				if strings.HasPrefix(a, "grp-") || strings.HasPrefix(a, "prof-") ||
					strings.HasPrefix(a, "wh-") || strings.HasPrefix(a, "proc-") {
					pl["id"] = a
				} else {
					pl["name"] = a
				}
			}
		}
	}
	return pl
}

// coerce converts a CLI string into an int or bool when it parses cleanly,
// otherwise returns the string unchanged.
func coerce(s string) any {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	switch s {
	case "true":
		return true
	case "false":
		return false
	}
	return s
}

// versionCmd prints the client version.
func (st *rootState) versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the pmmcp version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "pmmcp %s\n", version.String())
			return nil
		},
	}
}
