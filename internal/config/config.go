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

// Package config loads daemon and client configuration once at process start.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// ErrInvalid is returned for malformed or unsupported config.
var ErrInvalid = errors.New("config: invalid")

// Sandbox profiles.
const (
	SandboxStrict     = "strict"
	SandboxStandard   = "standard"
	SandboxPermissive = "permissive"
	SandboxOff        = "off"
)

// envPrefix is the environment-variable namespace for config overlays. With the
// key replacer below, dotted config keys map to SCREAMING_SNAKE env names, e.g.
// ipc.endpoint → PMMCP_IPC_ENDPOINT and logs.max_file_mb → PMMCP_LOGS_MAX_FILE_MB.
const envPrefix = "PMMCP"

// Config is the daemon configuration document (TOML, YAML, or JSON).
type Config struct {
	Version  int            `mapstructure:"version" toml:"version"`
	StateDir string         `mapstructure:"state_dir" toml:"state_dir"`
	IPC      IPCConfig      `mapstructure:"ipc" toml:"ipc"`
	Log      LogConfig      `mapstructure:"log" toml:"log"`
	Sandbox  SandboxConfig  `mapstructure:"sandbox" toml:"sandbox"`
	Logs     ProcessLogs    `mapstructure:"logs" toml:"logs"`
	Relaunch RelaunchConfig `mapstructure:"relaunch" toml:"relaunch"`
	Webhook  WebhookConfig  `mapstructure:"webhook" toml:"webhook"`
	// TokenFile is the resolved IPC token path (sensitive — redacted in views).
	// The spec-canonical location is [ipc].token_file; this top-level key is a
	// legacy alias. When both are set in a file, [ipc].token_file wins.
	TokenFile string `mapstructure:"token_file" toml:"token_file"`
}

// IPCConfig is private IPC endpoint configuration.
type IPCConfig struct {
	// Endpoint is a Unix socket path or Windows named pipe path. Empty → platform default.
	Endpoint string `mapstructure:"endpoint" toml:"endpoint"`
	// TokenFile is the IPC token path in its spec-canonical location ([ipc].token_file).
	// Sensitive — redacted in views. Resolved into Config.TokenFile at load time.
	TokenFile string `mapstructure:"token_file" toml:"token_file"`
}

// LogConfig is daemon process logging (slog).
type LogConfig struct {
	Level  string `mapstructure:"level" toml:"level"`
	Format string `mapstructure:"format" toml:"format"`
}

// SandboxConfig holds default sandbox posture.
type SandboxConfig struct {
	Default string `mapstructure:"default" toml:"default"`
}

// ProcessLogs is managed-process log capture limits.
type ProcessLogs struct {
	MaxFileMB int  `mapstructure:"max_file_mb" toml:"max_file_mb"`
	MaxFiles  int  `mapstructure:"max_files" toml:"max_files"`
	Compress  bool `mapstructure:"compress" toml:"compress"`
}

// RelaunchConfig controls boot relaunch behaviour.
type RelaunchConfig struct {
	Enabled bool `mapstructure:"enabled" toml:"enabled"`
}

// WebhookConfig configures outbound webhook egress.
type WebhookConfig struct {
	// Allowlist restricts webhook destinations to matching host patterns or URL
	// prefixes. Empty (the default) disables all webhooks — secure by default.
	Allowlist []string `mapstructure:"allowlist" toml:"allowlist"`
}

// LoadOptions customizes Load for tests and embedding.
type LoadOptions struct {
	// Path is the config file path. Empty resolves the DefaultPath search order.
	Path string
	// GOOS overrides runtime.GOOS for platform defaults (tests).
	GOOS string
	// Home is the user home directory; empty uses os.UserHomeDir.
	Home string
	// RuntimeDir overrides XDG_RUNTIME_DIR / temp runtime location.
	RuntimeDir string
	// StateHome overrides XDG_STATE_HOME.
	StateHome string
	// ConfigHome overrides XDG_CONFIG_HOME.
	ConfigHome string
	// LookupEnv resolves the path-discovery environment variables (PMMCP_CONFIG,
	// XDG_*, APPDATA, LOCALAPPDATA, USERNAME, USER); nil uses os.LookupEnv. It does
	// NOT affect the config-value overlays, which viper reads from the real
	// process environment via AutomaticEnv — tests set those with t.Setenv.
	LookupEnv func(string) (string, bool)
	// Username for Windows pipe naming; empty uses os.Getenv("USERNAME") / USER.
	Username string
	// Flags, when non-nil, binds recognized override flags (defined by
	// RegisterDaemonFlags) so an explicit CLI flag beats env and file.
	Flags *pflag.FlagSet
}

