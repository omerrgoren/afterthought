# afterthought

Your AI fixes the problem and you move on. afterthought notices what you never understood and hands you a learning roadmap every Friday.

- Local only. Your chat text is cleaned (tool output, file contents, long code dropped), secrets are masked, and it goes only to **your own AI CLI** (`claude -p`). No API key, nothing else leaves your machine.
- One global hook covers every project. Installing it is the consent.
- Output: a weekly `.md` in `~/.afterthought/roadmaps/`, max 5 topics, never empty.

```
brew install afterthought      # or: go install ./cmd/afterthought
afterthought install           # registers the Claude Code SessionStart hook
afterthought setup             # optional: topics and depth
afterthought scan --preview    # see exactly what would be sent
```

Other commands: `pause`, `resume`, `uninstall`, `ignore add <path>`, `roadmap --force`, `mark known <concept>`, `status`.

How it works: on session start the hook returns instantly and detaches a run that reads new sessions (Claude Code, Cursor), extracts concepts, updates a SQLite ledger (`~/.afterthought/ledger.db`), and on the first run on or after Friday 13:00 writes the week's roadmap and notifies you twice. Concepts outside your topics or depth are parked, and promoted when you keep re-asking.

Status: Claude Code adapter is the tested path. The Cursor adapter targets Cursor's local state DB; that layout is unverified. License: MIT.
