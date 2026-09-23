package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestResultBracketEverySize(t *testing.T) {
	for n := 2; n <= 256; n++ {
		for trial := 0; trial < 4; trial++ {
			tt := Tournament{ID: "t"}
			ids := []string{}
			for i := 0; i < n; i++ {
				item := Item{ID: fmt.Sprint(i), Kind: "image", Media: fmt.Sprint(i)}
				tt.Items = append(tt.Items, item)
				ids = append(ids, item.ID)
			}
			s := State{Run: fmt.Sprint(trial)}
			buildRound(&s, ids)
			for s.Phase != "done" {
				m := s.Matches[s.Index]
				pick := m.A
				if (len(s.History)+trial)%3 == 0 {
					pick = m.B
				}
				if err := advance(&s, pick); err != nil {
					t.Fatal(err)
				}
			}
			view, err := resultsView(tt, s)
			if err != nil {
				t.Fatalf("n=%d trial=%d: %v", n, trial, err)
			}
			count := 0
			seen := map[string]bool{}
			nodes := map[string]BracketMatch{}
			columns := map[string]int{}
			for col, column := range view.Columns {
				for _, match := range column.Matches {
					count++
					nodes[match.ID] = match
					columns[match.ID] = col
					original := s.History[match.Number-1]
					selected := 0
					for _, entry := range match.Entries {
						seen[entry.Item.ID] = true
						if entry.Winner {
							selected++
							if entry.Item.ID != original.Winner {
								t.Fatal("wrong winner")
							}
						}
					}
					if selected != 1 {
						t.Fatal("missing selection")
					}
				}
			}
			if count != len(s.History) || len(seen) != n {
				t.Fatalf("n=%d missing items/matches", n)
			}
			for _, column := range view.Columns {
				for _, match := range column.Matches {
					for _, entry := range match.Entries {
						if entry.Source != "" {
							parent, ok := nodes[entry.Source]
							if !ok || columns[parent.ID] >= columns[match.ID] {
								t.Fatal("bad connection")
							}
							found := false
							for _, p := range parent.Entries {
								if p.Item.ID == entry.Item.ID {
									found = true
								}
							}
							if !found {
								t.Fatal("unrelated connection")
							}
						}
					}
				}
			}
			if len(view.Slides) != n || view.Slides[n-1].Item.ID != s.Podium[0] {
				t.Fatalf("n=%d bad countdown", n)
			}
			for i, slide := range view.Slides {
				if i > 0 && slide.Rank > view.Slides[i-1].Rank {
					t.Fatal("wrong countdown order")
				}
			}
			for rank, item := range s.Podium {
				if view.Slides[n-1-rank].Item.ID != item {
					t.Fatal("podium wrong")
				}
			}
			again, _ := resultsView(tt, s)
			if !reflect.DeepEqual(view.Slides, again.Slides) {
				t.Fatal("ties changed on refresh")
			}
		}
	}
}
func TestCountdownSkipsTextAndKeepsRank(t *testing.T) {
	tt := Tournament{Items: []Item{{ID: "a", Title: "A", Kind: "text"}, {ID: "b", Kind: "video", Media: "movie"}, {ID: "c", Kind: "image", Media: "photo"}, {ID: "d", Kind: "text", Title: "D"}}}
	s := State{}
	buildRound(&s, []string{"a", "b", "c", "d"})
	for _, winner := range []string{"a", "c", "b", "a"} {
		advance(&s, winner)
	}
	view, err := resultsView(tt, s)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Slides) != 2 || view.Slides[0].Item.ID != "b" || view.Slides[0].Rank != 3 || view.Slides[1].Item.ID != "c" || view.Slides[1].Rank != 2 {
		t.Fatal(view.Slides)
	}
}
func TestCompletedPageIncludesBracket(t *testing.T) {
	a := testApp(t)
	tt := fixture(t, a)
	path := "/t/" + tt.ID
	request(a, "POST", path+"/start", "", a.start)
	finish(t, a, tt, "a")
	w := request(a, "GET", path+"/play", "", a.play)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Your bracket") || strings.Contains(w.Body.String(), "Watch your countdown") {
		t.Fatal("text-only results rendered incorrectly")
	}
}
