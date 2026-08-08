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
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/scrothers/pmmcp/internal/api"
	"github.com/scrothers/pmmcp/internal/domain"
)

// declarativeCommands builds the top-level payload-DSL commands (declare/apply
// family and cross-cutting query verbs).
func (st *rootState) declarativeCommands() []*cobra.Command {
	return []*cobra.Command{
		st.dslCmd("update", "Update a process spec", api.MethodUpdate),
		st.dslCmd("validate", "Validate a declared spec", api.MethodValidate),
		st.dslCmd("apply", "Apply a declared spec", api.MethodApply),
		st.dslCmd("diff", "Diff a declared spec", api.MethodDiff),
		st.dslCmd("audit", "Query the audit log", api.MethodAudit),
		st.dslCmd("runtime", "Show runtime info", api.MethodRuntimeInfo),
		st.dslCmd("import", "Import processes from a spec", api.MethodImport),
		st.dslCmd("share", "Share a process", api.MethodShare),
		st.dslCmd("unshare", "Unshare a process", api.MethodUnshare),
		parentCmd("declare", "Declarative spec commands",
			st.dslCmd("show", "Show the current declared spec", api.MethodDeclareShow),
		),
	}
}

// multiVerbCommands builds the noun-with-subcommands groups.
func (st *rootState) multiVerbCommands() []*cobra.Command {
	return []*cobra.Command{
		parentCmd("group", "Manage process groups",
			st.dslCmd("create", "Create a group", api.MethodGroupCreate),
			st.idCmd("remove", "Remove a group", api.MethodGroupRemove, "rm"),
			st.jsonCmd("list", "List groups", api.MethodGroupList, "ls"),
			st.idCmd("status", "Show group status", api.MethodGroupStatus),
			st.idCmd("start", "Start a group", api.MethodGroupStart),
			st.idCmd("stop", "Stop a group", api.MethodGroupStop),
			st.idCmd("restart", "Restart a group", api.MethodGroupRestart),
		),
		parentCmd("profile", "Manage sandbox profiles",
			st.jsonCmd("list", "List profiles", api.MethodProfileList, "ls"),
			st.idCmd("get", "Get a profile", api.MethodProfileGet),
			st.dslCmd("create", "Create a profile", api.MethodProfileCreate),
			st.dslCmd("update", "Update a profile", api.MethodProfileUpdate),
			st.idCmd("delete", "Delete a profile", api.MethodProfileDelete, "rm"),
			st.idCmd("use", "Use a profile", api.MethodProfileUse),
		),
		parentCmd("webhook", "Manage webhooks",
			st.dslCmd("create", "Create a webhook", api.MethodWebhookCreate),
			st.dslCmd("update", "Update a webhook", api.MethodWebhookUpdate),
			st.idCmd("delete", "Delete a webhook", api.MethodWebhookDelete, "rm"),
			st.jsonCmd("list", "List webhooks", api.MethodWebhookList, "ls"),
			st.idCmd("test", "Test a webhook", api.MethodWebhookTest),
		),
		parentCmd("session", "Session commands",
			st.dslCmd("info", "Show session info", api.MethodSessionInfo),
			st.dslCmd("end", "End the session", api.MethodSessionEnd),
		),
		parentCmd("secret", "Manage secrets",
			st.jsonCmd("list", "List secrets", api.MethodSecretList, "ls"),
			st.newSecretSetCmd(),
			st.dslCmd("check", "Check a secret reference", api.MethodSecretRefCheck),
		),
		parentCmd("watch", "Manage watches",
			st.dslCmd("set", "Set a watch", api.MethodWatchSet),
			st.jsonCmd("status", "Show watch status", api.MethodWatchStatus),
		),
		parentCmd("project", "Project commands",
			st.dslCmd("current", "Show the current project", api.MethodProjectCurrent),
			st.jsonCmd("list", "List projects", api.MethodProjectList, "ls"),
		),
	}
}

func (st *rootState) newSecretSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "set NAME",
		Short:              "Set a secret (value read from stdin)",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
				return cmd.Help()
			}
			return st.secretSet(cmd.Context(), args)
		},
	}
}

// secretSet stores a secret. The value is read from stdin (never argv) so it does
// not leak into shell history or `ps` output. Non-value fields may be supplied as
// flags; a value on argv is rejected.
func (st *rootState) secretSet(ctx context.Context, args []string) error {
	pl := payloadFromArgs(args)
	if _, ok := pl["value"]; ok {
		return domain.NewError(domain.CodeInvalidArgument,
			"secret value must be piped on stdin, not passed on the command line", false)
	}
	if _, ok := pl["name"]; !ok {
		return domain.NewError(domain.CodeInvalidArgument, "usage: pmmcp secret set NAME < value", false)
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return domain.WrapError(domain.CodeInvalidArgument, "read secret from stdin", false, err)
	}
	value := strings.TrimRight(string(data), "\r\n")
	if value == "" {
		return domain.NewError(domain.CodeInvalidArgument, "empty secret on stdin", false)
	}
	pl["value"] = value
	return st.callJSON(ctx, api.MethodSecretSet, pl)
}
