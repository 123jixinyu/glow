package ansi

import (
	"bytes"
	"strings"
	"unicode"
	"unicode/utf8"

	xansi "github.com/charmbracelet/x/ansi"
)

// wrapAtom is a single unit of text handled by the wrapper: a grapheme
// cluster, an escape/control sequence (zero width) or a run of whitespace.
type wrapAtom struct {
	text  string
	width int
	r     rune
}

// textWrapper lays out text into lines of at most limit cells. Unlike
// x/ansi.Wordwrap it keeps a prefix (such as a list bullet) attached to the
// first word whenever possible and hard-wraps words that are too long to fit
// on a line. Wide graphemes (CJK and friends) are treated as break
// opportunities, which is how those scripts are normally laid out.
type textWrapper struct {
	limit       int
	breakpoints string

	out       bytes.Buffer
	lineWidth int

	spaces      bytes.Buffer
	spacesWidth int

	word      []wrapAtom
	wordWidth int
}

// textWrap wraps s to limit display cells, preserving ANSI escape sequences.
// It breaks at whitespace, after ASCII breakpoints and between wide
// graphemes. Words that do not fit on a line are hard-wrapped. When
// preserveNewlines is false, existing line breaks are treated as spaces.
func textWrap(s string, limit int, breakpoints string, preserveNewlines bool) string {
	if limit < 1 {
		return s
	}
	if !preserveNewlines {
		s = strings.ReplaceAll(s, "\n", " ")
	}

	w := &textWrapper{
		limit:       limit,
		breakpoints: breakpoints,
	}

	var p = xansi.NewParser()
	var state byte
	for len(s) > 0 {
		seq, width, n, newState := xansi.DecodeSequence(s, state, p)
		if n <= 0 {
			break
		}
		w.feed(seq, width)
		state = newState
		s = s[n:]
	}
	w.flushWord()

	return w.out.String()
}

func (w *textWrapper) feed(seq string, width int) {
	if len(seq) == 0 {
		return
	}

	switch {
	case seq[0] == '\n':
		w.feedNewline()
	case seq[0] == '\t':
		w.feedSpace(seq, 1)
	case seq[0] == '\x1b' || seq[0] < 0x20 || seq[0] == 0x7f:
		// Escape and control sequences are zero width and always kept.
		w.word = append(w.word, wrapAtom{text: seq})
	default:
		r, _ := utf8.DecodeRuneInString(seq)
		switch {
		case unicode.IsSpace(r) && r != '\u00a0':
			w.feedSpace(seq, max(width, 1))
		case strings.ContainsRune(w.breakpoints, r):
			// Breakpoints may be split from the following word.
			w.word = append(w.word, wrapAtom{text: seq, width: width, r: r})
			w.wordWidth += width
			w.flushWord()
		case width >= 2:
			// Wide graphemes (CJK and friends) are break
			// opportunities on their own.
			w.feedWide(seq, r, width)
		default:
			w.word = append(w.word, wrapAtom{text: seq, width: width, r: r})
			w.wordWidth += width
		}
	}
}

// feedWide handles a wide (typically CJK) grapheme. Such graphemes may break
// between each other, except where basic kinsoku rules apply: closing
// punctuation stays with the preceding grapheme and opening punctuation with
// the following one.
func (w *textWrapper) feedWide(text string, r rune, width int) {
	if len(w.word) > 0 {
		last := w.word[len(w.word)-1].r
		if !isOpeningPunct(last) && !isClosingPunct(r) {
			w.flushWord()
		}
	}
	w.word = append(w.word, wrapAtom{text: text, width: width, r: r})
	w.wordWidth += width
}

// isOpeningPunct reports whether r is punctuation that must not end a line.
func isOpeningPunct(r rune) bool {
	switch r {
	case '(', '[', '{',
		'（', '［', '｛', '〈', '《', '「', '『', '【', '〔', '〖', '〘', '〚',
		'“', '‘', '﹁', '﹃':
		return true
	}
	return false
}

// isClosingPunct reports whether r is punctuation that must not start a line.
func isClosingPunct(r rune) bool {
	switch r {
	case ')', ']', '}', ',', '.', ';', ':', '!', '?',
		'）', '］', '｝', '〉', '》', '」', '』', '】', '〕', '〗', '〙', '〛',
		'、', '。', '，', '．', '；', '：', '！', '？', '…', '—', '～', '％', '℃',
		'”', '’', '﹂', '﹄':
		return true
	}
	return false
}

func (w *textWrapper) feedSpace(seq string, width int) {
	w.flushWord()
	w.spaces.WriteString(seq)
	w.spacesWidth += width
}

func (w *textWrapper) feedNewline() {
	if len(w.word) > 0 {
		w.flushWord()
	} else if w.lineWidth+w.spacesWidth <= w.limit {
		// Preserve trailing whitespace that still fits, matching the
		// behaviour of x/ansi.Wordwrap.
		w.emitSpaces()
	} else {
		w.spaces.Reset()
		w.spacesWidth = 0
	}
	w.out.WriteByte('\n')
	w.lineWidth = 0
}

func (w *textWrapper) flushWord() {
	if len(w.word) == 0 {
		return
	}
	w.placeWord()
	w.word = w.word[:0]
	w.wordWidth = 0
}

// placeWord emits the buffered word, wrapping it to the next line when it
// does not fit and hard-wrapping it when it is longer than a full line.
func (w *textWrapper) placeWord() {
	switch {
	case w.lineWidth+w.spacesWidth+w.wordWidth <= w.limit:
		w.emitSpaces()
		w.emitWord(w.word)
	case w.wordWidth <= w.limit:
		if w.lineWidth > 0 {
			w.newline()
		} else {
			// Leading whitespace is dropped rather than emitting a
			// blank line.
			w.spaces.Reset()
			w.spacesWidth = 0
		}
		w.emitWord(w.word)
	default:
		w.hardWrap()
	}
}

func (w *textWrapper) hardWrap() {
	for len(w.word) > 0 {
		remaining := w.limit - w.lineWidth - w.spacesWidth
		if w.lineWidth == 0 && w.spacesWidth > 0 {
			// Leading whitespace is dropped at the start of a line.
			w.spaces.Reset()
			w.spacesWidth = 0
			remaining = w.limit
		}
		if remaining <= 0 {
			w.newline()
			continue
		}

		i := 0
		chunkWidth := 0
		for i < len(w.word) && chunkWidth+w.word[i].width <= remaining {
			chunkWidth += w.word[i].width
			i++
		}
		if i == 0 {
			if w.lineWidth > 0 {
				w.newline()
				continue
			}
			// A single grapheme wider than the whole line: emit it
			// anyway so we always make progress.
			i = 1
			chunkWidth = w.word[0].width
		}

		w.emitSpaces()
		w.emitWord(w.word[:i])
		w.lineWidth += chunkWidth
		w.word = w.word[i:]
		w.wordWidth -= chunkWidth

		if len(w.word) > 0 && w.lineWidth >= w.limit {
			w.newline()
		}
	}
}

func (w *textWrapper) emitSpaces() {
	if w.spaces.Len() == 0 {
		return
	}
	w.out.Write(w.spaces.Bytes())
	w.lineWidth += w.spacesWidth
	w.spaces.Reset()
	w.spacesWidth = 0
}

func (w *textWrapper) emitWord(atoms []wrapAtom) {
	for _, a := range atoms {
		w.out.WriteString(a.text)
		w.lineWidth += a.width
	}
}

func (w *textWrapper) newline() {
	w.spaces.Reset()
	w.spacesWidth = 0
	w.out.WriteByte('\n')
	w.lineWidth = 0
}
