package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTournamentSizes(t *testing.T) {
	for n := 2; n <= 256; n++ {
		for trial := 0; trial < 12; trial++ {
			ids := make([]string, n)
			for i := range ids {
				ids[i] = fmt.Sprint(i)
			}
			s := State{}
			buildRound(&s, ids)
			for moves := 0; s.Phase != "done"; moves++ {
				if moves > n*3 {
					t.Fatalf("tournament does not converge: n=%d %+v", n, s)
				}
				m := s.Matches[s.Index]
				if m.A == m.B {
					t.Fatal("self match")
				}
				winner := m.A
				if (moves+trial)%3 == 0 {
					winner = m.B
				}
				if err := advance(&s, winner); err != nil {
					t.Fatal(err)
				}
			}
			want := 3
			if n == 2 {
				want = 2
			}
			if len(s.Podium) < want {
				t.Fatalf("n=%d: incomplete podium: %v", n, s.Podium)
			}
			seen := map[string]bool{}
			for _, id := range s.Podium {
				if seen[id] || !contains(ids, id) {
					t.Fatalf("invalid podium: %v", s.Podium)
				}
				seen[id] = true
			}
		}
	}
}
func TestOddRoundsRepeatExactlyOne(t *testing.T) {
	for n := 3; n <= 255; n += 2 {
		ids := make([]string, n)
		for i := range ids {
			ids[i] = fmt.Sprint(i)
		}
		s := State{}
		buildRound(&s, ids)
		counts := map[string]int{}
		for _, m := range s.Matches {
			counts[m.A]++
			counts[m.B]++
		}
		doubled := 0
		for _, id := range ids {
			if counts[id] == 2 {
				doubled++
			} else if counts[id] != 1 {
				t.Fatalf("n=%d invalid count %d", n, counts[id])
			}
		}
		if doubled != 1 {
			t.Fatalf("n=%d doubled %d", n, doubled)
		}
	}
}
func TestFourPlacements(t *testing.T) {
	s := State{}
	buildRound(&s, []string{"a", "b", "c", "d"})
	for _, winner := range []string{"a", "c", "d", "c"} {
		if err := advance(&s, winner); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Join(s.Podium, ",") != "c,a,d,b" {
		t.Fatal(s.Podium)
	}
}
func TestInvalidVote(t *testing.T) {
	s := State{}
	buildRound(&s, []string{"a", "b"})
	if advance(&s, "c") == nil || s.Index != 0 {
		t.Fatal("invalid vote accepted")
	}
}
func testApp(t *testing.T) *App {
	t.Helper()
	data := t.TempDir()
	os.MkdirAll(filepath.Join(data, "uploads"), 0700)
	db, err := sql.Open("sqlite", filepath.Join(data, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`CREATE TABLE tournaments(id TEXT PRIMARY KEY,name TEXT,description TEXT,created TEXT);CREATE TABLE media(id TEXT PRIMARY KEY,owner TEXT,path TEXT,kind TEXT,claimed INTEGER DEFAULT 0);CREATE TABLE items(id TEXT PRIMARY KEY,tournament TEXT,title TEXT,media TEXT,kind TEXT,position INTEGER);CREATE TABLE games(user TEXT,tournament TEXT,state TEXT,updated TEXT,PRIMARY KEY(user,tournament));`)
	if err != nil {
		t.Fatal(err)
	}
	if err = migrate(db); err != nil {
		t.Fatal(err)
	}
	return &App{db: db, data: data, templates: template.Must(template.ParseFS(assets, "templates/*.html"))}
}
func request(a *App, method, path, body string, handler http.HandlerFunc) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.AddCookie(&http.Cookie{Name: "crown_session", Value: strings.Repeat("a", 48)})
	if method == "POST" {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	parts := strings.Split(path, "/")
	if len(parts) > 2 {
		r.SetPathValue("id", parts[2])
	}
	w := httptest.NewRecorder()
	a.middleware(handler).ServeHTTP(w, r)
	return w
}
func TestPersistenceRestartAndStaleVotes(t *testing.T) {
	a := testApp(t)
	w := request(a, "POST", "/api/tournaments", `{"name":"Food","items":[{"title":"Pizza"},{"title":"Pasta"},{"title":"Rice"},{"title":"Tacos"}]}`, a.create)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var result map[string]string
	json.Unmarshal(w.Body.Bytes(), &result)
	path := result["url"]
	tid := strings.TrimPrefix(path, "/t/")
	u := strings.Repeat("a", 48)
	request(a, "POST", path+"/start", "", a.start)
	s, _ := a.getState(u, tid)
	vote := url.Values{"run": {s.Run}, "revision": {fmt.Sprint(s.Revision)}, "winner": {s.Matches[0].A}}.Encode()
	w = request(a, "POST", path+"/vote", vote, a.vote)
	if w.Code != 303 {
		t.Fatal(w.Body.String())
	}
	saved, _ := a.getState(u, tid)
	if saved.Index != 1 {
		t.Fatal("vote not persisted")
	}
	request(a, "POST", path+"/vote", vote, a.vote)
	saved, _ = a.getState(u, tid)
	if saved.Index != 1 {
		t.Fatal("stale vote mutated state")
	}
	for saved.Phase != "done" {
		v := url.Values{"run": {saved.Run}, "revision": {fmt.Sprint(saved.Revision)}, "winner": {saved.Matches[saved.Index].A}}.Encode()
		request(a, "POST", path+"/vote", v, a.vote)
		saved, _ = a.getState(u, tid)
	}
	for _, route := range []struct {
		path    string
		handler http.HandlerFunc
	}{{"/", a.home}, {path, a.landing}, {path + "/play", a.play}, {"/new", a.newPage}} {
		w = request(a, "GET", route.path, "", route.handler)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "</html>") {
			t.Fatalf("render %s failed: %s", route.path, w.Body.String())
		}
	}
	request(a, "POST", path+"/start", "restart=yes", a.start)
	reset, _ := a.getState(u, tid)
	if reset.Index != 0 || len(reset.History) != 0 || len(reset.Podium) != 0 || reset.Revision <= saved.Revision {
		t.Fatal("restart did not replace progress")
	}
	w = request(a, "GET", path+"/play", "", a.play)
	if !strings.Contains(w.Body.String(), "Crown this one") {
		t.Fatal("missing matchup")
	}
}
func TestMediaUploadAndRange(t *testing.T) {
	a := testApp(t)
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	f, _ := mw.CreateFormFile("file", "test.png")
	f.Write([]byte("\x89PNG\r\n\x1a\n" + strings.Repeat("x", 1024)))
	mw.Close()
	r := httptest.NewRequest("POST", "/api/upload", &b)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.AddCookie(&http.Cookie{Name: "crown_session", Value: strings.Repeat("a", 48)})
	w := httptest.NewRecorder()
	a.middleware(http.HandlerFunc(a.upload)).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var result map[string]string
	json.Unmarshal(w.Body.Bytes(), &result)
	r = httptest.NewRequest("GET", "/media/"+result["id"], nil)
	r.SetPathValue("id", result["id"])
	r.AddCookie(&http.Cookie{Name: "crown_session", Value: strings.Repeat("a", 48)})
	r.Header.Set("Range", "bytes=0-7")
	w = httptest.NewRecorder()
	a.middleware(http.HandlerFunc(a.media)).ServeHTTP(w, r)
	if w.Code != 206 {
		t.Fatalf("range not supported: %d", w.Code)
	}
	data, _ := io.ReadAll(w.Result().Body)
	if string(data) != "\x89PNG\r\n\x1a\n" {
		t.Fatal("wrong media bytes")
	}
}
func TestRejectCrossOrigin(t *testing.T) {
	a := testApp(t)
	r := httptest.NewRequest("POST", "http://localhost/t/abc/start", nil)
	r.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	a.middleware(http.HandlerFunc(a.start)).ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
