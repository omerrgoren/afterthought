// Package hook installs and removes the one global Claude Code hook.
package hook

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const event = "SessionStart"

func SettingsPath() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return filepath.Join(d, "settings.json")
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".claude", "settings.json")
}

func isOurs(cmd string) bool {
	return strings.HasSuffix(cmd, " hook") && strings.Contains(cmd, "afterthought")
}

func load(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	m := map[string]any{}
	if len(strings.TrimSpace(string(b))) == 0 {
		return m, nil
	}
	return m, json.Unmarshal(b, &m)
}

func save(path string, m map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if b, err := os.ReadFile(path); err == nil {
		bak := path + ".afterthought.bak"
		if _, err := os.Stat(bak); err != nil {
			_ = os.WriteFile(bak, b, 0o600)
		}
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Install adds our SessionStart hook once, leaving every other setting and hook untouched.
func Install(path, bin string) (changed bool, err error) {
	m, err := load(path)
	if err != nil {
		return false, err
	}
	hooks, _ := m["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	list, _ := hooks[event].([]any)
	for _, g := range list {
		if hasOurs(g) {
			return false, nil
		}
	}
	list = append(list, map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "\"" + bin + "\" hook"}}})
	hooks[event] = list
	m["hooks"] = hooks
	return true, save(path, m)
}

func hasOurs(g any) bool {
	gm, _ := g.(map[string]any)
	hs, _ := gm["hooks"].([]any)
	for _, h := range hs {
		hm, _ := h.(map[string]any)
		if c, _ := hm["command"].(string); isOurs(c) {
			return true
		}
	}
	return false
}

func Installed(path string) bool {
	m, err := load(path)
	if err != nil {
		return false
	}
	hooks, _ := m["hooks"].(map[string]any)
	list, _ := hooks[event].([]any)
	for _, g := range list {
		if hasOurs(g) {
			return true
		}
	}
	return false
}

// Uninstall removes only our hook entries.
func Uninstall(path string) (changed bool, err error) {
	m, err := load(path)
	if err != nil {
		return false, err
	}
	hooks, _ := m["hooks"].(map[string]any)
	list, _ := hooks[event].([]any)
	var keep []any
	for _, g := range list {
		if hasOurs(g) {
			changed = true
			gm, _ := g.(map[string]any)
			hs, _ := gm["hooks"].([]any)
			var rest []any
			for _, h := range hs {
				hm, _ := h.(map[string]any)
				if c, _ := hm["command"].(string); !isOurs(c) {
					rest = append(rest, h)
				}
			}
			if len(rest) > 0 {
				gm["hooks"] = rest
				keep = append(keep, gm)
			}
			continue
		}
		keep = append(keep, g)
	}
	if !changed {
		return false, nil
	}
	if len(keep) == 0 {
		delete(hooks, event)
	} else {
		hooks[event] = keep
	}
	if len(hooks) == 0 {
		delete(m, "hooks")
	}
	return true, save(path, m)
}
