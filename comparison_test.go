package main

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func checkPass(t *testing.T, s State, n int) {
	t.Helper()
	if len(s.Matches) != (n+1)/2 {
		t.Fatalf("n=%d: got %d matches", n, len(s.Matches))
	}
	counts := map[string]int{}
	for _, m := range s.Matches {
		if m.A == m.B || !contains(s.Entrants, m.A) || !contains(s.Entrants, m.B) {
			t.Fatalf("invalid comparison: %+v", m)
		}
		counts[m.A]++
		counts[m.B]++
	}
	repeats := 0
	for _, entrant := range s.Entrants {
		switch counts[entrant] {
		case 1:
		case 2:
			repeats++
		default:
			t.Fatalf("n=%d: entrant %s appeared %d times", n, entrant, counts[entrant])
		}
	}
	if repeats != n%2 {
		t.Fatalf("n=%d: %d repeated entrants", n, repeats)
	}
}

func TestComparisonModesAllSizes(t *testing.T) {
	for n := 2; n <= 256; n++ {
		ids := make([]string, n)
		for i := range ids {
			ids[i] = fmt.Sprint(i)
		}
		for _, mode := range []string{"normal", "endless"} {
			s := State{Mode: mode, Entrants: ids}
			buildComparisonPass(&s)
			for pass := 1; pass <= 3; pass++ {
				checkPass(t, s, n)
				before := s.Revision
				if err := advance(&s, "missing"); err == nil || s.Revision != before || s.Index != 0 {
					t.Fatal("invalid comparison vote mutated state")
				}
				matches := append([]Match(nil), s.Matches...)
				for i := range matches {
					winner := s.Matches[s.Index].A
					if i%2 == 1 {
						winner = s.Matches[s.Index].B
					}
					if err := advance(&s, winner); err != nil {
						t.Fatal(err)
					}
				}
				if s.Revision != before+len(matches) || len(s.Podium) != 0 {
					t.Fatalf("unexpected state: %+v", s)
				}
				if mode == "normal" {
					if s.Phase != "done" || len(s.History) != len(matches) || advance(&s, ids[0]) == nil {
						t.Fatal("normal mode did not finish after one pass")
					}
					break
				}
				if s.Phase != "comparison" || s.Round != pass+1 || s.Index != 0 || len(s.History) != 0 {
					t.Fatal("endless rollover failed or history grew across passes")
				}
				if n >= 4 {
					for _, old := range matches {
						for _, next := range s.Matches {
							if (old.A == next.A && old.B == next.B) || (old.A == next.B && old.B == next.A) {
								t.Fatalf("n=%d: repeated opponents across endless passes", n)
							}
						}
					}
				}
			}
		}
	}
}

func TestComparisonPersistenceAndScores(t *testing.T) {
	a := testApp(t)
	tt := fixture(t, a)
	path := "/t/" + tt.ID
	u := strings.Repeat("a", 48)
	request(a, "POST", path+"/start", "", a.start)
	finish(t, a, tt, "a")
	var originalResult string
	if err := a.db.QueryRow("SELECT state FROM results WHERE tournament=? AND user=?", tt.ID, u).Scan(&originalResult); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"normal", "endless"} {
		w := request(a, "POST", path+"/start", "restart=yes&mode="+mode, a.start)
		if w.Code != 303 {
			t.Fatal(w.Body.String())
		}
		s, err := a.getState(u, tt.ID)
		if err != nil || s.Mode != mode || s.Round != 1 {
			t.Fatalf("mode not saved: %+v %v", s, err)
		}
		for i := 0; i < 2; i++ {
			match := s.Matches[s.Index]
			body := url.Values{"run": {s.Run}, "revision": {fmt.Sprint(s.Revision)}, "winner": {s.Matches[s.Index].A}}.Encode()
			if w = request(a, "POST", path+"/vote", body, a.vote); w.Code != 303 {
				t.Fatal(w.Body.String())
			}
			next, _ := a.getState(u, tt.ID)
			var votedA, votedB, winner string
			if err := a.db.QueryRow("SELECT a,b,winner FROM votes WHERE tournament=? AND user=? AND run=? AND revision=?", tt.ID, u, s.Run, next.Revision).Scan(&votedA, &votedB, &winner); err != nil {
				t.Fatal(err)
			}
			if votedA != match.A || votedB != match.B || winner != match.A {
				t.Fatal("saved vote did not match the displayed pair at a pass boundary")
			}
			request(a, "POST", path+"/vote", body, a.vote)
			s, _ = a.getState(u, tt.ID)
			if s.Revision != next.Revision {
				t.Fatal("stale vote advanced comparison")
			}
		}
		w = request(a, "GET", path+"/play", "", a.play)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "</html>") {
			t.Fatal(w.Body.String())
		}
		if mode == "normal" && !strings.Contains(w.Body.String(), "All comparisons complete") {
			t.Fatal("missing normal completion screen")
		}
		if mode == "endless" && (s.Round != 2 || s.Index != 0) {
			t.Fatal("endless rollover not persisted")
		}
		var result string
		a.db.QueryRow("SELECT state FROM results WHERE tournament=? AND user=?", tt.ID, u).Scan(&result)
		if result != originalResult {
			t.Fatal("comparison changed completed tournament score")
		}
		// Re-running startup migrations must not backfill a normal completion.
		if _, err = a.db.Exec("DELETE FROM results WHERE tournament=?", tt.ID); err != nil {
			t.Fatal(err)
		}
		if err = migrate(a.db); err != nil {
			t.Fatal(err)
		}
		var count int
		a.db.QueryRow("SELECT count(*) FROM results WHERE tournament=?", tt.ID).Scan(&count)
		if count != 0 {
			t.Fatal("comparison backfilled into public results")
		}
		a.db.Exec("INSERT INTO results(tournament,user,state) VALUES(?,?,?)", tt.ID, u, originalResult)
	}
	before, _ := a.getState(u, tt.ID)
	if w := request(a, "POST", path+"/start", "restart=yes&mode=unknown", a.start); w.Code != 400 {
		t.Fatal("invalid mode accepted")
	}
	after, _ := a.getState(u, tt.ID)
	if before.Run != after.Run {
		t.Fatal("invalid mode replaced progress")
	}
	w := request(a, "GET", path, "", a.landing)
	for _, label := range []string{"Start as Tournament", "Start Normal", "Start Endless Mode", "Continue Endless Mode"} {
		if !strings.Contains(w.Body.String(), label) {
			t.Fatalf("missing mode control %q", label)
		}
	}
}
