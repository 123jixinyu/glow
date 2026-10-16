package table

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// The height of a row must match the number of lines the cell is wrapped to
// when it is rendered. detectContentHeight() used to wrap with x/ansi.Wrap,
// which disagrees with the cellbuf.Wrap used by lipgloss to render the cell:
// for this CJK cell at a content width of 24 cells it invents four empty
// lines, and the tallest row of the table then shows the same number of blank
// lines under its content.
func TestDetectContentHeightMatchesCellRender(t *testing.T) {
	content := "量达标；指数：盘中创业板/1000 破前低后收回，收盘深成/创业板/科创50 边际新低（-0.1%~-0.4%）"
	const width = 24

	r := &resizer{}
	got := r.detectContentHeight(content, width)

	want := lipgloss.Height(lipgloss.NewStyle().Inline(false).Width(width).Render(content))
	if got != want {
		t.Errorf("detectContentHeight() = %d, want %d (the rendered cell height)", got, want)
	}
}
