// Package pipeline wires adapters -> cleaner -> extraction -> ledger -> roadmap -> notifications.
package pipeline

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/omerrgoren/afterthought/internal/adapter"
	"github.com/omerrgoren/afterthought/internal/clean"
	"github.com/omerrgoren/afterthought/internal/config"
	"github.com/omerrgoren/afterthought/internal/extract"
	"github.com/omerrgoren/afterthought/internal/ledger"
	"github.com/omerrgoren/afterthought/internal/llm"
	"github.com/omerrgoren/afterthought/internal/notify"
	"github.com/omerrgoren/afterthought/internal/roadmap"
	"github.com/omerrgoren/afterthought/internal/signals"
)

type Deps struct {
	Cfg         config.Config
	L           *ledger.Ledger
	R           llm.Runner
	Adapters    []adapter.Adapter
	Now         time.Time
	RoadmapDir  string
	Notify      func(title, msg string) // defaults to notify.Send
	Active      time.Duration           // sessions touched more recently than this are left for later
	OnlySession string                  // when set, process just this session id (prefix) and skip the roadmap
}

type Result struct {
	Sessions int
	Concepts int
	Roadmap  string
	Created  bool
	Errors   []error
}

func Ignored(cfg config.Config, project string) bool {
	for _, p := range cfg.Ignore {
		if p != "" && strings.Contains(project, p) {
			return true
		}
	}
	return false
}

// Fetch returns sessions to look at: not ignored, not currently active.
func Fetch(d Deps, since time.Time) ([]adapter.Session, []error) {
	var out []adapter.Session
	var errs []error
	for _, a := range d.Adapters {
		ss, err := a.FetchSessions(since)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", a.Name(), err))
			continue
		}
		for _, s := range ss {
			if d.OnlySession != "" {
				if !strings.HasPrefix(s.ID, d.OnlySession) {
					continue
				}
			} else if Ignored(d.Cfg, s.Project) {
				continue
			}
			if d.OnlySession == "" && d.Active > 0 && !s.Updated.IsZero() && d.Now.Sub(s.Updated) < d.Active {
				continue
			}
			out = append(out, s)
		}
	}
	return out, errs
}

// Preview returns the exact masked, cleaned payloads that would go to the AI.
func Preview(d Deps, since time.Time) (map[string]string, []error) {
	ss, errs := Fetch(d, since)
	out := map[string]string{}
	for _, s := range ss {
		if d.L != nil && d.L.SessionDone(s.ID) {
			continue
		}
		out[s.Source+"/"+s.ID] = clean.Session(s, d.Cfg.TokenCap)
	}
	return out, errs
}

func Run(ctx context.Context, d Deps) (Result, error) {
	var res Result
	if d.Cfg.Paused {
		return res, nil
	}
	if d.Notify == nil {
		d.Notify = notify.Send
	}
	sessions, errs := Fetch(d, d.Now.AddDate(0, 0, -14))
	res.Errors = append(res.Errors, errs...)
	for _, s := range sessions {
		if d.L.SessionDone(s.ID) {
			continue
		}
		users := clean.UserTurns(s)
		if len(users) == 0 {
			_ = d.L.MarkSession(s.ID, s.Source, s.Project, s.Started, d.Now)
			continue
		}
		hint := ""
		for _, g := range signals.Repeated(users) {
			hint += "- " + g[0] + " (asked " + fmt.Sprint(len(g)) + " times)\n"
		}
		cs, err := extract.Concepts(ctx, d.R, clean.Session(s, d.Cfg.TokenCap), d.L.KnownNames(), hint)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Errorf("session %s: %w", s.ID, err))
			continue // retry next run
		}
		for _, c := range cs {
			if err := d.L.Upsert(d.Cfg, c, s.ID, d.Now); err != nil {
				res.Errors = append(res.Errors, err)
			}
		}
		res.Concepts += len(cs)
		res.Sessions++
		_ = d.L.MarkSession(s.ID, s.Source, s.Project, s.Started, d.Now)
	}
	if d.OnlySession != "" {
		return res, nil
	}
	path, created, err := roadmap.Generate(ctx, d.R, d.L, d.Cfg, d.RoadmapDir, d.Now, false)
	if err != nil {
		res.Errors = append(res.Errors, err)
	}
	res.Roadmap, res.Created = path, created
	notifyStep(d)
	return res, nil
}

// notifyStep sends two Friday notifications: one on generation, a second at least an hour later.
func notifyStep(d Deps) {
	week := roadmap.WeekKey(roadmap.Boundary(d.Now))
	rm, err := d.L.Roadmap(week)
	if err != nil || rm == nil || rm.Notified >= 2 || d.Now.Sub(roadmap.Boundary(d.Now)) > 72*time.Hour {
		return
	}
	if rm.Notified == 1 && d.Now.Sub(rm.LastNotified) < time.Hour {
		return
	}
	d.Notify("afterthought", "Your weekly learning roadmap is ready: "+rm.Path)
	_ = d.L.BumpNotified(week, d.Now)
}
