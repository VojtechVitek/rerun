package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang-cz/rerun"
	"gopkg.in/yaml.v3"
)

// ConfigFileName is the file rerun looks for.
const ConfigFileName = "rerun.yml"

// defaultDelay is the batch window between a file change and a restart.
const defaultDelay = 200 * time.Millisecond

// ErrConfigNotFound reports that no rerun.yml was found on the way up.
var ErrConfigNotFound = errors.New("no rerun.yml found")

type configNotFoundError struct{ dir string }

func (e *configNotFoundError) Error() string {
	return fmt.Sprintf("%v in %s or any parent directory", ErrConfigNotFound, e.dir)
}

func (e *configNotFoundError) Unwrap() error { return ErrConfigNotFound }

// Target is one entry of rerun.yml, in file order.
type Target struct {
	Name     string
	Desc     string
	On       []string
	Except   []string
	Cmd      string
	Delay    time.Duration
	Cooldown time.Duration
	Grace    time.Duration
}

// Config is a parsed rerun.yml.
type Config struct {
	Path    string
	Targets []Target
}

// Names returns the target names in definition order.
func (c *Config) Names() []string {
	names := make([]string, 0, len(c.Targets))
	for _, t := range c.Targets {
		names = append(names, t.Name)
	}
	return names
}

// Find returns the named target.
func (c *Config) Find(name string) (Target, bool) {
	for _, t := range c.Targets {
		if t.Name == name {
			return t, true
		}
	}
	return Target{}, false
}

// LoadConfig walks startDir and its parents, stopping at the git root, and
// parses the first rerun.yml it finds.
func LoadConfig(startDir string) (*Config, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return nil, err
	}

	for {
		path := filepath.Join(dir, ConfigFileName)
		if _, err := os.Stat(path); err == nil {
			return ParseConfig(path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}

		if isGitRoot(dir) {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return nil, &configNotFoundError{dir: startDir}
}

// isGitRoot reports whether dir holds a .git directory or file.
func isGitRoot(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// ParseConfig reads and validates one rerun.yml file.
func ParseConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %s", path, cleanYAMLError(err))
	}

	root := &doc
	if root.Kind == yaml.DocumentNode {
		if len(root.Content) == 0 {
			return nil, fmt.Errorf("%s: no targets defined", path)
		}
		root = root.Content[0]
	}
	if root.Kind == 0 || (root.Kind == yaml.ScalarNode && root.Tag == "!!null") {
		return nil, fmt.Errorf("%s: no targets defined", path)
	}
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: expected a mapping of target names", path)
	}
	if len(root.Content) == 0 {
		return nil, fmt.Errorf("%s: no targets defined", path)
	}

	cfg := &Config{Path: path}
	seen := make(map[string]bool)
	for i := 0; i+1 < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		if key.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("%s: target names must be strings", path)
		}
		name := key.Value
		if seen[name] {
			return nil, fmt.Errorf("%s: duplicate target %q", path, name)
		}
		seen[name] = true

		target, err := parseTarget(path, name, value)
		if err != nil {
			return nil, err
		}
		cfg.Targets = append(cfg.Targets, target)
	}

	return cfg, nil
}

func parseTarget(path, name string, node *yaml.Node) (Target, error) {
	if node.Kind != yaml.MappingNode {
		return Target{}, fmt.Errorf("%s: target %q: expected a mapping of fields", path, name)
	}

	target := Target{Name: name, Delay: defaultDelay, Grace: rerun.DefaultGrace}
	seen := make(map[string]bool)
	for i := 0; i+1 < len(node.Content); i += 2 {
		field := node.Content[i].Value
		value := node.Content[i+1]
		if seen[field] {
			return Target{}, fmt.Errorf("%s: target %q: duplicate field %q", path, name, field)
		}
		seen[field] = true

		switch field {
		case "desc":
			s, err := scalarField(path, name, field, value)
			if err != nil {
				return Target{}, err
			}
			target.Desc = s

		case "on":
			list, err := stringOrList(path, name, field, value)
			if err != nil {
				return Target{}, err
			}
			target.On = list

		case "except":
			list, err := stringOrList(path, name, field, value)
			if err != nil {
				return Target{}, err
			}
			target.Except = list

		case "cmd":
			s, err := scalarField(path, name, field, value)
			if err != nil {
				return Target{}, err
			}
			target.Cmd = s

		case "delay", "cooldown", "grace":
			d, err := durationField(path, name, field, value)
			if err != nil {
				return Target{}, err
			}
			switch field {
			case "delay":
				target.Delay = d
			case "cooldown":
				target.Cooldown = d
			case "grace":
				target.Grace = d
			}

		default:
			return Target{}, fmt.Errorf("%s: target %q: unknown field %q", path, name, field)
		}
	}

	if strings.TrimSpace(target.Cmd) == "" {
		return Target{}, fmt.Errorf("%s: target %q: cmd is required", path, name)
	}
	return target, nil
}

func scalarField(path, name, field string, node *yaml.Node) (string, error) {
	if node.Kind != yaml.ScalarNode {
		return "", fmt.Errorf("%s: target %q: field %q: expected a string", path, name, field)
	}
	return node.Value, nil
}

func stringOrList(path, name, field string, node *yaml.Node) ([]string, error) {
	switch node.Kind {
	case yaml.ScalarNode:
		// `on:` with no value decodes to a null scalar; that means "no paths".
		if node.Tag == "!!null" || strings.TrimSpace(node.Value) == "" {
			return nil, nil
		}
		return []string{node.Value}, nil

	case yaml.SequenceNode:
		list := make([]string, 0, len(node.Content))
		for _, item := range node.Content {
			if item.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("%s: target %q: field %q: expected a list of strings", path, name, field)
			}
			list = append(list, item.Value)
		}
		return list, nil

	default:
		return nil, fmt.Errorf("%s: target %q: field %q: expected a string or a list of strings", path, name, field)
	}
}

func durationField(path, name, field string, node *yaml.Node) (time.Duration, error) {
	if node.Kind != yaml.ScalarNode {
		return 0, fmt.Errorf("%s: target %q: field %q: expected a duration string", path, name, field)
	}
	d, err := time.ParseDuration(node.Value)
	if err != nil {
		return 0, fmt.Errorf("%s: target %q: field %q: invalid duration %q", path, name, field, node.Value)
	}
	return d, nil
}

// cleanYAMLError strips yaml.v3's package prefix from parse errors.
func cleanYAMLError(err error) string {
	return strings.TrimPrefix(err.Error(), "yaml: ")
}
