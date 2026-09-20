package ui

import (
	"testing"
	"time"
)

func TestSortMarkdownsByModtime(t *testing.T) {
	base := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	mds := []*markdown{
		{Note: "b.md", Modtime: base},
		{Note: "a.md", Modtime: base.Add(2 * time.Hour)},
		{Note: "c.md", Modtime: base.Add(-time.Hour)},
	}

	sortMarkdowns(mds)

	got := []string{mds[0].Note, mds[1].Note, mds[2].Note}
	want := []string{"a.md", "b.md", "c.md"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestSortMarkdownsTieBreakByName(t *testing.T) {
	modtime := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	mds := []*markdown{
		{Note: "b.md", Modtime: modtime},
		{Note: "a.md", Modtime: modtime},
	}

	sortMarkdowns(mds)

	if mds[0].Note != "a.md" || mds[1].Note != "b.md" {
		t.Fatalf("order = [%s %s], want [a.md b.md]", mds[0].Note, mds[1].Note)
	}
}
