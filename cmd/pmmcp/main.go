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

// Command pmmcp is the CLI and MCP adapter client for the pmmcp daemon.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/scrothers/pmmcp/internal/cli"
	"github.com/scrothers/pmmcp/internal/domain"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := cli.Execute(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "pmmcp: %v\n", err)
		os.Exit(domain.ExitCodeFromError(err))
	}
}
