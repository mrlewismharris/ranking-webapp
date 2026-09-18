package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// Migrations preserve existing brackets. Older tournaments have no provable owner.
func migrate(db *sql.DB) error {
	rows, err := db.Query("PRAGMA table_info(tournaments)")
	if err != nil {
		return err
	}
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, kind string
		var def any
		if err = rows.Scan(&cid, &name, &kind, &notnull, &def, &pk); err != nil {
			rows.Close()
			return err
		}
		columns[name] = true
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, column := range []string{"owner", "creator"} {
		if !columns[column] {
			if _, err = db.Exec("ALTER TABLE tournaments ADD COLUMN " + column + " TEXT NOT NULL DEFAULT ''"); err != nil {
				return err
			}
		}
	}
	if !columns["version"] {
		if _, err = db.Exec("ALTER TABLE tournaments ADD COLUMN version INTEGER NOT NULL DEFAULT 1"); err != nil {
			return err
		}
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS votes(tournament TEXT NOT NULL REFERENCES tournaments(id),user TEXT NOT NULL,run TEXT NOT NULL,revision INTEGER NOT NULL,a TEXT NOT NULL,b TEXT NOT NULL,winner TEXT NOT NULL,created TEXT DEFAULT CURRENT_TIMESTAMP,PRIMARY KEY(tournament,user,run,revision));
 CREATE TABLE IF NOT EXISTS results(tournament TEXT NOT NULL REFERENCES tournaments(id),user TEXT NOT NULL,state TEXT NOT NULL,updated TEXT DEFAULT CURRENT_TIMESTAMP,PRIMARY KEY(tournament,user));
 INSERT OR IGNORE INTO votes(tournament,user,run,revision,a,b,winner)
 SELECT g.tournament,g.user,coalesce(json_extract(g.state,'$.Run'),''),json_extract(g.state,'$.Revision')-json_array_length(g.state,'$.History')+CAST(h.key AS INTEGER)+1,json_extract(h.value,'$.A'),json_extract(h.value,'$.B'),json_extract(h.value,'$.Winner') FROM games g,json_each(g.state,'$.History') h;
 INSERT OR IGNORE INTO results(tournament,user,state) SELECT tournament,user,state FROM games WHERE json_extract(state,'$.Phase')='done';`)
	return err
}

type TournamentInput struct {
	Name        string
	Description string
	Creator     string
	Items       []Item
	Version     int
	Reset       bool
}

func readInput(w http.ResponseWriter, r *http.Request) (TournamentInput, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var in TournamentInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "Invalid tournament.", 400)
		return in, false
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Creator = strings.TrimSpace(in.Creator)
	if in.Name == "" || len(in.Name) > 120 || len(in.Creator) > 80 || len(in.Description) > 1000 || len(in.Items) < 2 || len(in.Items) > 256 {
		http.Error(w, "Use a name up to 120 characters, creator up to 80 characters, and 2–256 contenders.", 400)
		return in, false
	}
	for i := range in.Items {
		in.Items[i].Title = strings.TrimSpace(in.Items[i].Title)
		if len(in.Items[i].Title) > 160 || (in.Items[i].Title == "" && in.Items[i].Media == "") {
			http.Error(w, "Every contender needs a title or media; titles can be up to 160 characters.", 400)
			return in, false
		}
	}
	return in, true
}
func (a *App) editPage(w http.ResponseWriter, r *http.Request) {
	t, err := a.getTournament(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if t.Owner == "" || t.Owner != user(r) {
		http.Error(w, "Only the creator’s original browser can edit this tournament.", 403)
		return
	}
	a.render(w, "new", Page{Title: "Edit " + t.Name, Tournament: t, Editing: true, Owner: true})
}
func (a *App) edit(w http.ResponseWriter, r *http.Request) {
	in, ok := readInput(w, r)
	if !ok {
		return
	}
	tx, err := a.db.Begin()
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback()
	tid := r.PathValue("id")
	var owner string
	var version int
	if err = tx.QueryRow("SELECT owner,version FROM tournaments WHERE id=?", tid).Scan(&owner, &version); err != nil {
		http.NotFound(w, r)
		return
	}
	if owner == "" || owner != user(r) {
		http.Error(w, "Only the creator’s original browser can edit this tournament.", 403)
		return
	}
	if in.Version != version {
		http.Error(w, "This tournament was edited in another tab. Reload before saving.", 409)
		return
	}
	rows, err := tx.Query("SELECT id,title,media,kind FROM items WHERE tournament=? ORDER BY position", tid)
	if err != nil {
		fail(w, err)
		return
	}
	var old []Item
	byID := map[string]Item{}
	for rows.Next() {
		var i Item
		if err = rows.Scan(&i.ID, &i.Title, &i.Media, &i.Kind); err != nil {
			rows.Close()
			fail(w, err)
			return
		}
		old = append(old, i)
		byID[i.ID] = i
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		fail(w, err)
		return
	}
	changed := len(old) != len(in.Items)
	seen := map[string]bool{}
	for pos, i := range in.Items {
		if i.ID != "" {
			if _, ok := byID[i.ID]; !ok || seen[i.ID] {
				http.Error(w, "Invalid contender ID.", 400)
				return
			}
			seen[i.ID] = true
		}
		if pos >= len(old) || i.ID != old[pos].ID || i.Title != old[pos].Title || i.Media != old[pos].Media {
			changed = true
		}
	}
	if changed && !in.Reset {
		http.Error(w, "Changing contenders resets all saved games and public scores. Tick the reset acknowledgement to save.", 409)
		return
	}
	// Resolve media before deleting old items, accepting only this tournament's files or unclaimed uploads owned by its creator.
	for pos := range in.Items {
		i := &in.Items[pos]
		i.Kind = "text"
		if i.Media != "" {
			var count int
			err = tx.QueryRow("SELECT kind FROM media WHERE id=? AND ((owner=? AND claimed=0) OR id IN (SELECT media FROM items WHERE tournament=?))", i.Media, user(r), tid).Scan(&i.Kind)
			if err != nil {
				http.Error(w, "An upload is unavailable. Upload it again.", 400)
				return
			}
			for j := 0; j < pos; j++ {
				if in.Items[j].Media == i.Media {
					count++
				}
			}
			if count > 0 {
				http.Error(w, "Use each uploaded file only once.", 400)
				return
			}
			if _, err = tx.Exec("UPDATE media SET claimed=1 WHERE id=?", i.Media); err != nil {
				fail(w, err)
				return
			}
		}
		if i.ID == "" {
			i.ID = id()
		}
	}
	if changed {
		for _, table := range []string{"games", "votes", "results", "items"} {
			if _, err = tx.Exec("DELETE FROM "+table+" WHERE tournament=?", tid); err != nil {
				fail(w, err)
				return
			}
		}
		for pos, i := range in.Items {
			if _, err = tx.Exec("INSERT INTO items(id,tournament,title,media,kind,position) VALUES(?,?,?,?,?,?)", i.ID, tid, i.Title, i.Media, i.Kind, pos); err != nil {
				fail(w, err)
				return
			}
		}
	}
	if _, err = tx.Exec("UPDATE tournaments SET name=?,description=?,creator=?,version=version+1 WHERE id=?", in.Name, in.Description, in.Creator, tid); err != nil {
		fail(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": "/t/" + tid})
}
func (a *App) deleteTournament(w http.ResponseWriter, r *http.Request) {
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
	var owner string
	var version int
	if err = tx.QueryRow("SELECT owner,version FROM tournaments WHERE id=?", tid).Scan(&owner, &version); err != nil {
		http.NotFound(w, r)
		return
	}
	if owner == "" || owner != user(r) {
		http.Error(w, "Only the creator’s original browser can delete this tournament.", 403)
		return
	}
	if r.FormValue("confirm") != "delete" || r.FormValue("version") != fmt.Sprint(version) {
		http.Error(w, "Confirm deletion on the current edit page.", 409)
		return
	}
	// Files remain on disk for now, but removed tournament media is no longer publicly served.
	if _, err = tx.Exec("UPDATE media SET claimed=0 WHERE id IN (SELECT media FROM items WHERE tournament=?)", tid); err != nil {
		fail(w, err)
		return
	}
	for _, table := range []string{"games", "votes", "results", "items"} {
		if _, err = tx.Exec("DELETE FROM "+table+" WHERE tournament=?", tid); err != nil {
			fail(w, err)
			return
		}
	}
	if _, err = tx.Exec("DELETE FROM tournaments WHERE id=?", tid); err != nil {
		fail(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		fail(w, err)
		return
	}
	http.Redirect(w, r, "/", 303)
}

type Score struct {
	Item      Item
	Rank      int
	Points    int
	Wins      int
	Matches   int
	Champions int
	Rate      string
}

func scoreRun(s State) map[string]int {
	points := map[string]int{}
	base := 0
	for _, m := range s.History {
		points[m.Winner]++
		if points[m.Winner] > base {
			base = points[m.Winner]
		}
	}
	for rank, item := range s.Podium {
		points[item] = base + 4 - rank
	}
	return points
}
func (a *App) scoreboard(t Tournament) ([]Score, int, error) {
	rows, err := a.db.Query("SELECT state FROM results WHERE tournament=?", t.ID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	scores := make([]Score, len(t.Items))
	byID := map[string]int{}
	for i, item := range t.Items {
		scores[i].Item = item
		byID[item.ID] = i
	}
	voters := 0
	for rows.Next() {
		var raw string
		var s State
		if err = rows.Scan(&raw); err != nil {
			return nil, 0, err
		}
		if err = json.Unmarshal([]byte(raw), &s); err != nil {
			return nil, 0, err
		}
		voters++
		for id, points := range scoreRun(s) {
			if i, ok := byID[id]; ok {
				scores[i].Points += points
			}
		}
		for _, m := range s.History {
			for _, id := range []string{m.A, m.B} {
				if i, ok := byID[id]; ok {
					scores[i].Matches++
				}
			}
			if i, ok := byID[m.Winner]; ok {
				scores[i].Wins++
			}
		}
		if len(s.Podium) > 0 {
			if i, ok := byID[s.Podium[0]]; ok {
				scores[i].Champions++
			}
		}
	}
	if err = rows.Err(); err != nil {
		return nil, 0, err
	}
	sort.SliceStable(scores, func(i, j int) bool { return scores[i].Points > scores[j].Points })
	for i := range scores {
		scores[i].Rank = i + 1
		if i > 0 && scores[i].Points == scores[i-1].Points {
			scores[i].Rank = scores[i-1].Rank
		}
		scores[i].Rate = "—"
		if scores[i].Matches > 0 {
			scores[i].Rate = fmt.Sprintf("%.0f%%", 100*float64(scores[i].Wins)/float64(scores[i].Matches))
		}
	}
	return scores, voters, nil
}
