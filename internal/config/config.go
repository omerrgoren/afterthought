// Package config holds user settings and the afterthought data directory.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type Config struct {
	Topics    []string `json:"topics"`    // empty = every topic
	Depth     string   `json:"depth"`     // easy | mid | hard: minimum concept difficulty to promote
	Ignore    []string `json:"ignore"`    // project path substrings to skip
	Paused    bool     `json:"paused"`    // stop all processing
	TokenCap  int      `json:"token_cap"` // cleaned tokens sent per session
	LLMCmd    []string `json:"llm_cmd"`   // user's own AI CLI
	MaxTopics int      `json:"max_topics"`
}

func Default() Config {
	return Config{
		Depth:     "easy",
		TokenCap:  20000,
		LLMCmd:    []string{"claude", "-p", "--model", "haiku"},
		MaxTopics: 5,
	}
}

// Dir returns the data directory ($AFTERTHOUGHT_HOME or ~/.afterthought).
func Dir() string {
	if d := os.Getenv("AFTERTHOUGHT_HOME"); d != "" {
		return d
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".afterthought")
}

func path() string { return filepath.Join(Dir(), "config.json") }

func Load() (Config, error) {
	c := Default()
	b, err := os.ReadFile(path())
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return Default(), err
	}
	d := Default()
	if c.TokenCap <= 0 {
		c.TokenCap = d.TokenCap
	}
	if len(c.LLMCmd) == 0 {
		c.LLMCmd = d.LLMCmd
	}
	if c.MaxTopics <= 0 {
		c.MaxTopics = d.MaxTopics
	}
	if c.Depth == "" {
		c.Depth = d.Depth
	}
	return c, nil
}

func (c Config) Save() error {
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(path(), b, 0o644)
}
