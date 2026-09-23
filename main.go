package main

import (
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	mathrand "math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed templates/* static/*
var assets embed.FS

type App struct {
	db        *sql.DB
	templates *template.Template
	data      string
}
type Item struct {
	ID    string
	Title string
	Media string
	Kind  string
}
type Tournament struct {
	Owner       string
	Creator     string
	Version     int
	ID          string
	Name        string
	Description string
	Items       []Item
	Count       int
}
type Match struct {
	A      string
	B      string
	Winner string
}
type State struct {
	Run         string
	Round       int
	Phase       string
	Matches     []Match
	Index       int
	Semis       []Match
	Podium      []string
	Revision    int
	History     []Match
	Consolation *State
	Finalists   []string
}
type Game struct {
	ID     string
	Name   string
	Phase  string
	Round  int
	Count  int
	Podium []string
}
type Page struct {
	Results    ResultsView
	Immersive  bool
	Editing    bool
	Owner      bool
	Created    []Tournament
	Scores     []Score
	Voters     int
	Title      string
	User       string
	Error      string
	Tournament Tournament
	Games      []Game
	State      State
	Left       Item
	Right      Item
	Podium     []Item
	Played     int
	Total      int
}

func id() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func main() {
	data := os.Getenv("DATA_DIR")
	if data == "" {
		data = "data"
	}
	if err := os.MkdirAll(filepath.Join(data, "uploads"), 0700); err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(data, "bracket.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;
 CREATE TABLE IF NOT EXISTS tournaments(id TEXT PRIMARY KEY,name TEXT NOT NULL,description TEXT NOT NULL,created TEXT DEFAULT CURRENT_TIMESTAMP);
 CREATE TABLE IF NOT EXISTS media(id TEXT PRIMARY KEY,owner TEXT NOT NULL,path TEXT NOT NULL,kind TEXT NOT NULL,claimed INTEGER DEFAULT 0);
 CREATE TABLE IF NOT EXISTS items(id TEXT PRIMARY KEY,tournament TEXT NOT NULL REFERENCES tournaments(id),title TEXT NOT NULL,media TEXT NOT NULL,kind TEXT NOT NULL,position INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS games(user TEXT NOT NULL,tournament TEXT NOT NULL REFERENCES tournaments(id),state TEXT NOT NULL,updated TEXT DEFAULT CURRENT_TIMESTAMP,PRIMARY KEY(user,tournament));`); err != nil {
		log.Fatal(err)
	}
	if err = migrate(db); err != nil {
		log.Fatal(err)
	}
	a := &App{db: db, data: data, templates: template.Must(template.ParseFS(assets, "templates/*.html"))}
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.FileServerFS(assets))
	mux.HandleFunc("GET /{$}", a.home)
	mux.HandleFunc("GET /new", a.newPage)
	mux.HandleFunc("POST /api/upload", a.upload)
	mux.HandleFunc("POST /api/tournaments", a.create)
	mux.HandleFunc("GET /media/{id}", a.media)
	mux.HandleFunc("GET /t/{id}", a.landing)
	mux.HandleFunc("GET /t/{id}/edit", a.editPage)
	mux.HandleFunc("POST /t/{id}/edit", a.edit)
	mux.HandleFunc("POST /t/{id}/delete", a.deleteTournament)
	mux.HandleFunc("POST /t/{id}/start", a.start)
	mux.HandleFunc("GET /t/{id}/play", a.play)
	mux.HandleFunc("POST /t/{id}/vote", a.vote)
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("Crown is ready at http://localhost%s", addr)
	server := &http.Server{Addr: addr, Handler: a.middleware(mux), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second}
	log.Fatal(server.ListenAndServe())
}
func (a *App) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' blob:; media-src 'self' blob:; style-src 'self'; script-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		if !strings.HasPrefix(r.URL.Path, "/static/") && !strings.HasPrefix(r.URL.Path, "/media/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		c, err := r.Cookie("crown_session")
		valid := err == nil && len(c.Value) == 48
		if valid {
			_, decodeErr := hex.DecodeString(c.Value)
			valid = decodeErr == nil
		}
		if !valid {
			c = &http.Cookie{Name: "crown_session", Value: id(), Path: "/", MaxAge: 365 * 86400, HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteLaxMode}
			http.SetCookie(w, c)
			cookies := r.Cookies()
			r.Header.Del("Cookie")
			for _, other := range cookies {
				if other.Name != "crown_session" {
					r.AddCookie(other)
				}
			}
			r.AddCookie(c)
		}
		if r.Method == "POST" {
			if origin := r.Header.Get("Origin"); origin != "" {
				u, e := url.Parse(origin)
				if e != nil || u.Host != r.Host {
					http.Error(w, "Invalid origin", 403)
					return
				}
			}
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				http.Error(w, "Invalid origin", 403)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
func user(r *http.Request) string { c, _ := r.Cookie("crown_session"); return c.Value }
func (a *App) render(w http.ResponseWriter, name string, p Page) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.templates.ExecuteTemplate(w, name, p); err != nil {
		log.Printf("render: %v", err)
	}
}
func fail(w http.ResponseWriter, err error) {
	log.Print(err)
	http.Error(w, "Something went wrong. Please try again.", 500)
}
func (a *App) getTournament(id string) (Tournament, error) {
	t := Tournament{ID: id}
	err := a.db.QueryRow("SELECT name,description,owner,creator,version FROM tournaments WHERE id=?", id).Scan(&t.Name, &t.Description, &t.Owner, &t.Creator, &t.Version)
	if err != nil {
		return t, err
	}
	rows, err := a.db.Query("SELECT id,title,media,kind FROM items WHERE tournament=? ORDER BY position", id)
	if err != nil {
		return t, err
	}
	defer rows.Close()
	for rows.Next() {
		var i Item
		if err = rows.Scan(&i.ID, &i.Title, &i.Media, &i.Kind); err != nil {
			return t, err
		}
		t.Items = append(t.Items, i)
	}
	t.Count = len(t.Items)
	return t, rows.Err()
}
func (a *App) getState(u, t string) (State, error) {
	var raw string
	var s State
	err := a.db.QueryRow("SELECT state FROM games WHERE user=? AND tournament=?", u, t).Scan(&raw)
	if err != nil {
		return s, err
	}
	err = json.Unmarshal([]byte(raw), &s)
	return s, err
}
func (a *App) home(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(`SELECT t.id,t.name,g.state,(SELECT count(*) FROM items WHERE tournament=t.id) FROM games g JOIN tournaments t ON t.id=g.tournament WHERE g.user=? ORDER BY g.updated DESC`, user(r))
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	p := Page{Title: "Your arena"}
	for rows.Next() {
		var g Game
		var raw string
		var s State
		if err = rows.Scan(&g.ID, &g.Name, &raw, &g.Count); err != nil {
			fail(w, err)
			return
		}
		if err = json.Unmarshal([]byte(raw), &s); err != nil {
			fail(w, err)
			return
		}
		g.Phase = s.Phase
		g.Round = s.Round
		p.Games = append(p.Games, g)
	}
	rows.Close()
	created, err := a.db.Query("SELECT id,name,creator FROM tournaments WHERE owner=? ORDER BY created DESC", user(r))
	if err != nil {
		fail(w, err)
		return
	}
	defer created.Close()
	for created.Next() {
		var t Tournament
		if err = created.Scan(&t.ID, &t.Name, &t.Creator); err != nil {
			fail(w, err)
			return
		}
		p.Created = append(p.Created, t)
	}
	a.render(w, "home", p)
}
func (a *App) newPage(w http.ResponseWriter, r *http.Request) {
	a.render(w, "new", Page{Title: "Create a tournament"})
}
func (a *App) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 65<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		http.Error(w, "Choose a file smaller than 64 MB.", 400)
		return
	}
	defer r.MultipartForm.RemoveAll()
	f, h, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "A file is required.", 400)
		return
	}
	defer f.Close()
	if h.Size > 64<<20 {
		http.Error(w, "File must be smaller than 64 MB.", 400)
		return
	}
	buf := make([]byte, 512)
	n, _ := io.ReadFull(f, buf)
	buf = buf[:n]
	mime := http.DetectContentType(buf)
	kind := "image"
	ext := ""
	switch mime {
	case "image/jpeg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	case "image/gif":
		ext = ".gif"
	case "image/webp":
		ext = ".webp"
	case "video/mp4":
		ext = ".mp4"
		kind = "video"
	default:
		http.Error(w, "Supported files: JPG, PNG, GIF, WebP and MP4.", 400)
		return
	}
	mid := id()
	path := filepath.Join("uploads", mid+ext)
	out, err := os.Create(filepath.Join(a.data, path))
	if err != nil {
		fail(w, err)
		return
	}
	_, err = out.Write(buf)
	if err == nil {
		_, err = io.Copy(out, f)
	}
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		_, err = a.db.Exec("INSERT INTO media(id,owner,path,kind) VALUES(?,?,?,?)", mid, user(r), path, kind)
	}
	if err != nil {
		os.Remove(filepath.Join(a.data, path))
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": mid, "kind": kind})
}
func (a *App) media(w http.ResponseWriter, r *http.Request) {
	var path string
	err := a.db.QueryRow("SELECT path FROM media WHERE id=? AND (claimed=1 OR owner=?)", r.PathValue("id"), user(r)).Scan(&path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, filepath.Join(a.data, path))
}
func (a *App) create(w http.ResponseWriter, r *http.Request) {
	input, ok := readInput(w, r)
	if !ok {
		return
	}
	tx, err := a.db.Begin()
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback()
	tid := id()
	if _, err = tx.Exec("INSERT INTO tournaments(id,name,description,owner,creator) VALUES(?,?,?,?,?)", tid, input.Name, input.Description, user(r), input.Creator); err != nil {
		fail(w, err)
		return
	}
	for pos, i := range input.Items {
		i.Title = strings.TrimSpace(i.Title)
		if len(i.Title) > 160 || (i.Title == "" && i.Media == "") {
			http.Error(w, "Every contender needs a title or media. Titles can be up to 160 characters.", 400)
			return
		}
		kind := "text"
		if i.Media != "" {
			if err = tx.QueryRow("SELECT kind FROM media WHERE id=? AND owner=? AND claimed=0", i.Media, user(r)).Scan(&kind); err != nil {
				http.Error(w, "An upload is unavailable. Please upload it again.", 400)
				return
			}
			if _, err = tx.Exec("UPDATE media SET claimed=1 WHERE id=?", i.Media); err != nil {
				fail(w, err)
				return
			}
		}
		if _, err = tx.Exec("INSERT INTO items VALUES(?,?,?,?,?,?)", id(), tid, i.Title, i.Media, kind, pos); err != nil {
			fail(w, err)
			return
		}
	}
	if err = tx.Commit(); err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": "/t/" + tid})
}
func (a *App) landing(w http.ResponseWriter, r *http.Request) {
	t, err := a.getTournament(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s, err := a.getState(user(r), t.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		fail(w, err)
		return
	}
	a.render(w, "landing", Page{Title: t.Name, Tournament: t, State: s, Owner: t.Owner != "" && t.Owner == user(r)})
}
func shuffle(ids []string) {
	mathrand.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
}
func buildRound(s *State, ids []string) {
	s.Index = 0
	s.Matches = nil
	s.Round++
	switch len(ids) {
	case 1:
		s.Phase = "done"
		s.Podium = ids
		return
	case 2:
		s.Phase = "final"
		s.Matches = []Match{{A: ids[0], B: ids[1]}}
		return
	case 3:
		s.Phase = "three"
		s.Matches = []Match{{A: ids[0], B: ids[1]}, {A: ids[2], B: ids[mathrand.IntN(2)]}}
		return
	case 4:
		s.Phase = "semifinal"
	default:
		s.Phase = "round"
	}
	for i := 0; i+1 < len(ids); i += 2 {
		s.Matches = append(s.Matches, Match{A: ids[i], B: ids[i+1]})
	}
	if len(ids)%2 == 1 {
		s.Matches = append(s.Matches, Match{A: ids[len(ids)-1], B: ids[mathrand.IntN(len(ids)-1)]})
	}
}
func contains(ids []string, id string) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}
func loser(m Match) string {
	if m.Winner == m.A {
		return m.B
	}
	return m.A
}
func advance(s *State, winner string) error {
	if s.Phase == "done" || s.Index >= len(s.Matches) {
		return errors.New("no active match")
	}
	if s.Phase == "consolation" {
		if err := advance(s.Consolation, winner); err != nil {
			return err
		}
		s.Revision++
		s.History = append(s.History, s.Consolation.History[len(s.Consolation.History)-1])
		if s.Consolation.Phase == "done" {
			s.Podium = []string{"", "", s.Consolation.Podium[0], s.Consolation.Podium[1]}
			s.Phase = "final"
			s.Index = 0
			s.Round++
			s.Matches = []Match{{A: s.Finalists[0], B: s.Finalists[1]}}
			s.Consolation = nil
		} else {
			s.Matches = append([]Match{}, s.Consolation.Matches...)
			s.Index = s.Consolation.Index
		}
		return nil
	}
	m := &s.Matches[s.Index]
	if winner != m.A && winner != m.B {
		return errors.New("invalid contender")
	}
	m.Winner = winner
	s.History = append(s.History, *m)
	s.Index++
	s.Revision++
	if s.Index < len(s.Matches) {
		return nil
	}
	switch s.Phase {
	case "semifinal":
		s.Semis = append([]Match{}, s.Matches...)
		s.Phase = "bronze"
		s.Matches = []Match{{A: loser(s.Semis[0]), B: loser(s.Semis[1])}}
		s.Index = 0
	case "bronze":
		s.Podium = []string{"", "", winner, loser(*m)}
		s.Phase = "final"
		s.Round++
		s.Matches = []Match{{A: s.Semis[0].Winner, B: s.Semis[1].Winner}}
		s.Index = 0
	case "three":
		entrants := []string{}
		winners := []string{}
		for _, match := range s.Matches {
			for _, entrant := range []string{match.A, match.B} {
				if !contains(entrants, entrant) {
					entrants = append(entrants, entrant)
				}
			}
			if !contains(winners, match.Winner) {
				winners = append(winners, match.Winner)
			}
		}
		remaining := []string{}
		for _, entrant := range entrants {
			if !contains(winners, entrant) {
				remaining = append(remaining, entrant)
			}
		}
		s.Round++
		s.Index = 0
		if len(winners) == 2 {
			s.Phase = "final"
			s.Podium = []string{"", "", remaining[0]}
			s.Matches = []Match{{A: winners[0], B: winners[1]}}
		} else {
			s.Phase = "silver"
			s.Podium = []string{winners[0]}
			s.Matches = []Match{{A: remaining[0], B: remaining[1]}}
		}
	case "silver":
		s.Podium = append(s.Podium, winner, loser(*m))
		s.Phase = "done"
	case "final":
		podium := []string{winner, loser(*m)}
		if len(s.Podium) > 2 {
			podium = append(podium, s.Podium[2:]...)
		}
		s.Podium = podium
		s.Phase = "done"
	default:
		var ids []string
		seen := map[string]bool{}
		for _, match := range s.Matches {
			if !seen[match.Winner] {
				ids = append(ids, match.Winner)
				seen[match.Winner] = true
			}
		}
		if len(ids) == 2 {
			var eliminated []string
			for _, match := range s.Matches {
				for _, entrant := range []string{match.A, match.B} {
					if !contains(ids, entrant) && !contains(eliminated, entrant) {
						eliminated = append(eliminated, entrant)
					}
				}
			}
			s.Finalists = ids
			s.Consolation = &State{}
			buildRound(s.Consolation, eliminated)
			s.Phase = "consolation"
			s.Round++
			s.Index = 0
			s.Matches = append([]Match{}, s.Consolation.Matches...)
		} else {
			buildRound(s, ids)
		}
	}
	return nil
}
func (a *App) start(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid request", 400)
		return
	}
	tx, err := a.db.Begin()
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback()
	tid := r.PathValue("id")
	var exists int
	if err = tx.QueryRow("SELECT 1 FROM tournaments WHERE id=?", tid).Scan(&exists); err != nil {
		http.NotFound(w, r)
		return
	}
	var raw string
	var s State
	err = tx.QueryRow("SELECT state FROM games WHERE user=? AND tournament=?", user(r), tid).Scan(&raw)
	missing := errors.Is(err, sql.ErrNoRows)
	if err != nil && !missing {
		fail(w, err)
		return
	}
	if !missing {
		if err = json.Unmarshal([]byte(raw), &s); err != nil {
			fail(w, err)
			return
		}
	}
	if missing || r.FormValue("restart") == "yes" {
		rows, err := tx.Query("SELECT id FROM items WHERE tournament=? ORDER BY position", tid)
		if err != nil {
			fail(w, err)
			return
		}
		var ids []string
		for rows.Next() {
			var item string
			if err = rows.Scan(&item); err != nil {
				rows.Close()
				fail(w, err)
				return
			}
			ids = append(ids, item)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			fail(w, err)
			return
		}
		shuffle(ids)
		s = State{Revision: s.Revision + 1, Run: id()}
		buildRound(&s, ids)
		data, _ := json.Marshal(s)
		if _, err = tx.Exec("INSERT INTO games(user,tournament,state) VALUES(?,?,?) ON CONFLICT(user,tournament) DO UPDATE SET state=excluded.state,updated=CURRENT_TIMESTAMP", user(r), tid, string(data)); err != nil {
			fail(w, err)
			return
		}
	}
	if err = tx.Commit(); err != nil {
		fail(w, err)
		return
	}
	http.Redirect(w, r, "/t/"+tid+"/play", 303)
}
func (a *App) play(w http.ResponseWriter, r *http.Request) {
	t, err := a.getTournament(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s, err := a.getState(user(r), t.ID)
	if errors.Is(err, sql.ErrNoRows) {
		http.Redirect(w, r, "/t/"+t.ID, 303)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	p := Page{Title: t.Name, Tournament: t, State: s, Owner: t.Owner != "" && t.Owner == user(r), Played: s.Index + 1, Total: len(s.Matches)}
	items := map[string]Item{}
	for _, i := range t.Items {
		items[i.ID] = i
	}
	if s.Phase == "done" {
		for _, iid := range s.Podium {
			p.Podium = append(p.Podium, items[iid])
		}
		p.Scores, p.Voters, err = a.scoreboard(t)
		if err != nil {
			fail(w, err)
			return
		}
		p.Results, err = resultsView(t, s)
		if err != nil {
			fail(w, err)
			return
		}
		a.render(w, "results", p)
		return
	}
	p.Left = items[s.Matches[s.Index].A]
	p.Right = items[s.Matches[s.Index].B]
	p.Immersive = true
	a.render(w, "play", p)
}
func (a *App) vote(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid vote", 400)
		return
	}
	tx, err := a.db.Begin()
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback()
	var raw string
	var s State
	if err = tx.QueryRow("SELECT state FROM games WHERE user=? AND tournament=?", user(r), r.PathValue("id")).Scan(&raw); err != nil {
		http.Error(w, "Start a tournament first.", 400)
		return
	}
	if err = json.Unmarshal([]byte(raw), &s); err != nil {
		fail(w, err)
		return
	}
	if r.FormValue("revision") != fmt.Sprint(s.Revision) || r.FormValue("run") != s.Run {
		http.Redirect(w, r, "/t/"+r.PathValue("id")+"/play", 303)
		return
	}
	if err = advance(&s, r.FormValue("winner")); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	match := s.History[len(s.History)-1]
	if _, err = tx.Exec("INSERT INTO votes(tournament,user,run,revision,a,b,winner) VALUES(?,?,?,?,?,?,?)", r.PathValue("id"), user(r), s.Run, s.Revision, match.A, match.B, match.Winner); err != nil {
		fail(w, err)
		return
	}
	next, _ := json.Marshal(s)
	if s.Phase == "done" {
		if _, err = tx.Exec("INSERT INTO results(tournament,user,state) VALUES(?,?,?) ON CONFLICT(tournament,user) DO UPDATE SET state=excluded.state,updated=CURRENT_TIMESTAMP", r.PathValue("id"), user(r), string(next)); err != nil {
			fail(w, err)
			return
		}
	}
	if _, err = tx.Exec("UPDATE games SET state=?,updated=CURRENT_TIMESTAMP WHERE user=? AND tournament=?", string(next), user(r), r.PathValue("id")); err != nil {
		fail(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		fail(w, err)
		return
	}
	http.Redirect(w, r, "/t/"+r.PathValue("id")+"/play", 303)
}
