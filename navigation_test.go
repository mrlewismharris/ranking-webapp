package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func navigationState(t *testing.T, a *App, tt Tournament) State {
	t.Helper()
	s, err := a.getState(strings.Repeat("a", 48), tt.ID)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func navigationBody(s State, winner string) string {
	return url.Values{"run": {s.Run}, "revision": {fmt.Sprint(s.Revision)}, "winner": {winner}}.Encode()
}

func assertRestored(t *testing.T, got, want State) {
	t.Helper()
	want.Revision = got.Revision
	want.UndoReady = true
	if !reflect.DeepEqual(got, want) {
		a, _ := json.Marshal(got)
		b, _ := json.Marshal(want)
		t.Fatalf("restored state differs:\ngot  %s\nwant %s", a, b)
	}
}

func voteCount(t *testing.T, a *App, tt Tournament, run string) int {
	t.Helper()
	var count int
	if err := a.db.QueryRow("SELECT count(*) FROM votes WHERE tournament=? AND user=? AND run=?", tt.ID, strings.Repeat("a", 48), run).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestBackRestoresEverySelection(t *testing.T) {
	for _, mode := range []string{"tournament", "normal", "endless"} {
		t.Run(mode, func(t *testing.T) {
			a := testApp(t)
			tt := fixture(t, a)
			path := "/t/" + tt.ID
			request(a, "POST", path+"/start", "mode="+mode, a.start)
			var before []State
			for i := 0; i < 6; i++ {
				s := navigationState(t, a, tt)
				if s.Phase == "done" {
					break
				}
				before = append(before, s)
				m := s.Matches[s.Index]
				if w := request(a, "POST", path+"/vote", navigationBody(s, m.A), a.vote); w.Code != 303 {
					t.Fatal(w.Body.String())
				}
			}
			w := request(a, "GET", path+"/play", "", a.play)
			if !strings.Contains(w.Body.String(), `class="undo-form"`) {
				t.Fatal("Back missing from active or completed session")
			}
			for i := len(before) - 1; i >= 0; i-- {
				current := navigationState(t, a, tt)
				body := navigationBody(current, "")
				w = request(a, "POST", path+"/back", body, a.back)
				if w.Code != 303 {
					t.Fatal(w.Body.String())
				}
				restored := navigationState(t, a, tt)
				assertRestored(t, restored, before[i])
				if restored.Revision <= current.Revision {
					t.Fatal("rewind reused a stale-page token")
				}
				// Neither a duplicate Back nor an old vote may advance this pair.
				request(a, "POST", path+"/back", body, a.back)
				old := before[i]
				request(a, "POST", path+"/vote", navigationBody(old, old.Matches[old.Index].A), a.vote)
				assertRestored(t, navigationState(t, a, tt), restored)
				if err := migrate(a.db); err != nil {
					t.Fatal(err)
				}
				if count := voteCount(t, a, tt, restored.Run); count != i {
					t.Fatalf("undone votes returned after migration: %d, want %d", count, i)
				}
			}
			first := navigationState(t, a, tt)
			w = request(a, "GET", path+"/play", "", a.play)
			if strings.Contains(w.Body.String(), `class="undo-form"`) || !strings.Contains(w.Body.String(), "&lt; Go Home") {
				t.Fatal("first pair should show Go Home and hide Back")
			}
			request(a, "POST", path+"/back", navigationBody(first, ""), a.back)
			assertRestored(t, navigationState(t, a, tt), first)
			m := first.Matches[first.Index]
			request(a, "POST", path+"/vote", navigationBody(first, m.B), a.vote)
			changed := navigationState(t, a, tt)
			if voteCount(t, a, tt, changed.Run) != 1 || changed.History[0].Winner != m.B {
				t.Fatal("replacement added a vote instead of replacing the old choice")
			}
			request(a, "POST", path+"/start", "restart=yes&mode="+mode, a.start)
			var count int
			a.db.QueryRow("SELECT count(*) FROM game_steps WHERE tournament=?", tt.ID).Scan(&count)
			if count != 0 {
				t.Fatal("restart retained obsolete navigation history")
			}
			w = request(a, "GET", path+"/play", "", a.play)
			if strings.Contains(w.Body.String(), `class="undo-form"`) {
				t.Fatal("new run can navigate into a previous run")
			}
		})
	}
}

func TestBackPreservesPreviousBallotAndBrowserIsolation(t *testing.T) {
	a := testApp(t)
	tt := fixture(t, a)
	path := "/t/" + tt.ID
	request(a, "POST", path+"/start", "", a.start)
	finish(t, a, tt, "a")
	var ballot string
	if err := a.db.QueryRow("SELECT state FROM results WHERE tournament=?", tt.ID).Scan(&ballot); err != nil {
		t.Fatal(err)
	}
	// Reopening the completed run withdraws its score until completion again.
	completed := navigationState(t, a, tt)
	request(a, "POST", path+"/back", navigationBody(completed, ""), a.back)
	_, voters, _ := a.scoreboard(tt)
	if voters != 0 {
		t.Fatal("reopened completed ballot still contributes scores")
	}
	finish(t, a, tt, "a")
	if err := a.db.QueryRow("SELECT state FROM results WHERE tournament=?", tt.ID).Scan(&ballot); err != nil {
		t.Fatal(err)
	}
	request(a, "POST", path+"/start", "restart=yes", a.start)
	s := navigationState(t, a, tt)
	request(a, "POST", path+"/vote", navigationBody(s, s.Matches[s.Index].A), a.vote)
	s = navigationState(t, a, tt)
	if w := asUser(a, "b", "POST", path+"/back", navigationBody(s, ""), a.back); w.Code != 400 {
		t.Fatal("another browser accessed this run's Back history")
	}
	assertRestored(t, navigationState(t, a, tt), s)
	request(a, "POST", path+"/back", navigationBody(s, ""), a.back)
	var retained string
	a.db.QueryRow("SELECT state FROM results WHERE tournament=?", tt.ID).Scan(&retained)
	if retained != ballot {
		t.Fatal("rewinding a fresh run removed the earlier completed ballot")
	}
}

func TestBackUpgradesExistingRuns(t *testing.T) {
	for _, mode := range []string{"tournament", "normal", "endless"} {
		for _, n := range []int{2, 3, 4, 5, 7, 9, 16} {
			t.Run(fmt.Sprintf("%s-%d", mode, n), func(t *testing.T) {
				a := testApp(t)
				in := TournamentInput{Name: "Existing run"}
				for i := 0; i < n; i++ {
					in.Items = append(in.Items, Item{Title: fmt.Sprint(i)})
				}
				body, _ := json.Marshal(in)
				w := request(a, "POST", "/api/tournaments", string(body), a.create)
				if w.Code != 200 {
					t.Fatal(w.Body.String())
				}
				var result map[string]string
				json.Unmarshal(w.Body.Bytes(), &result)
				tt, _ := a.getTournament(strings.TrimPrefix(result["url"], "/t/"))
				var ids []string
				for _, item := range tt.Items {
					ids = append(ids, item.ID)
				}
				s := State{Run: id(), Revision: 1, Mode: mode}
				if mode == "tournament" {
					buildRound(&s, ids)
				} else {
					s.Entrants = ids
					buildComparisonPass(&s)
				}
				var before []State
				for i := 0; i < n*3 && s.Phase != "done"; i++ {
					if n == 7 && i == 2 {
						break // An unfinished legacy round still has unvoted pairs to restore.
					}
					raw, _ := json.Marshal(s)
					var saved State
					json.Unmarshal(raw, &saved)
					before = append(before, saved)
					m := s.Matches[s.Index]
					pick := m.A
					if i%3 == 0 {
						pick = m.B
					}
					if err := advance(&s, pick); err != nil {
						t.Fatal(err)
					}
					if _, err := a.db.Exec("INSERT INTO votes(tournament,user,run,revision,a,b,winner) VALUES(?,?,?,?,?,?,?)", tt.ID, strings.Repeat("a", 48), s.Run, s.Revision, m.A, m.B, pick); err != nil {
						t.Fatal(err)
					}
				}
				raw, _ := json.Marshal(s)
				if _, err := a.db.Exec("INSERT INTO games(user,tournament,state) VALUES(?,?,?)", strings.Repeat("a", 48), tt.ID, string(raw)); err != nil {
					t.Fatal(err)
				}
				if n == 7 {
					// Existing runs can also be upgraded by making a new vote first.
					before = append(before, s)
					m := s.Matches[s.Index]
					w := request(a, "POST", result["url"]+"/vote", navigationBody(s, m.B), a.vote)
					if w.Code != 303 {
						t.Fatal(w.Body.String())
					}
					s = navigationState(t, a, tt)
				}
				for i := len(before) - 1; i >= 0; i-- {
					w := request(a, "POST", result["url"]+"/back", navigationBody(s, ""), a.back)
					if w.Code != 303 {
						t.Fatal(w.Body.String())
					}
					s = navigationState(t, a, tt)
					assertRestored(t, s, before[i])
				}
			})
		}
	}
}
