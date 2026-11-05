package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// Upgrade old runs on demand using their recorded pairs, including earlier
// Endless passes. New runs save their exact pre-vote state with each vote.
func prepareUndo(tx *sql.Tx, tid, u string, current *State) error {
	if current.UndoReady {
		return nil
	}
	rows, err := tx.Query("SELECT revision,a,b,winner FROM votes WHERE tournament=? AND user=? AND run=? ORDER BY revision", tid, u, current.Run)
	if err != nil {
		return err
	}
	type recordedVote struct {
		revision int
		match    Match
	}
	var votes []recordedVote
	var pairs []Match
	for rows.Next() {
		var v recordedVote
		if err = rows.Scan(&v.revision, &v.match.A, &v.match.B, &v.match.Winner); err != nil {
			rows.Close()
			return err
		}
		votes = append(votes, v)
		pairs = append(pairs, v.match)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	current.UndoReady = true
	if len(votes) == 0 {
		return nil
	}
	if current.Phase != "done" {
		pairs = append(pairs, current.Matches[current.Index:]...)
	}
	ids := append([]string(nil), current.Entrants...)
	if len(ids) == 0 {
		rows, err = tx.Query("SELECT id FROM items WHERE tournament=? ORDER BY position", tid)
		if err != nil {
			return err
		}
		for rows.Next() {
			var entrant string
			if err = rows.Scan(&entrant); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, entrant)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	if len(ids) < 2 {
		return errors.New("cannot restore a run without contenders")
	}
	replay := State{Mode: current.Mode, Entrants: current.Entrants, Run: current.Run, UndoReady: true}
	if current.Mode == "normal" || current.Mode == "endless" {
		replay.Entrants = ids
		buildComparisonPass(&replay)
		if current.Mode == "endless" {
			// Older installations may only have records for the current pass.
			replay.Round = current.Round - (len(votes)-current.Index)/len(replay.Matches)
		}
	} else {
		buildRound(&replay, ids)
	}
	for index, vote := range votes {
		if replay.Phase == "done" || replay.Index >= len(replay.Matches) {
			return errors.New("cannot restore inconsistent vote history")
		}
		// Restore all known pairs in this stage before taking the snapshot;
		// rebuilding a random round must not change the pair shown by Back.
		for j := replay.Index; j < len(replay.Matches) && index+j-replay.Index < len(pairs); j++ {
			match := pairs[index+j-replay.Index]
			match.Winner = ""
			replay.Matches[j] = match
			if replay.Consolation != nil {
				replay.Consolation.Matches[j] = match
			}
		}
		replay.Revision = vote.revision - 1
		before, err := json.Marshal(replay)
		if err != nil {
			return err
		}
		if _, err = tx.Exec("INSERT INTO game_steps(tournament,user,run,revision,state) VALUES(?,?,?,?,?)", tid, u, current.Run, vote.revision, string(before)); err != nil {
			return err
		}
		if err = advance(&replay, vote.match.Winner); err != nil {
			return fmt.Errorf("restore vote history: %w", err)
		}
	}
	return nil
}

func (a *App) back(w http.ResponseWriter, r *http.Request) {
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
	tid, u := r.PathValue("id"), user(r)
	redirect := func() { http.Redirect(w, r, "/t/"+tid+"/play", 303) }
	var raw string
	var current State
	if err = tx.QueryRow("SELECT state FROM games WHERE tournament=? AND user=?", tid, u).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Start a session first.", 400)
		} else {
			fail(w, err)
		}
		return
	}
	if err = json.Unmarshal([]byte(raw), &current); err != nil {
		fail(w, err)
		return
	}
	if r.FormValue("run") != current.Run || r.FormValue("revision") != fmt.Sprint(current.Revision) {
		redirect()
		return
	}
	if err = prepareUndo(tx, tid, u, &current); err != nil {
		fail(w, err)
		return
	}
	var revision int
	err = tx.QueryRow("SELECT revision,state FROM game_steps WHERE tournament=? AND user=? AND run=? ORDER BY revision DESC LIMIT 1", tid, u, current.Run).Scan(&revision, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		redirect()
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	var previous State
	if err = json.Unmarshal([]byte(raw), &previous); err != nil {
		fail(w, err)
		return
	}
	// Revision is a monotonic stale-page token, independent of the vote count.
	previous.Revision = current.Revision + 1
	previous.UndoReady = true
	for _, table := range []string{"votes", "game_steps"} {
		if _, err = tx.Exec("DELETE FROM "+table+" WHERE tournament=? AND user=? AND run=? AND revision>=?", tid, u, current.Run, revision); err != nil {
			fail(w, err)
			return
		}
	}
	// Reopening this completed run withdraws its ballot until it is completed
	// again; an earlier completed run is retained while replaying a newer run.
	if _, err = tx.Exec("DELETE FROM results WHERE tournament=? AND user=? AND coalesce(json_extract(state,'$.Run'),'')=?", tid, u, current.Run); err != nil {
		fail(w, err)
		return
	}
	next, err := json.Marshal(previous)
	if err != nil {
		fail(w, err)
		return
	}
	if _, err = tx.Exec("UPDATE games SET state=?,updated=CURRENT_TIMESTAMP WHERE tournament=? AND user=?", string(next), tid, u); err != nil {
		fail(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		fail(w, err)
		return
	}
	redirect()
}