// configKeys is the authoritative set of viper keys (dotted, lowercase). Every
// key is registered so AutomaticEnv resolves nested env-only overlays (viper only
// consults the environment for keys it already knows) and so strict decoding can
// reject anything outside this set. defaultValues supplies each key's default;
// a nil value registers the key without a non-zero default.
var defaultValues = map[string]any{
	"version":           1,
	"state_dir":         "",
	"ipc.endpoint":      "",
	"ipc.token_file":    "",
	"token_file":        "",
	"log.level":         "info",
	"log.format":        "json",
	"sandbox.default":   SandboxStrict,
	"logs.max_file_mb":  10,
	"logs.max_files":    5,
	"logs.compress":     true,
	"relaunch.enabled":  true,
	"webhook.allowlist": []string{},
}

// flagBindings maps each override flag name (dashed) to its viper key (dotted).
// --config is intentionally absent: it selects the config file rather than a
// config value, so callers pass it via LoadOptions.Path.
var flagBindings = map[string]string{
	"state-dir":         "state_dir",
	"ipc-endpoint":      "ipc.endpoint",
	"token-file":        "token_file",
	"log-level":         "log.level",
	"log-format":        "log.format",
	"sandbox-default":   "sandbox.default",
	"logs-max-file-mb":  "logs.max_file_mb",
	"logs-max-files":    "logs.max_files",
	"logs-compress":     "logs.compress",
	"relaunch":          "relaunch.enabled",
	"webhook-allowlist": "webhook.allowlist",
}

// RegisterDaemonFlags defines the persistent config-override flags on fs. Binding
// happens in Load when LoadOptions.Flags is set, keeping definition and binding in
// one vocabulary so precedence (flag > env > file > default) stays consistent
// across both binaries.
func RegisterDaemonFlags(fs *pflag.FlagSet) {
	fs.String("config", "", "config file path (overrides PMMCP_CONFIG and search order)")
	fs.String("state-dir", "", "state directory")
	fs.String("ipc-endpoint", "", "IPC endpoint (unix socket or named pipe)")
	fs.String("token-file", "", "IPC token file path")
	fs.String("log-level", "", "log level (debug, info, warn, error)")
	fs.String("log-format", "", "log format (json, text)")
	fs.String("sandbox-default", "", "default sandbox profile (strict, standard, permissive, off)")
	fs.Int("logs-max-file-mb", 0, "managed-process log file size cap in MiB")
	fs.Int("logs-max-files", 0, "managed-process log files retained")
	fs.Bool("logs-compress", true, "gzip rotated managed-process logs")
	fs.Bool("relaunch", true, "relaunch managed processes on boot")
	fs.StringSlice("webhook-allowlist", nil, "webhook destination allowlist (repeatable)")
}

// Load reads an optional config file (TOML, YAML, or JSON), applies environment
// overlays (PMMCP_*) and bound CLI flags, then fills platform defaults. Precedence
// is flag > env > file > default. When opts.Path is empty it resolves the
// Config search order via DefaultPath.
func Load(opts LoadOptions) (*Config, error) {
	v := viper.New()
	for key, val := range defaultValues {
		v.SetDefault(key, val)
	}
	v.SetEnvPrefix(envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	if opts.Flags != nil {
		for flag, key := range flagBindings {
			if f := opts.Flags.Lookup(flag); f != nil {
				_ = v.BindPFlag(key, f)
			}
		}
	}

	path := opts.Path
	if path == "" {
		if p, ok := DefaultPath(opts); ok {
			path = p
		}
	}
	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("config: read: %w", err)
			}
			return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
		}
	}

	cfg := &Config{}
	// Strict decode: an unknown or misplaced key is a misconfiguration, not a
	// silently dropped value (config load scopes validation).
	strict := func(dc *mapstructure.DecoderConfig) { dc.ErrorUnused = true }
	if err := v.Unmarshal(cfg, strict); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}

	// Fold the spec-canonical [ipc].token_file into TokenFile, preferring it over
	// the legacy top-level key. An explicit non-empty PMMCP_TOKEN_FILE (which viper
	// already resolved into TokenFile) still wins, matching the pre-viper
	// precedence; viper treats an empty env value as unset, so we do too.
	if cfg.IPC.TokenFile != "" && os.Getenv(envPrefix+"_TOKEN_FILE") == "" {
		cfg.TokenFile = cfg.IPC.TokenFile
	}

	if err := cfg.normalizeAndDefault(opts); err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// DefaultPath resolves the daemon config file per the documented search
