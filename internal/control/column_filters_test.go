package control

import (
	"net/http/httptest"
	"testing"
)

func TestConsoleQueryParsesWatchColumnFilters(t *testing.T) {
	q, err := consoleQuery(httptest.NewRequest("GET", "/v1/console/watches?condition=open&input=day&health=missing&delivery=clear&status=paused", nil))
	if err != nil || q.Condition != "open" || q.Input != "day" || q.Health != "missing" || q.Delivery != "clear" || q.Status != "paused" {
		t.Fatal(q, err)
	}
	for _, query := range []string{"condition=typo", "input=typo", "health=typo", "delivery=typo"} {
		if _, err := consoleQuery(httptest.NewRequest("GET", "/v1/console/watches?"+query, nil)); err == nil {
			t.Fatal("invalid filter accepted", query)
		}
	}
}
