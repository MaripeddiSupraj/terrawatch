package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Workspaces []Workspace `mapstructure:"workspaces"`
	GitHub     GitHub      `mapstructure:"github"`
	Terraform  Terraform   `mapstructure:"terraform"`
}

type Workspace struct {
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

	// allow token via env var override
	if token := os.Getenv("GITHUB_TOKEN"); token != "" && cfg.GitHub.Token == "" {
		cfg.GitHub.Token = token
	}

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func validate(cfg *Config) error {
	if len(cfg.Workspaces) == 0 {
		return fmt.Errorf("config: at least one workspace is required")
	}
	for _, ws := range cfg.Workspaces {
		if ws.Name == "" {
			return fmt.Errorf("config: workspace name is required")
		}
		if ws.Path == "" {
			return fmt.Errorf("config: workspace %q path is required", ws.Name)
		}
	}
	if cfg.GitHub.Repo == "" {
		return fmt.Errorf("config: github.repo is required (format: owner/repo)")
	}
	if cfg.GitHub.Token == "" {
		return fmt.Errorf("config: github token required via config or GITHUB_TOKEN env var")
	}
	if cfg.GitHub.BaseBranch == "" {
		cfg.GitHub.BaseBranch = "main"
	}
	return nil
}
