package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallKeepsOtherHooksAndUninstallRestores(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	orig := `{"model":"x","hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"icm wake-up"}]}],"Stop":[{"hooks":[]}]}}`
	os.WriteFile(p, []byte(orig), 0o644)
	if ch, err := Install(p, "/usr/local/bin/afterthought"); err != nil || !ch {
		t.Fatal(ch, err)
	}
	if ch, _ := Install(p, "/usr/local/bin/afterthought"); ch {
		t.Fatal("second install should be a no-op")
	}
	if !Installed(p) {
		t.Fatal("not installed")
	}
	var m map[string]any
	b, _ := os.ReadFile(p)
	json.Unmarshal(b, &m)
	if m["model"] != "x" || len(m["hooks"].(map[string]any)["SessionStart"].([]any)) != 2 {
		t.Fatalf("other settings damaged: %s", b)
	}
	if ch, _ := Uninstall(p); !ch || Installed(p) {
		t.Fatal("uninstall failed")
	}
	b, _ = os.ReadFile(p)
	json.Unmarshal(b, &m)
	if len(m["hooks"].(map[string]any)["SessionStart"].([]any)) != 1 {
		t.Fatalf("icm hook lost: %s", b)
	}
}
