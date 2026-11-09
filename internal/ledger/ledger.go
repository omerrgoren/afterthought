// Package ledger is the local SQLite concept ledger: concepts with counts, parked or promoted.
package ledger

import (
	"database/sql"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/omerrgoren/afterthought/internal/config"
	"github.com/omerrgoren/afterthought/internal/extract"
	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS sessions(id TEXT PRIMARY KEY, source TEXT, project TEXT, started_at TEXT, processed_at TEXT);
CREATE TABLE IF NOT EXISTS concepts(
  id INTEGER PRIMARY KEY AUTOINCREMENT, key TEXT UNIQUE, name TEXT, category TEXT, description TEXT,
  context TEXT, recap TEXT, difficulty TEXT, importance INTEGER, first_seen TEXT, last_seen TEXT,
  status TEXT DEFAULT 'new', parked INTEGER DEFAULT 0, seen INTEGER DEFAULT 1, reasked INTEGER DEFAULT 0,
  last_listed TEXT DEFAULT '', resurfaced INTEGER DEFAULT 0);
CREATE TABLE IF NOT EXISTS occurrences(
  id INTEGER PRIMARY KEY AUTOINCREMENT, concept_id INTEGER, session_id TEXT, context TEXT, importance INTEGER, signal TEXT);
CREATE TABLE IF NOT EXISTS roadmaps(week TEXT PRIMARY KEY, path TEXT, created_at TEXT, notified INTEGER DEFAULT 0, last_notified TEXT DEFAULT '');
`

type Concept struct {
	ID         int64
	Key        string
	Name       string
	Category   string
	Why        string
	Context    string
	Recap      string
	Difficulty string
	Importance int
	FirstSeen  time.Time
	LastSeen   time.Time
	Status     string
	Parked     bool
	Seen       int
	Reasked    int
	LastListed string
	Resurfaced bool
}

type Ledger struct{ db *sql.DB }

func Open(path string) (*Ledger, error) {
	if path == "" {
		path = filepath.Join(config.Dir(), "ledger.db")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}
	return &Ledger{db: db}, nil
}

func (l *Ledger) Close() error { return l.db.Close() }

var nonAlnum = regexp.MustCompile(`[^\p{L}\p{N}]+`)

func Key(name string) string {
	return strings.TrimSpace(nonAlnum.ReplaceAllString(strings.ToLower(name), " "))
}

func (l *Ledger) SessionDone(id string) bool {
	var n int
	_ = l.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id=?`, id).Scan(&n)
	return n > 0
}

func (l *Ledger) MarkSession(id, source, project string, started, now time.Time) error {
	_, err := l.db.Exec(`INSERT OR REPLACE INTO sessions VALUES(?,?,?,?,?)`, id, source, project, ts(started), ts(now))
	return err
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }
func pt(s string) time.Time { t, _ := time.Parse(time.RFC3339, s); return t }

var depthRank = map[string]int{"easy": 0, "mid": 1, "hard": 2}

// InScope applies the user's chosen topics and depth.
func InScope(cfg config.Config, name, category, difficulty string) bool {
	if depthRank[difficulty] < depthRank[cfg.Depth] {
		return false
	}
	if len(cfg.Topics) == 0 {
		return true
	}
	hay := strings.ToLower(name + " " + category)
	for _, t := range cfg.Topics {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" && strings.Contains(hay, t) {
			return true
		}
	}
	return false
}

