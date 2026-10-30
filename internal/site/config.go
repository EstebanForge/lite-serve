package site

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

const (
	ModePHP   = "php"
	ModeProxy = "proxy"
)

const ConfigFile = "lserve.json"

// Config is the optional, committable project configuration.
type Config struct {
	Mode string `json:"mode,omitempty"`
	Port int    `json:"port,omitempty"`
	Name string `json:"name,omitempty"`
}

func LoadConfig(projectDir string) (Config, bool, error) {
	path := filepath.Join(projectDir, ConfigFile)
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Config{}, false, nil
	}
	if err != nil {
		return Config{}, false, err
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, false, fmt.Errorf("%s: %w", path, err)
	}
	if cfg.Mode == "" {
		cfg.Mode = ModePHP
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, false, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, true, nil
}

func SaveConfig(projectDir string, cfg Config) error {
	if cfg.Mode == "" {
		cfg.Mode = ModePHP
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(projectDir, ConfigFile), append(b, '\n'), 0o644)
}

func (c Config) Validate() error {
	switch c.Mode {
	case ModePHP:
		if c.Port != 0 {
			return fmt.Errorf("port only applies to proxy mode")
		}
	case ModeProxy:
		if c.Port < 1 || c.Port > 65535 {
			return fmt.Errorf("proxy port must be between 1 and 65535")
		}
		reserved := map[int]bool{2019: true}
		for key, fallback := range map[string]int{
			"DEVENV_DB_PORT": 3306, "DEVENV_MAILPIT_UI_PORT": 8025,
			"DEVENV_MAILPIT_SMTP_PORT": 1025, "DEVENV_HTTP_PORT": 8080,
			"DEVENV_HTTPS_PORT": 8443,
		} {
			port := fallback
			if value, err := strconv.Atoi(os.Getenv(key)); err == nil {
				port = value
			}
			reserved[port] = true
		}
		if reserved[c.Port] {
			return fmt.Errorf("proxy port %d is reserved by lserve", c.Port)
		}
	default:
		return fmt.Errorf("unknown mode %q (want php or proxy)", c.Mode)
	}
	if c.Name != "" && !validName(c.Name) {
		return fmt.Errorf("name %q must be a valid host label", c.Name)
	}
	return nil
}

// Detect identifies common app projects for the interactive default.
// The direct result means PHP or static content can be served without
// an upstream, including in public/, web/, or an explicit docroot.
func Detect(projectDir string) (kind string, defaultPort int, direct bool) {
	return DetectWithDocroot(projectDir, "")
}

func DetectWithDocroot(projectDir, subdir string) (kind string, defaultPort int, direct bool) {
	root := filepath.Join(projectDir, subdir)
	for _, marker := range []string{"wp-load.php", "composer.json"} {
		if fileExists(filepath.Join(projectDir, marker)) {
			return "", 0, true
		}
	}
	roots := []string{root}
	if subdir == "" {
		roots = append(roots, filepath.Join(root, "public"), filepath.Join(root, "web"))
	}
	for _, candidate := range roots {
		if matches, _ := filepath.Glob(filepath.Join(candidate, "*.php")); len(matches) > 0 {
			return "", 0, true
		}
		if fileExists(filepath.Join(candidate, "index.html")) {
			direct = true
		}
	}
	checks := []struct {
		kind    string
		port    int
		markers []string
	}{
		{"node", 3000, []string{"package.json"}},
		{"ruby", 3000, []string{"Gemfile"}},
		{"python", 8000, []string{"pyproject.toml", "requirements.txt"}},
		{"go", 8081, []string{"go.mod"}},
	}
	for _, check := range checks {
		for _, marker := range check.markers {
			if fileExists(filepath.Join(projectDir, marker)) {
				return check.kind, check.port, direct
			}
		}
	}
	return "", 0, direct
}

func StartHint(kind string, port int) string {
	switch kind {
	case "node":
		return "npm run dev"
	case "ruby":
		return "bundle exec rails server"
	case "python":
		return "uvicorn main:app --reload --port " + strconv.Itoa(port)
	case "go":
		return "go run ."
	}
	return "start your app on port " + strconv.Itoa(port)
}
