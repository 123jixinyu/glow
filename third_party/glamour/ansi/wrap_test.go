package ansi

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
)

func TestTextWrap(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		limit    int
		preserve bool
		breakpts string
		want     string
	}{
		{
			name:     "list marker stays with wide text",
			in:       "• 一二三四五六七八九十\n",
			limit:    12,
			preserve: true,
			breakpts: " ,.;-+|",
			want:     "• 一二三四五\n六七八九十\n",
		},
		{
			name:     "closing punctuation does not start a line",
			in:       "字字字字字，字字字字字\n",
			limit:    10,
			preserve: true,
			breakpts: " ,.;-+|",
			want:     "字字字字\n字，字字字\n字字\n",
		},
		{
			name:     "opening punctuation does not end a line",
			in:       "（字字字字字）字字字字字\n",
			limit:    10,
			preserve: true,
			breakpts: " ,.;-+|",
			want:     "（字字字字\n字）字字字\n字字\n",
		},
		{
			name:     "long word fills the line it starts on",
			in:       "• supercalifragilistic\nnext item\n",
			limit:    12,
			preserve: true,
			breakpts: " ,.;-+|",
			want:     "• supercalif\nragilistic\nnext item\n",
		},
		{
			name:     "ansi sequences are preserved",
			in:       "• \x1b[31m红色文字很长很长很长很长很长很长\x1b[0m plain\n",
			limit:    16,
			preserve: true,
			breakpts: " ,.;-+|",
			want:     "• \x1b[31m红色文字很长很\n长很长很长很长很\n长\x1b[0m plain\n",
		},
		{
			name:     "newlines become spaces when not preserved",
			in:       "one\ntwo\n",
			limit:    80,
			preserve: false,
			breakpts: "-",
			want:     "one two",
		},
		{
			name:     "wrapping disabled below limit one",
			in:       "abc\ndef",
			limit:    0,
			preserve: true,
			breakpts: " ,.;-+|",
			want:     "abc\ndef",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := textWrap(tt.in, tt.limit, tt.breakpts, tt.preserve); got != tt.want {
				t.Errorf("textWrap() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTextWrapDoesNotExceedLimit(t *testing.T) {
	inputs := []string{
		"• 中美脱钩：恒生及纳斯达克中国金龙指数在全球股市上涨中反走弱，外资逃离中国资产；老虎证券线索显示国内法留资，A 股及汇率相对强的原因，脱钩风险加大\n",
		"• A very long english list item that should wrap at word boundaries rather than breaking words apart.\n",
		"supercalifragilisticexpialidocious-and-then-some-more-extremely-long-word\n",
		"• \x1b[1m加粗中文内容\x1b[0m mixed with english text and more 中文内容\n",
		"  leading spaces should not create blank lines\n",
	}

	for _, in := range inputs {
		for limit := 2; limit <= 40; limit++ {
			out := textWrap(in, limit, " ,.;-+|", true)
			for i, line := range strings.Split(out, "\n") {
				if w := xansi.StringWidth(line); w > limit {
					t.Errorf("limit %d: line %d width %d exceeds limit in %q", limit, i, w, out)
				}
			}
		}
	}
}

func TestTextWrapKeepsContent(t *testing.T) {
	inputs := []string{
		"• 中美脱钩：恒生及纳斯达克中国金龙指数在全球股市上涨中反走弱，外资逃离中国资产。\n",
		"supercalifragilisticexpialidocious-and-then-some-more-extremely-long-word\n",
		"参考 https://example.com/a-very-long-path?with=query&and=more 的内容\n",
	}

	for _, in := range inputs {
		for limit := 4; limit <= 30; limit++ {
			out := textWrap(in, limit, " ,.;-+|", true)
			got := strings.Join(strings.Fields(xansi.Strip(out)), "")
			want := strings.Join(strings.Fields(in), "")
			if got != want {
				t.Fatalf("limit %d: content mismatch\n got: %q\nwant: %q", limit, got, want)
			}
		}
	}
}

func TestTextWrapNoOrphanClosingPunct(t *testing.T) {
	in := "中文内容，需要换行处理；同时要避免标点出现在行首。中文内容，需要换行处理；同时要避免标点出现在行首。\n"
	closing := "，；。、！？：）"
	for limit := 6; limit <= 40; limit++ {
		out := textWrap(in, limit, " ,.;-+|", true)
		for i, line := range strings.Split(out, "\n") {
			if line == "" {
				continue
			}
			r := []rune(xansi.Strip(line))[0]
			if strings.ContainsRune(closing, r) {
				t.Errorf("limit %d: line %d starts with closing punctuation %q in %q", limit, i, r, out)
			}
		}
	}
}
