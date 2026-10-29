package files

import (
	"context"
	"strings"

	"copypaste.hk/platform/internal/core"
)

const TextComparisonMaxLines = 40000
const textComparisonMaxDistance = 256
const textComparisonWork = 1000000

type TextComparisonRow struct {
	Kind       string `json:"kind"`
	BaseLine   *int   `json:"base_line"`
	TargetLine *int   `json:"target_line"`
	Text       string `json:"text"`
	Newline    bool   `json:"newline"`
	Count      int    `json:"count"`
}

type textComparisonDiff struct {
	Rows                      []TextComparisonRow
	Added, Removed, Unchanged int
	BaseLines, TargetLines    int
	Algorithm                 string
}

func comparisonLines(body string) []string {
	if body == "" {
		return nil
	}
	lines := strings.SplitAfter(body, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// The trace is bounded independently of document length. If an exact shortest
// line edit exceeds the work budget, replacement still preserves every byte of
// the complete middle. It never reports a truncated or approximate body.
func comparisonMiddle(ctx context.Context, a, b []string) ([]byte, bool, error) {
	if len(a) == 0 {
		return []byte(strings.Repeat("+", len(b))), false, nil
	}
	if len(b) == 0 {
		return []byte(strings.Repeat("-", len(a))), false, nil
	}
	v := map[int]int{1: 0}
	trace := make([]map[int]int, 0, textComparisonMaxDistance+1)
	work := 0
	for d := 0; d <= textComparisonMaxDistance; d++ {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		previous := make(map[int]int, len(v))
		for k, x := range v {
			previous[k] = x
		}
		trace = append(trace, previous)
		for k := -d; k <= d; k += 2 {
			work++
			if work > textComparisonWork {
				return nil, true, nil
			}
			x := v[k-1] + 1
			if k == -d || k != d && v[k-1] < v[k+1] {
				x = v[k+1]
			}
			y := x - k
			for x < len(a) && y < len(b) && a[x] == b[y] {
				x++
				y++
				work++
				if work > textComparisonWork {
					return nil, true, nil
				}
			}
			v[k] = x
			if x < len(a) || y < len(b) {
				continue
			}
			ops := make([]byte, 0, len(a)+len(b))
			x, y = len(a), len(b)
			for depth := d; depth >= 0; depth-- {
				p := trace[depth]
				key := x - y
				prior := key - 1
				if key == -depth || key != depth && p[key-1] < p[key+1] {
					prior = key + 1
				}
				px, py := p[prior], p[prior]-prior
				for x > px && y > py {
					ops = append(ops, '=')
					x--
					y--
				}
				if depth == 0 {
					break
				}
				if x == px {
					ops = append(ops, '+')
					y--
				} else {
					ops = append(ops, '-')
					x--
				}
			}
			for l, r := 0, len(ops)-1; l < r; l, r = l+1, r-1 {
				ops[l], ops[r] = ops[r], ops[l]
			}
			return ops, false, nil
		}
	}
	return nil, true, nil
}

func compareTextLines(ctx context.Context, before, after, view string) (textComparisonDiff, error) {
	out := textComparisonDiff{Rows: []TextComparisonRow{}, Algorithm: "myers-v1"}
	if strings.Count(before, "\n") > TextComparisonMaxLines || strings.Count(after, "\n") > TextComparisonMaxLines {
		return out, core.Fail(422, "FILE_COMPARISON_LINE_LIMIT", "每个版本最多比较40000行；未生成部分比较结果，请查阅准确原件")
	}
	a, b := comparisonLines(before), comparisonLines(after)
	out.BaseLines, out.TargetLines = len(a), len(b)
	if len(a) > TextComparisonMaxLines || len(b) > TextComparisonMaxLines {
		return out, core.Fail(422, "FILE_COMPARISON_LINE_LIMIT", "每个版本最多比较40000行；未生成部分比较结果，请查阅准确原件")
	}
	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(a)-prefix && suffix < len(b)-prefix && a[len(a)-1-suffix] == b[len(b)-1-suffix] {
		suffix++
	}
	middle, fallback, err := comparisonMiddle(ctx, a[prefix:len(a)-suffix], b[prefix:len(b)-suffix])
	if err != nil {
		return out, err
	}
	if fallback {
		out.Algorithm = "bounded-replacement-v1"
		middle = append([]byte(strings.Repeat("-", len(a)-prefix-suffix)), []byte(strings.Repeat("+", len(b)-prefix-suffix))...)
	}
	ops := append([]byte(strings.Repeat("=", prefix)), middle...)
	ops = append(ops, []byte(strings.Repeat("=", suffix))...)
	rows := make([]TextComparisonRow, 0, len(ops))
	x, y := 0, 0
	for i, op := range ops {
		if i%1024 == 0 {
			if err = ctx.Err(); err != nil {
				return out, err
			}
		}
		row := TextComparisonRow{Count: 1}
		var line string
		if op != '+' {
			x++
			n := x
			row.BaseLine, line = &n, a[x-1]
		}
		if op != '-' {
			y++
			n := y
			row.TargetLine, line = &n, b[y-1]
		}
		row.Text, row.Newline = strings.TrimSuffix(line, "\n"), strings.HasSuffix(line, "\n")
		switch op {
		case '=':
			row.Kind = "equal"
			out.Unchanged++
		case '-':
			row.Kind = "delete"
			out.Removed++
		case '+':
			row.Kind = "insert"
			out.Added++
		}
		rows = append(rows, row)
	}
	if view == "all" {
		out.Rows = rows
		return out, nil
	}
	keep := make([]bool, len(rows))
	for i, row := range rows {
		if row.Kind != "equal" {
			for j := max(0, i-3); j < min(len(rows), i+4); j++ {
				keep[j] = true
			}
		}
	}
	for i := 0; i < len(rows); {
		if keep[i] {
			out.Rows = append(out.Rows, rows[i])
			i++
			continue
		}
		start := i
		for i < len(rows) && !keep[i] {
			i++
		}
		out.Rows = append(out.Rows, TextComparisonRow{Kind: "skip", BaseLine: rows[start].BaseLine, TargetLine: rows[start].TargetLine, Count: i - start})
	}
	return out, nil
}
