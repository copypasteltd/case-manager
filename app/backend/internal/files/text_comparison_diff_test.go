package files

import (
	"context"
	"math/rand"
	"strings"
	"testing"
)

func reconstructComparison(t *testing.T, rows []TextComparisonRow) (string, string) {
	t.Helper()
	var before, after strings.Builder
	x, y := 1, 1
	for _, row := range rows {
		if row.Count != 1 || row.Kind == "skip" {
			t.Fatal("complete view contains omitted or grouped lines", row)
		}
		text := row.Text
		if row.Newline {
			text += "\n"
		}
		if row.Kind != "insert" {
			if row.BaseLine == nil || *row.BaseLine != x {
				t.Fatal("base coordinates", row, x)
			}
			x++
			before.WriteString(text)
		} else if row.BaseLine != nil {
			t.Fatal("insert has base coordinate")
		}
		if row.Kind != "delete" {
			if row.TargetLine == nil || *row.TargetLine != y {
				t.Fatal("target coordinates", row, y)
			}
			y++
			after.WriteString(text)
		} else if row.TargetLine != nil {
			t.Fatal("deletion has target coordinate")
		}
	}
	return before.String(), after.String()
}

func TestTextComparisonExactBodiesAndShortestSmallDiff(t *testing.T) {
	random := rand.New(rand.NewSource(113))
	cases := [][2]string{{"", ""}, {"", "\n"}, {"香港🙂\n旧段\n", "香港🙂\n新段\n"}, {"same\n", "same"}, {"\n\n", "\n"}, {"<script>alert('x')</script>", "<&>🙂"}, {strings.Repeat("长正文🙂", 20000), strings.Repeat("长正文🙂", 20000) + "后缀"}}
	for i := 0; i < 300; i++ {
		var a, b strings.Builder
		for j := 0; j < random.Intn(15); j++ {
			a.WriteString([]string{"甲\n", "乙🙂\n", "\n"}[random.Intn(3)])
		}
		for j := 0; j < random.Intn(15); j++ {
			b.WriteString([]string{"甲\n", "乙🙂\n", "\n"}[random.Intn(3)])
		}
		cases = append(cases, [2]string{a.String(), b.String()})
	}
	for i, pair := range cases {
		diff, err := compareTextLines(context.Background(), pair[0], pair[1], "all")
		if err != nil {
			t.Fatal(i, err)
		}
		a, b := reconstructComparison(t, diff.Rows)
		if a != pair[0] || b != pair[1] {
			t.Fatal("full Unicode/body/final-newline reconstruction differs", i)
		}
		left, right := comparisonLines(a), comparisonLines(b)
		// Independent dynamic-programming oracle, not the production algorithm.
		previous := make([]int, len(right)+1)
		for x := range left {
			current := make([]int, len(right)+1)
			for y := range right {
				if left[x] == right[y] {
					current[y+1] = previous[y] + 1
				} else {
					current[y+1] = max(previous[y+1], current[y])
				}
			}
			previous = current
		}
		if diff.Algorithm != "myers-v1" || diff.Added+diff.Removed != len(left)+len(right)-2*previous[len(right)] {
			t.Fatal("small diff is not a shortest exact edit", i, diff)
		}
	}
}

func TestTextComparisonCompleteBudgetFallbackAndContext(t *testing.T) {
	before, after := strings.Repeat("原行\n", 600), strings.Repeat("新行🙂\n", 600)
	diff, err := compareTextLines(context.Background(), before, after, "all")
	if err != nil || diff.Algorithm != "bounded-replacement-v1" || diff.Added != 600 || diff.Removed != 600 {
		t.Fatal("bounded full replacement", diff.Algorithm, err)
	}
	a, b := reconstructComparison(t, diff.Rows)
	if a != before || b != after {
		t.Fatal("budget fallback lost content")
	}
	prefix, suffix := strings.Repeat("共同行\n", 15), strings.Repeat("末段\n", 12)
	contextual, err := compareTextLines(context.Background(), prefix+"旧\n"+suffix, prefix+"新\n"+suffix, "changes")
	if err != nil || len(contextual.Rows) != 10 || contextual.Rows[0].Kind != "skip" || contextual.Rows[0].Count != 12 || contextual.Rows[9].Kind != "skip" || contextual.Rows[9].Count != 9 {
		t.Fatal("context must explicitly account for all omitted equal lines", contextual.Rows, err)
	}
	if _, err = compareTextLines(context.Background(), strings.Repeat("\n", TextComparisonMaxLines+1), "", "all"); err == nil {
		t.Fatal("line resource limit returned a partial success")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = compareTextLines(ctx, "old\n", "new\n", "all"); err == nil {
		t.Fatal("cancelled work continued")
	}
}
