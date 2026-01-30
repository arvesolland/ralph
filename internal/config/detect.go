// Package config handles configuration loading and management.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// DetectedConfig contains auto-detected project configuration.
type DetectedConfig struct {
	Language    string   // Primary language detected (e.g., "node", "go", "python")
	Framework   string   // Framework if detected (e.g., "react", "nextjs", "django")
	PackageJSON *PackageJSON // Parsed package.json if Node.js project
	Commands    CommandsConfig
}

// PackageJSON represents a minimal package.json structure for script extraction.
type PackageJSON struct {
	Name    string            `json:"name"`
	Scripts map[string]string `json:"scripts"`
}

// Detect auto-detects project configuration from files in the given directory.
// It examines common files (package.json, go.mod, etc.) to determine project type
// and appropriate test/lint/build commands.
func Detect(dir string) (*DetectedConfig, error) {
	detected := &DetectedConfig{}

	// Check for Node.js project
	if cfg, err := detectNodeJS(dir); err == nil && cfg != nil {
		return cfg, nil
	}

	// Check for Go project
	if cfg, err := detectGo(dir); err == nil && cfg != nil {
		return cfg, nil
	}

	// Check for Python project
	if cfg, err := detectPython(dir); err == nil && cfg != nil {
		return cfg, nil
	}

	// Check for PHP project
	if cfg, err := detectPHP(dir); err == nil && cfg != nil {
		return cfg, nil
	}

	// Check for Rust project
	if cfg, err := detectRust(dir); err == nil && cfg != nil {
		return cfg, nil
	}

	// Check for Ruby project
	if cfg, err := detectRuby(dir); err == nil && cfg != nil {
		return cfg, nil
	}

	// No project detected, return empty config
	return detected, nil
}

// detectNodeJS checks for package.json and extracts configuration.
func detectNodeJS(dir string) (*DetectedConfig, error) {
	pkgPath := filepath.Join(dir, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var pkg PackageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}

	cfg := &DetectedConfig{
		Language:    "node",
		PackageJSON: &pkg,
	}

	// Extract commands from scripts
	if pkg.Scripts != nil {
		if script, ok := pkg.Scripts["test"]; ok && script != "" {
			cfg.Commands.Test = "npm test"
		}
		if script, ok := pkg.Scripts["lint"]; ok && script != "" {
			cfg.Commands.Lint = "npm run lint"
		}
		if script, ok := pkg.Scripts["build"]; ok && script != "" {
			cfg.Commands.Build = "npm run build"
		}
		if script, ok := pkg.Scripts["dev"]; ok && script != "" {
			cfg.Commands.Dev = "npm run dev"
		}
	}

	// Detect framework from dependencies or files
	cfg.Framework = detectNodeFramework(dir, &pkg)

	return cfg, nil
}

// detectNodeFramework attempts to identify the framework used.
func detectNodeFramework(dir string, pkg *PackageJSON) string {
	// Check for Next.js
	if fileExists(filepath.Join(dir, "next.config.js")) ||
		fileExists(filepath.Join(dir, "next.config.mjs")) ||
		fileExists(filepath.Join(dir, "next.config.ts")) {
		return "nextjs"
	}

	// Check for Nuxt
	if fileExists(filepath.Join(dir, "nuxt.config.js")) ||
		fileExists(filepath.Join(dir, "nuxt.config.ts")) {
		return "nuxt"
	}

	// Check for Vite
	if fileExists(filepath.Join(dir, "vite.config.js")) ||
		fileExists(filepath.Join(dir, "vite.config.ts")) {
		return "vite"
	}

	return ""
}

// detectGo checks for go.mod and returns Go project configuration.
func detectGo(dir string) (*DetectedConfig, error) {
	modPath := filepath.Join(dir, "go.mod")
	if !fileExists(modPath) {
		return nil, nil
	}

	cfg := &DetectedConfig{
		Language: "go",
		Commands: CommandsConfig{
			Test:  "go test ./...",
			Build: "go build ./...",
		},
	}

	// Check for common linters
	if fileExists(filepath.Join(dir, ".golangci.yml")) ||
		fileExists(filepath.Join(dir, ".golangci.yaml")) {
		cfg.Commands.Lint = "golangci-lint run"
	}

	return cfg, nil
}

// detectPython checks for Python project files.
func detectPython(dir string) (*DetectedConfig, error) {
	// Check for pyproject.toml first (modern Python)
	if fileExists(filepath.Join(dir, "pyproject.toml")) {
		cfg := &DetectedConfig{
			Language: "python",
			Commands: CommandsConfig{
				Test: "pytest",
			},
		}

		// Check for common lint tools
		if fileExists(filepath.Join(dir, ".flake8")) {
			cfg.Commands.Lint = "flake8"
		} else if fileExists(filepath.Join(dir, "ruff.toml")) ||
			fileExists(filepath.Join(dir, ".ruff.toml")) {
			cfg.Commands.Lint = "ruff check"
		}

		return cfg, nil
	}

	// Check for requirements.txt (traditional Python)
	if fileExists(filepath.Join(dir, "requirements.txt")) {
		cfg := &DetectedConfig{
			Language: "python",
			Commands: CommandsConfig{
				Test: "pytest",
			},
		}

		return cfg, nil
	}

	return nil, nil
}

// detectPHP checks for composer.json.
func detectPHP(dir string) (*DetectedConfig, error) {
	composerPath := filepath.Join(dir, "composer.json")
	if !fileExists(composerPath) {
		return nil, nil
	}

	cfg := &DetectedConfig{
		Language: "php",
		Commands: CommandsConfig{
			Test: "vendor/bin/phpunit",
		},
	}

	// Check for Laravel
	if fileExists(filepath.Join(dir, "artisan")) {
		cfg.Framework = "laravel"
		cfg.Commands.Test = "php artisan test"
	}

	// Check for common linters
	if fileExists(filepath.Join(dir, "phpcs.xml")) ||
		fileExists(filepath.Join(dir, "phpcs.xml.dist")) {
		cfg.Commands.Lint = "vendor/bin/phpcs"
	} else if fileExists(filepath.Join(dir, "phpstan.neon")) ||
		fileExists(filepath.Join(dir, "phpstan.neon.dist")) {
		cfg.Commands.Lint = "vendor/bin/phpstan analyse"
	}

	return cfg, nil
}

// detectRust checks for Cargo.toml.
func detectRust(dir string) (*DetectedConfig, error) {
	cargoPath := filepath.Join(dir, "Cargo.toml")
	if !fileExists(cargoPath) {
		return nil, nil
	}

	cfg := &DetectedConfig{
		Language: "rust",
		Commands: CommandsConfig{
			Test:  "cargo test",
			Build: "cargo build",
			Lint:  "cargo clippy",
		},
	}

	return cfg, nil
}

// detectRuby checks for Gemfile.
func detectRuby(dir string) (*DetectedConfig, error) {
	gemfilePath := filepath.Join(dir, "Gemfile")
	if !fileExists(gemfilePath) {
		return nil, nil
	}

	cfg := &DetectedConfig{
		Language: "ruby",
		Commands: CommandsConfig{
			Test: "bundle exec rspec",
		},
	}

	// Check for Rails
	if fileExists(filepath.Join(dir, "config", "application.rb")) {
		cfg.Framework = "rails"
		cfg.Commands.Test = "bundle exec rails test"
	}

	// Check for RuboCop
	if fileExists(filepath.Join(dir, ".rubocop.yml")) {
		cfg.Commands.Lint = "bundle exec rubocop"
	}

	return cfg, nil
}

// fileExists checks if a file exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
