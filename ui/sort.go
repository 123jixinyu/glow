package ui

import (
	"slices"
	"strings"
)

// sortMarkdowns orders documents by most recently modified first. Ties are
// broken by name so the order stays stable across reloads.
func sortMarkdowns(mds []*markdown) {
	slices.SortStableFunc(mds, func(a, b *markdown) int {
		if c := a.Modtime.Compare(b.Modtime); c != 0 {
			return -c
		}
		return strings.Compare(a.Note, b.Note)
	})
}
