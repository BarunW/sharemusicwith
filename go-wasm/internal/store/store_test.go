package store

import "testing"

// The discover API validates section names against its own allowlist and then
// passes them here; this guards that the store has an ORDER BY column for each.
func TestRankingColumns(t *testing.T) {
	for _, s := range []string{"trending", "today", "popular", "new"} {
		if rankingColumns[s] == "" {
			t.Errorf("rankingColumns missing column for section %q", s)
		}
	}
}
