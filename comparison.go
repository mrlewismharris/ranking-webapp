package main

import (
	"errors"
	mathrand "math/rand/v2"
)

// A pass uses every entrant once, plus one random repeat for an odd count.
// Retry shuffled draws to minimize repeated opponents between endless passes.
func buildComparisonPass(s *State) {
	previous := map[[2]string]bool{}
	pair := func(a, b string) [2]string {
		if a > b {
			a, b = b, a
		}
		return [2]string{a, b}
	}
	for _, m := range s.Matches {
		previous[pair(m.A, m.B)] = true
	}
	bestRepeats := len(s.Entrants) + 1
	var best []Match
	for attempt := 0; attempt < 128; attempt++ {
		ids := append([]string(nil), s.Entrants...)
		shuffle(ids)
		var matches []Match
		for i := 0; i+1 < len(ids); i += 2 {
			matches = append(matches, Match{A: ids[i], B: ids[i+1]})
		}
		if len(ids)%2 == 1 {
			matches = append(matches, Match{A: ids[len(ids)-1], B: ids[mathrand.IntN(len(ids)-1)]})
		}
		repeats := 0
		for _, m := range matches {
			if previous[pair(m.A, m.B)] {
				repeats++
			}
		}
		if repeats < bestRepeats {
			best, bestRepeats = matches, repeats
		}
		// Two entrants must repeat; three must reuse at least one pair.
		if repeats == 0 || (len(ids) <= 3 && repeats == 1) {
			break
		}
	}
	s.Matches = best
	s.Index = 0
	s.Round++
	s.Phase = "comparison"
	s.History = nil // Endless sessions keep only the current pass in saved state.
}

func advanceComparison(s *State, winner string) error {
	m := &s.Matches[s.Index]
	if winner != m.A && winner != m.B {
		return errors.New("invalid contender")
	}
	m.Winner = winner
	s.History = append(s.History, *m)
	s.Index++
	s.Revision++
	if s.Index == len(s.Matches) {
		if s.Mode == "endless" {
			buildComparisonPass(s)
		} else {
			s.Phase = "done"
		}
	}
	return nil
}
