package ansi

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

func renderMarkdown(t *testing.T, in string) string {
	t.Helper()

	b, err := os.ReadFile("../styles/dark.json")
	if err != nil {
		t.Fatal(err)
	}

	options := Options{
		WordWrap:     100,
		ColorProfile: termenv.TrueColor,
	}
	if err := json.Unmarshal(b, &options.Styles); err != nil {
		t.Fatal(err)
	}

	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.DefinitionList),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)
	ar := NewRenderer(options)
	md.SetRenderer(renderer.NewRenderer(renderer.WithNodeRenderers(util.Prioritized(ar, 1000))))

	var buf bytes.Buffer
	if err := md.Convert([]byte(in), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// tableWidths returns the widths of the columns of every table in the
// rendered output, using the row separator lines.
func tableWidths(out string) [][]int {
	var tables [][]int
	for _, line := range strings.Split(out, "\n") {
		line = xansi.Strip(line)
		if !strings.Contains(line, "─") {
			continue
		}
		parts := strings.FieldsFunc(strings.TrimSpace(line), func(r rune) bool {
			return r == '┼' || r == '┌' || r == '┐' || r == '└' || r == '┘' ||
				r == '├' || r == '┤' || r == '┬' || r == '┴'
		})
		widths := make([]int, 0, len(parts))
		for _, p := range parts {
			widths = append(widths, xansi.StringWidth(p))
		}
		if len(widths) > 0 {
			tables = append(tables, widths)
		}
	}
	return tables
}

func TestTableColumnWidthsConsistent(t *testing.T) {
	in := `| 时间 | 来源 | 内容 |
| --- | --- | --- |
| 09-20 18:00 | 金十 | 短内容 |
| 09-20 17:32 | 同花顺+金十 | 这是一条比较长的新闻内容，用来撑开内容列 |

| 时间 | 来源 | 内容 |
| --- | --- | --- |
| 09-20 17:31 | 同花顺 | 另一张表的内容，长度与上一张不同但表头一致 |
| 09-20 17:15 | 金十 | 短 |
`
	out := renderMarkdown(t, in)
	widths := tableWidths(out)
	if len(widths) < 2 {
		t.Fatalf("expected at least two tables, got %d in %q", len(widths), out)
	}
	first, second := widths[0], widths[len(widths)/2]
	if len(first) != 3 || len(second) != 3 {
		t.Fatalf("expected 3 columns per table, got %v and %v", first, second)
	}
	if first[0] != second[0] || first[1] != second[1] || first[2] != second[2] {
		t.Errorf("tables with the same headers have different widths: %v vs %v", first, second)
	}
}

func TestTableDifferentHeadersDoNotShareLayout(t *testing.T) {
	in := `| 时间 | 来源 | 内容 |
| --- | --- | --- |
| 09-20 18:00 | 金十 | 一条比较长的中文内容，用来测试表格列宽 |

| A | B |
| --- | --- |
| 1 | 2 |

| 时间 | 来源 | 内容 |
| --- | --- | --- |
| 09-20 17:31 | 同花顺 | 另一条内容 |
`
	out := renderMarkdown(t, in)
	widths := tableWidths(out)
	if len(widths) < 3 {
		t.Fatalf("expected three tables, got %v", widths)
	}

	first, middle, last := widths[0], widths[1], widths[2]
	if len(first) != 3 || len(middle) != 2 || len(last) != 3 {
		t.Fatalf("unexpected column counts: %v, %v, %v", first, middle, last)
	}

	// Tables sharing headers must match.
	if first[0] != last[0] || first[1] != last[1] || first[2] != last[2] {
		t.Errorf("tables with the same headers differ: %v vs %v", first, last)
	}

	// The A/B table must keep its own balanced layout instead of inheriting
	// the widths of the neighboring table.
	if middle[0] == first[0] && middle[1] == first[1] {
		t.Errorf("table with different headers inherited layout %v", first)
	}
}

func TestTableColumnWidthsShrink(t *testing.T) {
	in := `| 时间 | 来源 | 内容 |
| --- | --- | --- |
| 09-20 18:00 | 同花顺+金十 | 一段很长很长的中文内容，需要在窄终端里被折行显示，不能溢出。 |
`
	for limit := 30; limit <= 60; limit += 10 {
		b, err := os.ReadFile("../styles/dark.json")
		if err != nil {
			t.Fatal(err)
		}
		options := Options{WordWrap: limit, ColorProfile: termenv.TrueColor}
		if err := json.Unmarshal(b, &options.Styles); err != nil {
			t.Fatal(err)
		}
		md := goldmark.New(goldmark.WithExtensions(extension.GFM, extension.DefinitionList))
		ar := NewRenderer(options)
		md.SetRenderer(renderer.NewRenderer(renderer.WithNodeRenderers(util.Prioritized(ar, 1000))))
		var buf bytes.Buffer
		if err := md.Convert([]byte(in), &buf); err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(buf.String(), "\n") {
			if w := xansi.StringWidth(line); w > limit {
				t.Errorf("limit %d: line width %d exceeds limit: %q", limit, w, line)
			}
		}
	}
}
