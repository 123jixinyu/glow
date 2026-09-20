package ansi

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/muesli/reflow/indent"
	astext "github.com/yuin/goldmark/extension/ast"
)

// A TableElement is used to render tables.
type TableElement struct {
	lipgloss *table.Table
	table    *astext.Table
	header   []string
	row      []string
	source   []byte

	// pinned holds the column widths of the first table with the current
	// headers. Later tables reuse them so equivalent tables line up across
	// the document.
	pinned     []int
	tableWidth int
	headerSig  string
	layouts    map[string]tableColumnLayout

	tableImages []tableLink
	tableLinks  []tableLink
}

// tableColumnLayout is the column layout shared by every table with the same
// headers rendered at the same width.
type tableColumnLayout struct {
	widths []int
}

// A TableRowElement is used to render a single row in a table.
type TableRowElement struct{}

// A TableHeadElement is used to render a table's head element.
type TableHeadElement struct{}

// A TableCellElement is used to render a single cell in a row.
type TableCellElement struct {
	Children []ElementRenderer
	Head     bool
}

// Render renders a TableElement.
func (e *TableElement) Render(w io.Writer, ctx RenderContext) error {
	bs := ctx.blockStack

	var indentation uint
	var margin uint
	rules := ctx.options.Styles.Table
	if rules.Indent != nil {
		indentation = *rules.Indent
	}
	if rules.Margin != nil {
		margin = *rules.Margin
	}

	iw := indent.NewWriterPipe(w, indentation+margin, func(_ io.Writer) {
		renderText(w, ctx.options.ColorProfile, bs.Current().Style.StylePrimitive, " ")
	})

	style := bs.With(rules.StylePrimitive)

	renderText(iw, ctx.options.ColorProfile, bs.Current().Style.StylePrimitive, rules.BlockPrefix)
	renderText(iw, ctx.options.ColorProfile, style, rules.Prefix)
	width := int(ctx.blockStack.Width(ctx)) //nolint: gosec

	wrap := true
	if ctx.options.TableWrap != nil {
		wrap = *ctx.options.TableWrap
	}
	ctx.table.tableWidth = width
	ctx.table.lipgloss = table.New().Width(width).Wrap(wrap)

	if err := e.collectLinksAndImages(ctx); err != nil {
		return err
	}

	return nil
}

func (e *TableElement) setStyles(ctx RenderContext) {
	ctx.table.lipgloss = ctx.table.lipgloss.StyleFunc(func(_, col int) lipgloss.Style {
		st := e.columnStyle(ctx, col)

		// Pin the columns to the layout of the first table with the same
		// headers so equivalent tables line up across the document.
		if col < len(e.pinned) {
			st = st.Width(e.pinned[col])
		}

		return st
	})
}

// columnStyle returns the style used for a column before its width is
// computed.
func (e *TableElement) columnStyle(ctx RenderContext, col int) lipgloss.Style {
	st := lipgloss.NewStyle().Inline(false)
	// Default Styles
	st = st.Margin(0, 1)

	// Override with custom styles
	if m := ctx.options.Styles.Table.Margin; m != nil {
		st = st.Padding(0, int(*m)) //nolint: gosec
	}

	if col < len(e.table.Alignments) {
		switch e.table.Alignments[col] {
		case astext.AlignLeft:
			st = st.Align(lipgloss.Left).PaddingRight(0)
		case astext.AlignCenter:
			st = st.Align(lipgloss.Center)
		case astext.AlignRight:
			st = st.Align(lipgloss.Right).PaddingLeft(0)
		case astext.AlignNone:
			// do nothing
		}
	}

	return st
}

func (e *TableElement) setBorders(ctx RenderContext) {
	rules := ctx.options.Styles.Table
	border := lipgloss.NormalBorder()

	if rules.RowSeparator != nil && rules.ColumnSeparator != nil {
		border = lipgloss.Border{
			Top:    *rules.RowSeparator,
			Bottom: *rules.RowSeparator,
			Left:   *rules.ColumnSeparator,
			Right:  *rules.ColumnSeparator,
			Middle: *rules.CenterSeparator,
		}
	}
	border.MiddleLeft = ""
	border.MiddleRight = ""
	ctx.table.lipgloss.Border(border)
	ctx.table.lipgloss.BorderRow(true)
	ctx.table.lipgloss.BorderTop(false)
	ctx.table.lipgloss.BorderLeft(false)
	ctx.table.lipgloss.BorderRight(false)
	ctx.table.lipgloss.BorderBottom(false)
}

