package condition

import (
	"strings"
	"testing"
)

func TestGrammar(t *testing.T) {
	for _, input := range []string{"", "value > 1 AND ", " AND value > 1", " OR value > 1", "value > 1 OR ", "value > 1 and value < 2", "avg(value) over 0s > 1", "avg(value) over 9999999999999999999999999h > 1", "value > " + strings.Repeat("9", 400), "sum(value) over 1m > " + strings.Repeat("9", 400)} {
		if _, err := ParseExpression(input); err == nil {
			t.Error("accepted", input)
		}
	}
	e, err := ParseExpression("value == 1 OR avg(value) over 1m > 2 AND value < 10")
	if err != nil {
		t.Fatal(err)
	}
	if !e.Eval(Context{Value: 1}) || e.Eval(Context{Value: 11, Available: map[int]bool{0: true}, Aggregates: map[int]float64{0: 5}}) {
		t.Fatal("precedence")
	}
	if e.Eval(Context{Value: 5}) {
		t.Fatal("missing window available")
	}
	if !e.Eval(Context{Value: 5, Available: map[int]bool{0: true}, Aggregates: map[int]float64{0: 3}}) {
		t.Fatal("compound evaluation")
	}
	if len(e.Windows()) != 1 || e.Windows()[0].ID != 0 {
		t.Fatal("window IDs")
	}
	if CompareNumber(1, "invalid", 1) {
		t.Fatal("unknown operator")
	}
	if v, err := ParseCondition("max(value) over run >= 1"); err != nil || !v.RunBounded {
		t.Fatal("legacy oracle run support", v, err)
	}
}
