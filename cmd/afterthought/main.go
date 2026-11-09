// afterthought finds what you didn't understand in AI coding sessions and writes a weekly learning roadmap.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/omerrgoren/afterthought/internal/adapter"
	"github.com/omerrgoren/afterthought/internal/config"
	"github.com/omerrgoren/afterthought/internal/hook"
	"github.com/omerrgoren/afterthought/internal/ledger"
	"github.com/omerrgoren/afterthought/internal/llm"
	"github.com/omerrgoren/afterthought/internal/notify"
	"github.com/omerrgoren/afterthought/internal/pipeline"
	"github.com/omerrgoren/afterthought/internal/roadmap"
)

const usage = `afterthought: learn what your AI fixed for you.

  install              register the global Claude Code hook (works for every project)
  uninstall            remove the hook
  pause | resume       stop / restart all processing
  setup [--topics a,b] [--depth easy|mid|hard]
  ignore add|rm|list [path-substring]
  scan [--preview] [--session <id>]  process sessions now (or just one, no roadmap), or print what would be sent
  roadmap [--force]    generate this week's roadmap now
  mark known|ignore|learning <concept>
  status`

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		fmt.Println(usage)
		return
	}
	cmd, args := os.Args[1], os.Args[2:]
	if cmd == "version" || cmd == "--version" {
		fmt.Println("afterthought", version)
		return
	}
	cfg, err := config.Load()
	if err != nil {
		fatal(err)
	}
	switch cmd {
	case "hook":
		cmdHook(cfg)
	case "run":
		cmdRun(cfg, false, "")
	case "scan":
		only := flagVal(args, "--session")
		if has(args, "--preview") {
			cmdPreview(cfg, only)
		} else {
			cmdRun(cfg, true, only)
		}
	case "install":
		cmdInstall()
	case "uninstall":
		changed, err := hook.Uninstall(hook.SettingsPath())
		fatalIf(err)
		fmt.Println(map[bool]string{true: "afterthought hook removed.", false: "No afterthought hook was installed."}[changed])
	case "pause", "resume":
		cfg.Paused = cmd == "pause"
		fatalIf(cfg.Save())
		fmt.Println("afterthought " + cmd + "d.")
	case "setup":
		cmdSetup(cfg, args)
	case "ignore":
		cmdIgnore(cfg, args)
	case "roadmap":
		cmdRoadmap(cfg, has(args, "--force"))
	case "mark":
		cmdMark(args)
	case "status":
		cmdStatus(cfg)
	default:
		fmt.Println(usage)
	}
}

func fatal(err error) { fmt.Fprintln(os.Stderr, "afterthought:", err); os.Exit(1) }
func fatalIf(err error) {
	if err != nil {
		fatal(err)
	}
}
func flagVal(a []string, name string) string {
	for i, x := range a {
		if x == name && i+1 < len(a) {
			return a[i+1]
		}
	}
	return ""
}
func has(a []string, s string) bool {
	for _, x := range a {
		if x == s {
			return true
		}
	}
	return false
}

func deps(cfg config.Config, l *ledger.Ledger) pipeline.Deps {
	return pipeline.Deps{
		Cfg: cfg, L: l, R: llm.CLI{Cmd: cfg.LLMCmd},
		Adapters:   []adapter.Adapter{adapter.NewClaudeCode(), adapter.NewCursor()},
		Now:        time.Now(),
		RoadmapDir: filepath.Join(config.Dir(), "roadmaps"),
		Active:     5 * time.Minute,
	}
}

