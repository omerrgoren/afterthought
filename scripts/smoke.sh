#!/bin/sh
# End-to-end smoke test: real binary, throwaway HOME, fake AI CLI. Touches nothing of yours.
set -eu
BIN="${1:-./bin/afterthought}"
T=$(mktemp -d)
trap 'rm -rf "$T"' EXIT
export HOME="$T" AFTERTHOUGHT_HOME="$T/.at"
mkdir -p "$T/.claude/projects/-w" "$T/bin" "$T/.at"
cat > "$T/bin/fakeai" <<'AI'
#!/bin/sh
in=$(cat)
case "$in" in
 *"Write learning notes"*) printf '=== TOPIC: iptables NAT vs filter ===\n### What is X\na\n### What is Y\nb\n### Where to use each\nc\n### Best practices\nd\n### Interesting articles and official docs\n- https://example.com/\n';;
 *) printf '[{"concept":"iptables NAT vs filter","category":"network","context":"dns fails","why_it_matters":"w","understanding_signal":"auto_accepted","importance":4,"difficulty":"hard","times_asked":1,"recap":"r"}]';;
esac
AI
chmod +x "$T/bin/fakeai"
printf '{"type":"user","sessionId":"s1","cwd":"/w","timestamp":"2026-11-01T10:00:00Z","message":{"content":"dns fails, token=abcdef123456789"}}\n{"type":"assistant","timestamp":"2026-11-01T10:00:05Z","message":{"content":[{"type":"text","text":"Add a MASQUERADE rule."}]}}\n' > "$T/.claude/projects/-w/s1.jsonl"
touch -t 202611010000 "$T/.claude/projects/-w/s1.jsonl"
echo "{\"llm_cmd\":[\"$T/bin/fakeai\"]}" > "$T/.at/config.json"

"$BIN" install >/dev/null
"$BIN" status | grep -q 'hook installed: true'
"$BIN" scan --preview | grep -q 'token=\[REDACTED\]'
"$BIN" hook
i=0; while [ ! -f "$T"/.at/roadmaps/*-roadmap.md ] 2>/dev/null && [ $i -lt 50 ]; do sleep 0.2; i=$((i+1)); done
grep -q 'iptables NAT vs filter' "$T"/.at/roadmaps/*-roadmap.md
"$BIN" uninstall | grep -q removed
echo "smoke: OK"
