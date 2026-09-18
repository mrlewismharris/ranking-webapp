package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func asUser(a *App, u, method, path, body string, h http.HandlerFunc) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.AddCookie(&http.Cookie{Name: "crown_session", Value: strings.Repeat(u, 48)})
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.SetPathValue("id", strings.Split(path, "/")[2])
	w := httptest.NewRecorder()
	a.middleware(h).ServeHTTP(w, r)
	return w
}
func fixture(t *testing.T, a *App) Tournament {
	t.Helper()
	w := request(a, "POST", "/api/tournaments", `{"name":"Test","creator":"Paige","items":[{"title":"A"},{"title":"B"},{"title":"C"},{"title":"D"}]}`, a.create)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var v map[string]string
	json.Unmarshal(w.Body.Bytes(), &v)
	tt, err := a.getTournament(strings.TrimPrefix(v["url"], "/t/"))
	if err != nil {
		t.Fatal(err)
	}
	return tt
}
func finish(t *testing.T, a *App, tt Tournament, u string) {
	t.Helper()
	path := "/t/" + tt.ID
	for i := 0; i < 20; i++ {
		s, err := a.getState(strings.Repeat(u, 48), tt.ID)
		if err != nil {
			t.Fatal(err)
		}
		if s.Phase == "done" {
			return
		}
		body := url.Values{"run": {s.Run}, "revision": {fmt.Sprint(s.Revision)}, "winner": {s.Matches[s.Index].A}}.Encode()
		w := asUser(a, u, "POST", path+"/vote", body, a.vote)
		if w.Code != 303 {
			t.Fatal(w.Body.String())
		}
	}
	t.Fatal("did not finish")
}
func editBody(tt Tournament, reset bool) string {
	b, _ := json.Marshal(TournamentInput{Name: tt.Name, Description: tt.Description, Creator: tt.Creator, Items: tt.Items, Version: tt.Version, Reset: reset})
	return string(b)
}
func TestOwnerEditingAndDeletion(t *testing.T) {
	a := testApp(t)
	tt := fixture(t, a)
	path := "/t/" + tt.ID
	if tt.Owner != strings.Repeat("a", 48) || tt.Creator != "Paige" {
		t.Fatal("creator not recorded")
	}
	for _, r := range []struct {
		method, suffix, body string
		h                    http.HandlerFunc
	}{{"GET", "/edit", "", a.editPage}, {"POST", "/edit", editBody(tt, false), a.edit}, {"POST", "/delete", "confirm=delete&version=1", a.deleteTournament}} {
		w := asUser(a, "b", r.method, path+r.suffix, r.body, r.h)
		if w.Code != 403 {
			t.Fatalf("non-owner %s: %d", r.suffix, w.Code)
		}
	}
	w := request(a, "GET", path+"/edit", "", a.editPage)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Save changes") {
		t.Fatal("edit page missing")
	}
	request(a, "POST", path+"/start", "", a.start)
	finish(t, a, tt, "a")
	tt.Name = "Renamed"
	w = request(a, "POST", path+"/edit", editBody(tt, false), a.edit)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	saved, _ := a.getState(strings.Repeat("a", 48), tt.ID)
	if saved.Phase != "done" {
		t.Fatal("metadata edit reset game")
	}
	if w = request(a, "POST", path+"/edit", editBody(tt, false), a.edit); w.Code != 409 {
		t.Fatal("stale edit accepted")
	}
	tt, _ = a.getTournament(tt.ID)
	tt.Items[0].Title = "Changed"
	if w = request(a, "POST", path+"/edit", editBody(tt, false), a.edit); w.Code != 409 {
		t.Fatal("missing reset acknowledged")
	}
	if w = request(a, "POST", path+"/edit", editBody(tt, true), a.edit); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	for _, table := range []string{"games", "results", "votes"} {
		var n int
		a.db.QueryRow("SELECT count(*) FROM "+table+" WHERE tournament=?", tt.ID).Scan(&n)
		if n != 0 {
			t.Fatalf("%s not reset", table)
		}
	}
	request(a, "POST", path+"/start", "", a.start)
	current, _ := a.getState(strings.Repeat("a", 48), tt.ID)
	stale := url.Values{"run": {saved.Run}, "revision": {fmt.Sprint(current.Revision)}, "winner": {current.Matches[0].A}}.Encode()
	request(a, "POST", path+"/vote", stale, a.vote)
	current, _ = a.getState(strings.Repeat("a", 48), tt.ID)
	if current.Index != 0 {
		t.Fatal("old run vote accepted")
	}
	tt, _ = a.getTournament(tt.ID)
	if w = request(a, "POST", path+"/delete", "version="+fmt.Sprint(tt.Version), a.deleteTournament); w.Code != 409 {
		t.Fatal("unconfirmed deletion accepted")
	}
	w = request(a, "POST", path+"/delete", "confirm=delete&version="+fmt.Sprint(tt.Version), a.deleteTournament)
	if w.Code != 303 {
		t.Fatal(w.Body.String())
	}
	if _, err := a.getTournament(tt.ID); err == nil {
		t.Fatal("not deleted")
	}
}
func TestPublicBallotsAndVoteTracking(t *testing.T) {
	a := testApp(t)
	tt := fixture(t, a)
	path := "/t/" + tt.ID
	for _, u := range []string{"a", "b"} {
		asUser(a, u, "POST", path+"/start", "", a.start)
		finish(t, a, tt, u)
	}
	scores, voters, err := a.scoreboard(tt)
	if err != nil || voters != 2 || len(scores) != 4 {
		t.Fatalf("bad scoreboard %d %v", voters, err)
	}
	var votes, users int
	a.db.QueryRow("SELECT count(*),count(DISTINCT user) FROM votes WHERE tournament=?", tt.ID).Scan(&votes, &users)
	if votes != 8 || users != 2 {
		t.Fatalf("votes=%d users=%d", votes, users)
	}
	request(a, "POST", path+"/start", "restart=yes", a.start)
	_, voters, _ = a.scoreboard(tt)
	if voters != 2 {
		t.Fatal("restart discarded public ballot")
	}
	finish(t, a, tt, "a")
	_, voters, _ = a.scoreboard(tt)
	if voters != 2 {
		t.Fatal("repeat run stuffed ballot box")
	}
	asUser(a, "c", "POST", path+"/start", "", a.start)
	_, voters, _ = a.scoreboard(tt)
	if voters != 2 {
		t.Fatal("unfinished run counted")
	}
	s, _ := a.getState(strings.Repeat("a", 48), tt.ID)
	points := scoreRun(s)
	for i := 1; i < len(s.Podium); i++ {
		if points[s.Podium[i-1]] <= points[s.Podium[i]] {
			t.Fatal("podium order not reflected in points")
		}
	}
	w := request(a, "GET", path+"/play", "", a.play)
	if !strings.Contains(w.Body.String(), "How everyone voted") {
		t.Fatal("scoreboard missing from results")
	}
	if err = migrate(a.db); err != nil {
		t.Fatal(err)
	}
	_, voters, _ = a.scoreboard(tt)
	if voters != 2 {
		t.Fatal("migration changed ballot count")
	}
}
func TestZeroScoreContendersAndTies(t *testing.T) {
	a := testApp(t)
	tt := fixture(t, a)
	s := State{Phase: "done", History: []Match{{A: tt.Items[0].ID, B: tt.Items[1].ID, Winner: tt.Items[0].ID}}, Podium: []string{tt.Items[0].ID, tt.Items[1].ID}}
	raw, _ := json.Marshal(s)
	a.db.Exec("INSERT INTO results(tournament,user,state) VALUES(?,?,?)", tt.ID, "person", string(raw))
	scores, _, err := a.scoreboard(tt)
	if err != nil {
		t.Fatal(err)
	}
	if scores[2].Points != 0 || scores[3].Points != 0 || scores[2].Rank != scores[3].Rank {
		t.Fatal("zero score entries or ties missing")
	}
}