// cmdHook runs on every session start: it must return at once and never block the session.
func cmdHook(cfg config.Config) {
	if os.Getenv("AFTERTHOUGHT_RUNNING") != "" || cfg.Paused {
		return
	}
	if n := notify.Pending(); n != "" {
		b, _ := json.Marshal(map[string]string{"systemMessage": n})
		fmt.Println(string(b))
	}
	self, err := os.Executable()
	if err != nil {
		return
	}
	c := exec.Command(self, "run")
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if f, err := os.OpenFile(filepath.Join(config.Dir(), "run.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		c.Stdout, c.Stderr = f, f
	}
	_ = os.MkdirAll(config.Dir(), 0o755)
	_ = c.Start() // detached; not waited on
}

func lock() (func(), bool) {
	_ = os.MkdirAll(config.Dir(), 0o755)
	p := filepath.Join(config.Dir(), "run.lock")
	if st, err := os.Stat(p); err == nil && time.Since(st.ModTime()) > 30*time.Minute {
		_ = os.Remove(p)
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, false
	}
	f.Close()
	return func() { _ = os.Remove(p) }, true
}

func cmdRun(cfg config.Config, verbose bool, only string) {
	if cfg.Paused {
		fmt.Println("afterthought is paused (run `afterthought resume`).")
		return
	}
	unlock, ok := lock()
	if !ok {
		if verbose {
			fmt.Println("Another afterthought run is in progress.")
		}
		return
	}
	defer unlock()
	l, err := ledger.Open("")
	fatalIf(err)
	defer l.Close()
	d := deps(cfg, l)
	d.OnlySession = only
	res, err := pipeline.Run(context.Background(), d)
	fatalIf(err)
	for _, e := range res.Errors {
		fmt.Fprintln(os.Stderr, "warning:", e)
	}
	if verbose {
		fmt.Printf("Processed %d session(s), %d concept(s) found.\n", res.Sessions, res.Concepts)
		if res.Created {
			fmt.Println("New roadmap:", res.Roadmap)
		}
	}
}

func cmdPreview(cfg config.Config, only string) {
	l, err := ledger.Open("")
	fatalIf(err)
	defer l.Close()
	d := deps(cfg, l)
	d.OnlySession = only
	payloads, errs := pipeline.Preview(d, d.Now.AddDate(0, 0, -14))
	for _, e := range errs {
		fmt.Fprintln(os.Stderr, "warning:", e)
	}
	for k, p := range payloads {
		fmt.Printf("===== %s =====\n%s\n\n", k, p)
	}
	fmt.Printf("(%d session(s) previewed; nothing was sent.)\n", len(payloads))
}

func cmdInstall() {
	self, err := os.Executable()
	fatalIf(err)
	self, _ = filepath.EvalSymlinks(self)
	changed, err := hook.Install(hook.SettingsPath(), self)
	fatalIf(err)
	if changed {
		fmt.Println("Installed the afterthought hook in", hook.SettingsPath())
	} else {
		fmt.Println("afterthought hook was already installed.")
	}
	fmt.Println(`
What afterthought reads: your Claude Code (and Cursor) chat text, in every project, locally.
Tool output, file contents and long code are dropped and secrets are masked before anything
goes to your own AI CLI. Nothing else leaves this machine.
Exclude a project: afterthought ignore add <path>   Stop: afterthought pause | uninstall
See what would be sent: afterthought scan --preview`)
}

func cmdSetup(cfg config.Config, args []string) {
	var topics, depth string
	for i, a := range args {
		if a == "--topics" && i+1 < len(args) {
			topics = args[i+1]
		}
		if a == "--depth" && i+1 < len(args) {
			depth = args[i+1]
		}
	}
	if topics == "" && depth == "" {
		in := bufio.NewReader(os.Stdin)
		fmt.Print("Topics to cover (comma-separated, empty = everything): ")
		topics, _ = in.ReadString('\n')
		fmt.Print("Minimum depth (easy = everything, mid, hard) [easy]: ")
		depth, _ = in.ReadString('\n')
	}
	topics, depth = strings.TrimSpace(topics), strings.TrimSpace(depth)
	if topics != "" || len(args) > 0 {
		cfg.Topics = nil
		for _, t := range strings.Split(topics, ",") {
			if t = strings.TrimSpace(t); t != "" {
				cfg.Topics = append(cfg.Topics, t)
			}
		}
	}
	switch depth {
	case "easy", "mid", "hard":
		cfg.Depth = depth
	case "":
	default:
		fatal(fmt.Errorf("depth must be easy, mid or hard"))
	}
	fatalIf(cfg.Save())
	fmt.Printf("Saved. Topics: %v, depth: %s. Concepts outside these are parked, not dropped.\n", cfg.Topics, cfg.Depth)
}

func cmdIgnore(cfg config.Config, args []string) {
	if len(args) == 0 {
		fatal(fmt.Errorf("usage: ignore add|rm|list [path-substring]"))
	}
	switch args[0] {
	case "list":
		for _, p := range cfg.Ignore {
			fmt.Println(p)
		}
		return
	case "add", "rm":
		if len(args) < 2 {
			fatal(fmt.Errorf("missing path"))
		}
		var keep []string
		for _, p := range cfg.Ignore {
			if p != args[1] {
				keep = append(keep, p)
			}
		}
		if args[0] == "add" {
			keep = append(keep, args[1])
		}
		cfg.Ignore = keep
		fatalIf(cfg.Save())
		fmt.Println("Ignore list:", cfg.Ignore)
	}
}

func cmdRoadmap(cfg config.Config, force bool) {
	l, err := ledger.Open("")
	fatalIf(err)
	defer l.Close()
	d := deps(cfg, l)
	path, created, err := roadmap.Generate(context.Background(), d.R, l, cfg, d.RoadmapDir, time.Now(), force)
	fatalIf(err)
	if created {
		fmt.Println("Wrote", path)
	} else {
		fmt.Println("This week's roadmap already exists:", path, "(use --force to regenerate)")
	}
}

func cmdMark(args []string) {
	if len(args) < 2 {
		fatal(fmt.Errorf("usage: mark known|ignore|learning <concept>"))
	}
	st := map[string]string{"known": "known", "ignore": "ignored", "learning": "learning"}[args[0]]
	if st == "" {
		fatal(fmt.Errorf("unknown status %q", args[0]))
	}
	l, err := ledger.Open("")
	fatalIf(err)
	defer l.Close()
	ok, err := l.SetStatus(strings.Join(args[1:], " "), st)
	fatalIf(err)
	if !ok {
		fatal(fmt.Errorf("no such concept"))
	}
	fmt.Println("Marked as", st)
}

func cmdStatus(cfg config.Config) {
	l, err := ledger.Open("")
	fatalIf(err)
	defer l.Close()
	c, p, s := l.Counts()
	fmt.Printf("hook installed: %v\npaused: %v\nsessions processed: %d\nconcepts: %d (parked: %d)\n", hook.Installed(hook.SettingsPath()), cfg.Paused, s, c, p)
	if rm, _ := l.LatestRoadmap(); rm != nil {
		fmt.Println("latest roadmap:", rm.Path)
	}
}
