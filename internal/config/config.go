package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Stacks    []Stack   `mapstructure:"stacks"`
	GitHub    GitHub    `mapstructure:"github"`
	GitLab    GitLab    `mapstructure:"gitlab"`
	Terraform Terraform `mapstructure:"terraform"`
}

type Stack struct {
	Name          string            `mapstructure:"name"`
	Path          string            `mapstructure:"path"`
	VarsFile      string            `mapstructure:"vars_file"`
	BackendConfig map[string]string `mapstructure:"backend_config"`
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

	return nil
}
