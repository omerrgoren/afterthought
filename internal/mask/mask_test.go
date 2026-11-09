package mask

import (
	"strings"
	"testing"
)

func TestSecretsMasked(t *testing.T) {
	in := "key sk-abcdefghijklmnop1234 and ghp_abcdefghijklmnopqrstuvwx and password=hunter2hunter2 and postgres://u:pa55word@host/db\n-----BEGIN RSA PRIVATE KEY-----\nabc\n-----END RSA PRIVATE KEY-----"
	out := Text(in)
	for _, s := range []string{"sk-abcdefghijklmnop1234", "ghp_abc", "hunter2hunter2", "pa55word", "BEGIN RSA"} {
		if strings.Contains(out, s) {
			t.Errorf("%q leaked: %s", s, out)
		}
	}
	if !strings.Contains(out, "password=") {
		t.Error("key name should survive")
	}
}

func TestPlainTextUntouched(t *testing.T) {
	in := "why does readiness differ from liveness?"
	if Text(in) != in {
		t.Error("plain text changed")
	}
}
