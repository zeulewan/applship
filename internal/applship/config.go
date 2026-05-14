package applship

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Project       string `json:"project"`
	Workspace     string `json:"workspace"`
	Scheme        string `json:"scheme"`
	BundleID      string `json:"bundleId"`
	TeamID        string `json:"teamId"`
	Configuration string `json:"configuration"`
	OutputDir     string `json:"outputDir"`
}

func LoadConfig() (Config, error) {
	cfg := Config{
		Configuration: "Release",
		OutputDir:     "build/applship",
	}
	path := ".applship.json"
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &cfg); err != nil {
			return cfg, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return cfg, err
	}
	if cfg.Project == "" && cfg.Workspace == "" {
		project, workspace := detectXcodeContainer()
		cfg.Project = project
		cfg.Workspace = workspace
	}
	return cfg, nil
}

func (c Config) XcodeContainerArgs() []string {
	if c.Workspace != "" {
		return []string{"-workspace", c.Workspace}
	}
	if c.Project != "" {
		return []string{"-project", c.Project}
	}
	return nil
}

func (c Config) ValidateBuild() error {
	if c.Scheme == "" {
		return errors.New("missing scheme; set it in .applship.json or pass --scheme")
	}
	if c.Project == "" && c.Workspace == "" {
		return errors.New("missing project/workspace; set project or workspace in .applship.json")
	}
	return nil
}

func detectXcodeContainer() (project, workspace string) {
	var projects, workspaces []string
	_ = filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil || path == "." {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasSuffix(name, ".xcworkspace") {
				workspaces = append(workspaces, path)
				return filepath.SkipDir
			}
			if strings.HasSuffix(name, ".xcodeproj") {
				projects = append(projects, path)
				return filepath.SkipDir
			}
			if name == "node_modules" || name == ".git" || name == "build" || name == "DerivedData" {
				return filepath.SkipDir
			}
		}
		return nil
	})
	if len(workspaces) == 1 {
		workspace = workspaces[0]
	}
	if len(projects) == 1 {
		project = projects[0]
	}
	return project, workspace
}

func mergeFlag(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func marshalConfig(cfg Config) ([]byte, error) {
	return json.MarshalIndent(cfg, "", "  ")
}
