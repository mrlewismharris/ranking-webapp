package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
)

type BracketEntry struct {
	Item   Item
	Label  string
	Winner bool
	Source string
}
type BracketMatch struct {
	ID      string
	Number  int
	Label   string
	Entries []BracketEntry
}
type BracketColumn struct {
	Label   string
	Matches []BracketMatch
}
type Slide struct {
	Item      Item
	Label     string
	Placement string
	Rank      int
}
type ResultsView struct {
	Columns []BracketColumn
	Slides  []Slide
}

func itemLabel(item Item, position int) string {
	if item.Title != "" {
		return item.Title
	}
	if item.Kind == "video" {
		return fmt.Sprintf("Video %d", position+1)
	}
	return fmt.Sprintf("Image %d", position+1)
}
func stageLabel(s State) string {
	if s.Phase == "consolation" {
		return "Bronze playoff · " + stageLabel(*s.Consolation)
	}
	switch s.Phase {
	case "semifinal":
		return "Semifinal"
	case "three":
		return "Final three"
	case "bronze":
		return "Third-place match"
	case "silver":
		return "Second-place match"
	case "final":
		return "Championship final"
	}
	return fmt.Sprintf("Round %d", s.Round)
}

// Replay only derives round boundaries; the saved pairs overwrite generated pairs.
// This also supports completed games created before bracket views were introduced.
func resultsView(t Tournament, finished State) (ResultsView, error) {
	view := ResultsView{}
	if finished.Phase != "done" {
		return view, nil
	}
	items := map[string]Item{}
	labels := map[string]string{}
	var ids []string
	for pos, item := range t.Items {
		items[item.ID] = item
		labels[item.ID] = itemLabel(item, pos)
		ids = append(ids, item.ID)
	}
	if len(ids) < 2 {
		return view, fmt.Errorf("tournament requires at least two contenders")
	}
	replay := State{}
	buildRound(&replay, ids)
	type source struct {
		id     string
		column int
	}
	sources := map[string]source{}
	pending := map[string]source{}
	winners := map[string]source{}
	lastStage := map[string]int{}
	stage := -1
	for index, match := range finished.History {
		if replay.Phase == "done" || replay.Index >= len(replay.Matches) {
			return view, fmt.Errorf("unexpected match %d", index)
		}
		if replay.Index == 0 {
			for item, ref := range pending {
				sources[item] = ref
			}
			for item, ref := range winners {
				sources[item] = ref
			}
			pending = map[string]source{}
			winners = map[string]source{}
			stage++
		}
		column := 0
		for _, item := range []string{match.A, match.B} {
			if _, ok := items[item]; !ok {
				return view, fmt.Errorf("unknown contender in match")
			}
			if ref, ok := sources[item]; ok && ref.column+1 > column {
				column = ref.column + 1
			}
		}
		node := BracketMatch{ID: fmt.Sprintf("match-%d", index+1), Number: index + 1, Label: stageLabel(replay)}
		for _, item := range []string{match.A, match.B} {
			node.Entries = append(node.Entries, BracketEntry{Item: items[item], Label: labels[item], Winner: item == match.Winner, Source: sources[item].id})
			lastStage[item] = stage
			pending[item] = source{node.ID, column}
		}
		winners[match.Winner] = source{node.ID, column}
		for len(view.Columns) <= column {
			view.Columns = append(view.Columns, BracketColumn{Label: fmt.Sprintf("Stage %d", len(view.Columns)+1)})
		}
		view.Columns[column].Matches = append(view.Columns[column].Matches, node)
		replay.Matches[replay.Index] = match
		if replay.Consolation != nil {
			replay.Consolation.Matches[replay.Consolation.Index] = match
		}
		if err := advance(&replay, match.Winner); err != nil {
			return view, err
		}
	}
	if replay.Phase != "done" {
		return view, fmt.Errorf("incomplete match history")
	}
	for index, column := range view.Columns {
		label := column.Matches[0].Label
		for _, match := range column.Matches {
			if match.Label != label {
				label = "Placement matches"
				break
			}
		}
		view.Columns[index].Label = label
	}
	// Lower elimination stages come first. Equal placements use a stable shuffle
	// derived from this run, so refresh/back/forward never reorders the slideshow.
	podium := map[string]int{}
	for index, item := range finished.Podium {
		podium[item] = index + 1
	}
	raw, _ := json.Marshal(finished.History)
	seed := t.ID + finished.Run + string(raw)
	tieKey := map[string]string{}
	for _, item := range ids {
		sum := sha256.Sum256([]byte(seed + item))
		tieKey[item] = fmt.Sprintf("%x", sum)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := ids[i], ids[j]
		pa, pb := podium[a], podium[b]
		if pa != 0 || pb != 0 {
			if pa == 0 {
				return true
			}
			if pb == 0 {
				return false
			}
			return pa > pb
		}
		if lastStage[a] != lastStage[b] {
			return lastStage[a] < lastStage[b]
		}
		return tieKey[a] < tieKey[b]
	})
	for start := 0; start < len(ids); {
		end := start + 1
		if podium[ids[start]] == 0 {
			for end < len(ids) && podium[ids[end]] == 0 && lastStage[ids[start]] == lastStage[ids[end]] {
				end++
			}
		}
		rank := len(ids) - end + 1
		if p := podium[ids[start]]; p != 0 {
			rank = p
		}
		placement := fmt.Sprintf("#%d", rank)
		if end-start > 1 {
			placement = fmt.Sprintf("Tied #%d", rank)
		}
		if rank == 1 {
			placement = "♛ Champion"
		}
		for _, item := range ids[start:end] {
			if items[item].Media != "" && (items[item].Kind == "image" || items[item].Kind == "video") {
				view.Slides = append(view.Slides, Slide{Item: items[item], Label: labels[item], Placement: placement, Rank: rank})
			}
		}
		start = end
	}
	return view, nil
}