// order, consulted by Load when no explicit path is given: PMMCP_CONFIG →
// $XDG_CONFIG_HOME/pmmcp/daemon.{toml,yaml,yml,json} → platform fallback
// (~/Library/Application Support/pmmcp/… on macOS, %APPDATA%\pmmcp\… on Windows).
// An explicitly set PMMCP_CONFIG is returned verbatim; the platform candidates
// return the first that exists. The second result reports whether one was found.
func DefaultPath(opts LoadOptions) (string, bool) {
	lookup := opts.lookup()
	if v, ok := lookup("PMMCP_CONFIG"); ok && v != "" {
		return v, true
	}
	for _, cand := range configCandidates(opts, lookup) {
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
			return cand, true
		}
	}
	return "", false
}

// configBaseName is the config file stem; supported extensions are appended.
const configBaseName = "daemon"

// configExts lists supported config file extensions in search precedence order.
var configExts = []string{"toml", "yaml", "yml", "json"}

// filesInDir expands a config home directory into pmmcp/daemon.<ext> candidates.
func filesInDir(dir string) []string {
	out := make([]string, 0, len(configExts))
	for _, ext := range configExts {
		out = append(out, filepath.Join(dir, "pmmcp", configBaseName+"."+ext))
	}
	return out
}

// configCandidates lists platform config-file locations in search order.
func configCandidates(opts LoadOptions, lookup func(string) (string, bool)) []string {
	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	home := opts.Home
	if home == "" {
		if h, err := os.UserHomeDir(); err == nil {
			home = h
		}
	}
	switch goos {
	case "windows":
		if v, ok := lookup("APPDATA"); ok && v != "" {
			return filesInDir(v)
		}
		if home != "" {
			return filesInDir(filepath.Join(home, "AppData", "Roaming"))
		}
		return nil
	case "darwin":
		var out []string
		if ch := explicitConfigHome(opts, lookup); ch != "" {
			out = append(out, filesInDir(ch)...)
		}
		if home != "" {
			out = append(out, filesInDir(filepath.Join(home, "Library", "Application Support"))...)
		}
		return out
	default:
		ch := explicitConfigHome(opts, lookup)
		if ch == "" && home != "" {
			ch = filepath.Join(home, ".config")
		}
		if ch == "" {
			return nil
		}
		return filesInDir(ch)
	}
}

// explicitConfigHome returns an operator-set config home (opts override or
// XDG_CONFIG_HOME), or "" when neither is set.
func explicitConfigHome(opts LoadOptions, lookup func(string) (string, bool)) string {
	if opts.ConfigHome != "" {
		return opts.ConfigHome
	}
	if v, ok := lookup("XDG_CONFIG_HOME"); ok && v != "" {
		return v
	}
	return ""
}

func (o LoadOptions) lookup() func(string) (string, bool) {
	if o.LookupEnv != nil {
		return o.LookupEnv
	}
	return os.LookupEnv
}

func (c *Config) normalizeAndDefault(opts LoadOptions) error {
	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	home := opts.Home
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("config: home: %w", err)
		}
		home = h
	}
	if c.StateDir == "" {
		c.StateDir = defaultStateDir(goos, home, opts)
	}
	if c.IPC.Endpoint == "" {
		c.IPC.Endpoint = defaultIPCEndpoint(goos, home, opts)
	}
	if c.Sandbox.Default == "" {
		c.Sandbox.Default = SandboxStrict
	}
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	if c.Log.Format == "" {
		c.Log.Format = "json"
	}
	if c.Logs.MaxFileMB <= 0 {
		c.Logs.MaxFileMB = 10
	}
	if c.Logs.MaxFiles <= 0 {
		c.Logs.MaxFiles = 5
	}
	if c.Version == 0 {
		c.Version = 1
	}
	return nil
}

func defaultStateDir(goos, home string, opts LoadOptions) string {
	switch goos {
	case "windows":
		base, _ := opts.lookup()("LOCALAPPDATA")
		if base == "" {
			base = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(base, "pmmcp")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "pmmcp")
	default:
		state := opts.StateHome
		if state == "" {
			if v, ok := opts.lookup()("XDG_STATE_HOME"); ok && v != "" {
				state = v
			} else {
				state = filepath.Join(home, ".local", "state")
			}
		}
		return filepath.Join(state, "pmmcp")
	}
}

