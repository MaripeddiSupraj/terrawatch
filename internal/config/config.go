package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// IgnoreRule defines a rule for filtering out specific resource changes from drift detection.
type IgnoreRule struct {
	// Resource is a glob pattern matched against the resource address (e.g. "aws_autoscaling_group.*").
	// Uses path.Match semantics: * matches any sequence within a segment.
	Resource string `mapstructure:"resource"`
	// Attributes is an optional list of dot-path attribute names to ignore.
	// When empty, the entire resource is ignored. When set, only those specific attributes
	// are ignored — if no other diffs remain, the resource is reported clean.
	Attributes []string `mapstructure:"attributes"`
}

// Drift mode values for Config.DriftMode.
const (
	// DriftModeAll treats any plan changes as drift (default).
	DriftModeAll = "all"
	// DriftModeStrict only treats true infrastructure drift as drift:
	// a refresh-only plan is run first, and merged-but-unapplied code
	// changes are reported informationally without failing the run.
	DriftModeStrict = "strict"
)

type Config struct {
	Ignore    []IgnoreRule `mapstructure:"ignore"`
	Stacks    []Stack      `mapstructure:"stacks"`
	GitHub    GitHub       `mapstructure:"github"`
	GitLab    GitLab       `mapstructure:"gitlab"`
	Terraform Terraform    `mapstructure:"terraform"`
	// DriftMode is "all" (default) or "strict" — see DriftMode constants.
	DriftMode string `mapstructure:"drift_mode"`
	// AutoClose closes a stack's open drift PR when the stack comes back
	// clean. Defaults to true; nil means unset.
	AutoClose *bool `mapstructure:"auto_close"`
	// Concurrency is how many stacks are scanned in parallel.
	// 0 (unset) means sequential; the --parallel flag overrides it.
	Concurrency int `mapstructure:"concurrency"`
}

// AutoCloseEnabled returns the auto_close setting, defaulting to true.
func (c *Config) AutoCloseEnabled() bool {
	return c.AutoClose == nil || *c.AutoClose
}

type Stack struct {
	Name          string            `mapstructure:"name"`
	Path          string            `mapstructure:"path"`
	VarsFile      string            `mapstructure:"vars_file"`
	BackendConfig map[string]string `mapstructure:"backend_config"`
	Ignore        []IgnoreRule      `mapstructure:"ignore"`
}

type GitHub struct {
	Token      string   `mapstructure:"token"`
	Repo       string   `mapstructure:"repo"` // "owner/repo"
	BaseBranch string   `mapstructure:"base_branch"`
	Labels     []string `mapstructure:"labels"`
	Assignees  []string `mapstructure:"assignees"`
}

type GitLab struct {
	Token      string   `mapstructure:"token"`
	Repo       string   `mapstructure:"repo"` // "group/project"
	BaseURL    string   `mapstructure:"url"`  // defaults to https://gitlab.com
	BaseBranch string   `mapstructure:"base_branch"`
	Labels     []string `mapstructure:"labels"`
	Assignees  []string `mapstructure:"assignees"`
}

type Terraform struct {
	BinPath string `mapstructure:"bin_path"`
	// Timeout bounds each terraform command (e.g. "10m", "1h").
	// Empty uses the built-in default; "0" disables the timeout.
	Timeout string `mapstructure:"timeout"`
}

// LocalConfigFromDirs builds a minimal config from one or more directories,
// with no reporter — used when terrawatch is run without a config file.
func LocalConfigFromDirs(dirs []string) *Config {
	var stacks []Stack
	for _, dir := range dirs {
		name := filepath.Base(dir)
		if name == "." || name == "" {
			name = "current"
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			abs = dir
		}
		stacks = append(stacks, Stack{Name: name, Path: abs})
	}
	return &Config{Stacks: stacks}
}

// HasTerraformFiles reports whether dir contains at least one .tf file.
func HasTerraformFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".tf" {
			return true
		}
	}
	return false
}

// FindTerraformDirs walks root and returns all directories that contain .tf files.
func FindTerraformDirs(root string) ([]string, error) {
	var dirs []string
	seen := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && (d.Name() == ".terraform" || d.Name() == ".git") {
			return filepath.SkipDir
		}
		if !d.IsDir() && filepath.Ext(d.Name()) == ".tf" {
			parent := filepath.Dir(path)
			if !seen[parent] {
				seen[parent] = true
				dirs = append(dirs, parent)
			}
		}
		return nil
	})
	return dirs, err
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetEnvPrefix("TERRAWATCH")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// env var token overrides
	if token := os.Getenv("GITHUB_TOKEN"); token != "" && cfg.GitHub.Token == "" {
		cfg.GitHub.Token = token
	}
	if token := os.Getenv("GITLAB_TOKEN"); token != "" && cfg.GitLab.Token == "" {
		cfg.GitLab.Token = token
	}

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func validate(cfg *Config) error {
	if len(cfg.Stacks) == 0 {
		return fmt.Errorf("config: at least one stack is required")
	}
	for _, s := range cfg.Stacks {
		if s.Name == "" {
			return fmt.Errorf("config: stack name is required")
		}
		if s.Path == "" {
			return fmt.Errorf("config: stack %q path is required", s.Name)
		}
	}

	hasGitHub := cfg.GitHub.Repo != ""
	hasGitLab := cfg.GitLab.Repo != ""

	if !hasGitHub && !hasGitLab {
		return fmt.Errorf("config: either github.repo or gitlab.repo is required")
	}
	if hasGitHub && hasGitLab {
		return fmt.Errorf("config: specify either github or gitlab, not both")
	}

	if hasGitHub {
		if cfg.GitHub.Token == "" {
			return fmt.Errorf("config: github token required via config or GITHUB_TOKEN env var")
		}
		if cfg.GitHub.BaseBranch == "" {
			cfg.GitHub.BaseBranch = "main"
		}
	}

	if hasGitLab {
		if cfg.GitLab.Token == "" {
			return fmt.Errorf("config: gitlab token required via config or GITLAB_TOKEN env var")
		}
		if cfg.GitLab.BaseBranch == "" {
			cfg.GitLab.BaseBranch = "main"
		}
		if cfg.GitLab.BaseURL == "" {
			cfg.GitLab.BaseURL = "https://gitlab.com"
		}
	}

	if cfg.Concurrency < 0 {
		return fmt.Errorf("config: concurrency must not be negative, got %d", cfg.Concurrency)
	}

	switch cfg.DriftMode {
	case "", DriftModeAll:
		cfg.DriftMode = DriftModeAll
	case DriftModeStrict:
	default:
		return fmt.Errorf("config: drift_mode must be %q or %q, got %q",
			DriftModeAll, DriftModeStrict, cfg.DriftMode)
	}

	if cfg.Terraform.Timeout != "" {
		d, err := time.ParseDuration(cfg.Terraform.Timeout)
		if err != nil {
			return fmt.Errorf("config: terraform.timeout %q is not a valid duration (e.g. \"10m\", \"1h\"): %w",
				cfg.Terraform.Timeout, err)
		}
		if d < 0 {
			return fmt.Errorf("config: terraform.timeout %q must not be negative (use \"0\" to disable)",
				cfg.Terraform.Timeout)
		}
	}

	return nil
}

// TerraformTimeout returns the configured per-command timeout, or def when unset.
func (c *Config) TerraformTimeout(def time.Duration) time.Duration {
	if c.Terraform.Timeout == "" {
		return def
	}
	d, err := time.ParseDuration(c.Terraform.Timeout)
	if err != nil {
		return def
	}
	return d
}