// Upsert records one extracted concept. Out-of-scope or weak-signal concepts are parked, never dropped;
// repeated re-asking promotes a parked concept.
func (l *Ledger) Upsert(cfg config.Config, c extract.Concept, sessionID string, now time.Time) error {
	key := Key(c.Name)
	if key == "" {
		return nil
	}
	reask := 0
	if c.TimesAsked > 1 {
		reask = c.TimesAsked - 1
	}
	parked := !InScope(cfg, c.Name, c.Category, c.Difficulty)
	if c.Signal == "explained_only" && c.Difficulty != "hard" && reask == 0 {
		parked = true
	}
	var id int64
	var status string
	var oldParked, listed, reasked int
	var lastListed string
	err := l.db.QueryRow(`SELECT id,status,parked,last_listed,reasked FROM concepts WHERE key=?`, key).
		Scan(&id, &status, &oldParked, &lastListed, &reasked)
	if err == sql.ErrNoRows {
		st := "new"
		if c.Signal == "knows" {
			st = "known"
		}
		if reask >= 2 {
			parked = false
		}
		res, err := l.db.Exec(`INSERT INTO concepts(key,name,category,description,context,recap,difficulty,importance,first_seen,last_seen,status,parked,seen,reasked)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,1,?)`, key, c.Name, c.Category, c.Why, c.Context, c.Recap, c.Difficulty, c.Importance, ts(now), ts(now), st, b2i(parked), reask)
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
	} else if err != nil {
		return err
	} else {
		if c.Signal == "asked" {
			reask++ // asking again in a later session means it didn't stick
		}
		reasked += reask
		newParked := oldParked == 1 && parked
		if reasked >= 2 {
			newParked = false
		}
		if oldParked == 0 {
			newParked = false
		}
		if status == "known" && c.Signal != "knows" && (c.Signal == "asked" || c.Signal == "auto_accepted") {
			status = "new" // "known" can fade
			listed = 1
		}
		if c.Signal == "knows" {
			status = "known"
		}
		if lastListed != "" && (c.Signal == "asked" || c.Signal == "auto_accepted") {
			listed = 1
		}
		_, err := l.db.Exec(`UPDATE concepts SET seen=seen+1,last_seen=?,status=?,parked=?,reasked=?,resurfaced=MAX(resurfaced,?),
			recap=CASE WHEN ?<>'' THEN ? ELSE recap END, importance=MAX(importance,?) WHERE id=?`,
			ts(now), status, b2i(newParked), reasked, listed, c.Recap, c.Recap, c.Importance, id)
		if err != nil {
			return err
		}
	}
	_, err = l.db.Exec(`INSERT INTO occurrences(concept_id,session_id,context,importance,signal) VALUES(?,?,?,?,?)`,
		id, sessionID, c.Context, c.Importance, c.Signal)
	return err
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (l *Ledger) query(where string, args ...any) ([]Concept, error) {
	rows, err := l.db.Query(`SELECT id,key,name,category,description,context,recap,difficulty,importance,first_seen,last_seen,status,parked,seen,reasked,last_listed,resurfaced FROM concepts WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Concept
	for rows.Next() {
		var c Concept
		var fs, ls string
		var parked, res int
		if err := rows.Scan(&c.ID, &c.Key, &c.Name, &c.Category, &c.Why, &c.Context, &c.Recap, &c.Difficulty, &c.Importance, &fs, &ls, &c.Status, &parked, &c.Seen, &c.Reasked, &c.LastListed, &res); err != nil {
			return nil, err
		}
		c.FirstSeen, c.LastSeen, c.Parked, c.Resurfaced = pt(fs), pt(ls), parked == 1, res == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

// Score = repeats x importance x recency, plus extra weight for re-asking.
func Score(c Concept, now time.Time) float64 {
	age := now.Sub(c.LastSeen).Hours() / 24
	return float64(c.Seen)*float64(c.Importance)*(1/(1+age/30)) + float64(c.Reasked)*2
}

func sortByScore(cs []Concept, now time.Time) {
	sort.SliceStable(cs, func(i, j int) bool { return Score(cs[i], now) > Score(cs[j], now) })
}

// Fresh returns promoted concepts not yet listed (or resurfaced since), best first.
func (l *Ledger) Fresh(now time.Time) ([]Concept, error) {
	cs, err := l.query(`parked=0 AND status IN ('new','learning') AND (last_listed='' OR resurfaced=1)`)
	sortByScore(cs, now)
	return cs, err
}

// Parked returns the parking lot, most-seen first.
func (l *Ledger) Parked(now time.Time) ([]Concept, error) {
	cs, err := l.query(`parked=1 AND status IN ('new','learning') AND last_listed=''`)
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].Seen > cs[j].Seen })
	return cs, err
}

// Listed returns concepts already delivered, for recap fill on quiet weeks.
func (l *Ledger) Listed(now time.Time) ([]Concept, error) {
	cs, err := l.query(`last_listed<>'' AND status IN ('new','learning')`)
	sortByScore(cs, now)
	return cs, err
}

func (l *Ledger) All() ([]Concept, error) { return l.query(`1=1 ORDER BY id`) }

func (l *Ledger) KnownNames() []string {
	cs, _ := l.query(`1=1 ORDER BY last_seen DESC LIMIT 200`)
	var out []string
	for _, c := range cs {
		out = append(out, c.Name)
	}
	return out
}

func (l *Ledger) MarkListed(ids []int64, week string) error {
	for _, id := range ids {
		if _, err := l.db.Exec(`UPDATE concepts SET last_listed=?,resurfaced=0,parked=0 WHERE id=?`, week, id); err != nil {
			return err
		}
	}
	return nil
}

func (l *Ledger) SetStatus(name, status string) (bool, error) {
	r, err := l.db.Exec(`UPDATE concepts SET status=? WHERE key=?`, status, Key(name))
	if err != nil {
		return false, err
	}
	n, _ := r.RowsAffected()
	return n > 0, nil
}

// Roadmaps

type Roadmap struct {
	Week         string
	Path         string
	Created      time.Time
	Notified     int
	LastNotified time.Time
}

func (l *Ledger) Roadmap(week string) (*Roadmap, error) {
	var r Roadmap
	var cr, ln string
	err := l.db.QueryRow(`SELECT week,path,created_at,notified,last_notified FROM roadmaps WHERE week=?`, week).Scan(&r.Week, &r.Path, &cr, &r.Notified, &ln)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.Created, r.LastNotified = pt(cr), pt(ln)
	return &r, nil
}

func (l *Ledger) LatestRoadmap() (*Roadmap, error) {
	var w string
	if err := l.db.QueryRow(`SELECT week FROM roadmaps ORDER BY week DESC LIMIT 1`).Scan(&w); err != nil {
		return nil, nil
	}
	return l.Roadmap(w)
}

func (l *Ledger) AddRoadmap(week, path string, now time.Time) error {
	_, err := l.db.Exec(`INSERT OR IGNORE INTO roadmaps(week,path,created_at) VALUES(?,?,?)`, week, path, ts(now))
	return err
}

func (l *Ledger) BumpNotified(week string, now time.Time) error {
	_, err := l.db.Exec(`UPDATE roadmaps SET notified=notified+1,last_notified=? WHERE week=?`, ts(now), week)
	return err
}

func (l *Ledger) Counts() (concepts, parked, sessions int) {
	_ = l.db.QueryRow(`SELECT COUNT(*) FROM concepts`).Scan(&concepts)
	_ = l.db.QueryRow(`SELECT COUNT(*) FROM concepts WHERE parked=1`).Scan(&parked)
	_ = l.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessions)
	return
}