// Finish finishes rendering a TableElement.
func (e *TableElement) Finish(_ io.Writer, ctx RenderContext) error {
	defer func() {
		ctx.table.lipgloss = nil
		ctx.table.tableImages = nil
		ctx.table.tableLinks = nil
		ctx.table.headerSig = ""
	}()

	rules := ctx.options.Styles.Table

	// Reuse the column widths of the first table with the same headers at
	// the same width, so equivalent tables line up across the document. The
	// first table is laid out by lipgloss and its widths are captured from
	// the rendered row separators.
	key := tableLayoutKey(ctx.table.headerSig, ctx.table.tableWidth)
	layout, ok := ctx.table.layouts[key]
	e.pinned = layout.widths

	e.setStyles(ctx)
	e.setBorders(ctx)

	out := ctx.table.lipgloss.String()

	if !ok {
		sep, junction := tableSeparators(rules)
		if widths := tableColumnWidths(out, sep, junction); widths != nil {
			if ctx.table.layouts == nil {
				ctx.table.layouts = map[string]tableColumnLayout{}
			}
			ctx.table.layouts[key] = tableColumnLayout{widths: widths}
		}
	}

	ow := ctx.blockStack.Current().Block
	if _, err := ow.WriteString(out); err != nil {
		return fmt.Errorf("glamour: error writing to buffer: %w", err)
	}

	renderText(ow, ctx.options.ColorProfile, ctx.blockStack.With(rules.StylePrimitive), rules.Suffix)
	renderText(ow, ctx.options.ColorProfile, ctx.blockStack.Current().Style.StylePrimitive, rules.BlockSuffix)

	e.printTableLinks(ctx)

	return nil
}

// Finish finishes rendering a TableRowElement.
func (e *TableRowElement) Finish(_ io.Writer, ctx RenderContext) error {
	if ctx.table.lipgloss == nil {
		return nil
	}

	ctx.table.lipgloss.Row(ctx.table.row...)
	ctx.table.row = []string{}
	return nil
}

// Finish finishes rendering a TableHeadElement.
func (e *TableHeadElement) Finish(_ io.Writer, ctx RenderContext) error {
	if ctx.table.lipgloss == nil {
		return nil
	}

	ctx.table.headerSig = tableHeaderSig(ctx.table.header)
	ctx.table.lipgloss.Headers(ctx.table.header...)
	ctx.table.header = []string{}
	return nil
}

// Render renders a TableCellElement.
func (e *TableCellElement) Render(_ io.Writer, ctx RenderContext) error {
	var b bytes.Buffer
	style := ctx.options.Styles.Table.StylePrimitive
	for _, child := range e.Children {
		if r, ok := child.(StyleOverriderElementRenderer); ok {
			if err := r.StyleOverrideRender(&b, ctx, style); err != nil {
				return fmt.Errorf("glamour: error rendering with style: %w", err)
			}
		} else {
			var bb bytes.Buffer
			if err := child.Render(&bb, ctx); err != nil {
				return fmt.Errorf("glamour: error rendering: %w", err)
			}
			el := &BaseElement{
				Token: bb.String(),
				Style: style,
			}
			if err := el.Render(&b, ctx); err != nil {
				return err
			}
		}
	}

	if e.Head {
		ctx.table.header = append(ctx.table.header, b.String())
	} else {
		ctx.table.row = append(ctx.table.row, b.String())
	}

	return nil
}

// tableLayoutKey identifies a table by its headers and available width.
func tableLayoutKey(headerSig string, width int) string {
	return fmt.Sprintf("%d%s", width, headerSig)
}

// tableHeaderSig builds a key for a table's header row. ANSI styles are
// stripped so the same headers with different styling still match.
func tableHeaderSig(header []string) string {
	var b strings.Builder
	for _, h := range header {
		b.WriteByte(0)
		b.WriteString(xansi.Strip(h))
	}
	return b.String()
}

// tableSeparators returns the characters used for the row separators and the
// column junctions.
func tableSeparators(rules StyleTable) (sep, junction string) {
	sep, junction = "─", "┼"
	if rules.RowSeparator != nil {
		sep = *rules.RowSeparator
	}
	if rules.CenterSeparator != nil {
		junction = *rules.CenterSeparator
	}
	return sep, junction
}

// tableColumnWidths extracts the column widths from the first row separator
// of a rendered table.
func tableColumnWidths(out, sep, junction string) []int {
	if sep == "" {
		return nil
	}

	for _, line := range strings.Split(out, "\n") {
		line = xansi.Strip(line)

		isSeparator := line != ""
		for _, r := range line {
			if r != ' ' && !strings.ContainsRune(sep+junction, r) {
				isSeparator = false
				break
			}
		}
		if !isSeparator || !strings.Contains(line, sep) {
			continue
		}

		columns := strings.Split(line, junction)
		widths := make([]int, len(columns))
		for i, col := range columns {
			widths[i] = xansi.StringWidth(col)
			if widths[i] == 0 {
				return nil
			}
		}
		return widths
	}

	return nil
}