func defaultIPCEndpoint(goos, home string, opts LoadOptions) string {
	switch goos {
	case "windows":
		user := opts.Username
		if user == "" {
			user, _ = opts.lookup()("USERNAME")
			if user == "" {
				user, _ = opts.lookup()("USER")
			}
			if user == "" {
				user = "user"
			}
		}
		// Named pipe path form used by gRPC-go / winio later.
		return `\\.\pipe\pmmcpd-` + sanitizePipeUser(user)
	case "darwin":
		runtimeDir := opts.RuntimeDir
		if runtimeDir == "" {
			runtimeDir = filepath.Join(os.TempDir(), "pmmcp-"+sanitizePipeUser(filepath.Base(home)))
		}
		return filepath.Join(runtimeDir, "pmmcpd.sock")
	default:
		runtimeDir := opts.RuntimeDir
		if runtimeDir == "" {
			if v, ok := opts.lookup()("XDG_RUNTIME_DIR"); ok && v != "" {
				runtimeDir = filepath.Join(v, "pmmcp")
			} else {
				runtimeDir = filepath.Join(defaultStateDir(goos, home, opts), "runtime")
			}
		} else if filepath.Base(runtimeDir) != "pmmcp" {
			// A caller-supplied runtime dir that isn't already the pmmcp dir
			// (e.g. a raw XDG_RUNTIME_DIR) gets pmmcp nested under it.
			runtimeDir = filepath.Join(runtimeDir, "pmmcp")
		}
		return filepath.Join(runtimeDir, "pmmcpd.sock")
	}
}

func sanitizePipeUser(s string) string {
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, s)
	if s == "" {
		return "user"
	}
	return s
}

func (c *Config) validate() error {
	if c.Version != 1 {
		return fmt.Errorf("%w: unsupported config version %d (want 1)", ErrInvalid, c.Version)
	}
	switch c.Sandbox.Default {
	case SandboxStrict, SandboxStandard, SandboxPermissive, SandboxOff:
	default:
		return fmt.Errorf("%w: sandbox.default %q", ErrInvalid, c.Sandbox.Default)
	}
	if c.StateDir == "" {
		return fmt.Errorf("%w: empty state_dir after defaults", ErrInvalid)
	}
	if c.IPC.Endpoint == "" {
		return fmt.Errorf("%w: empty ipc.endpoint after defaults", ErrInvalid)
	}
	return nil
}

// Redacted returns a shallow copy safe for logs/doctor output.
// The token_file path is replaced with "[redacted]"; other fields are kept.
func (c *Config) Redacted() Config {
	if c == nil {
		return Config{}
	}
	out := *c
	out.TokenFile = redactSecretPath(out.TokenFile)
	out.IPC.TokenFile = redactSecretPath(out.IPC.TokenFile)
	// Endpoint and state dir are not secrets; token_file is.
	return out
}

// String implements fmt.Stringer with redaction.
func (c *Config) String() string {
	r := c.Redacted()
	return fmt.Sprintf(
		"config{version=%d state_dir=%q ipc.endpoint=%q sandbox.default=%q token_file=%q log.level=%q}",
		r.Version, r.StateDir, r.IPC.Endpoint, r.Sandbox.Default, r.TokenFile, r.Log.Level,
	)
}

// DoctorView is a multi-line redacted dump for `pmmcp doctor`.
func (c *Config) DoctorView() string {
	r := c.Redacted()
	var b strings.Builder
	fmt.Fprintf(&b, "version = %d\n", r.Version)
	fmt.Fprintf(&b, "state_dir = %q\n", r.StateDir)
	fmt.Fprintf(&b, "ipc.endpoint = %q\n", r.IPC.Endpoint)
	fmt.Fprintf(&b, "sandbox.default = %q\n", r.Sandbox.Default)
	fmt.Fprintf(&b, "token_file = %q\n", r.TokenFile)
	fmt.Fprintf(&b, "log.level = %q\n", r.Log.Level)
	fmt.Fprintf(&b, "log.format = %q\n", r.Log.Format)
	fmt.Fprintf(&b, "logs.max_file_mb = %d\n", r.Logs.MaxFileMB)
	fmt.Fprintf(&b, "logs.max_files = %d\n", r.Logs.MaxFiles)
	fmt.Fprintf(&b, "logs.compress = %t\n", r.Logs.Compress)
	fmt.Fprintf(&b, "relaunch.enabled = %t\n", r.Relaunch.Enabled)
	fmt.Fprintf(&b, "webhook.allowlist = %d entries\n", len(r.Webhook.Allowlist))
	return b.String()
}

func redactSecretPath(p string) string {
	if p == "" {
		return ""
	}
	// Always redact token_file contents path display as redacted marker for doctor.
	return "[redacted]"
}
