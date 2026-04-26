package notifier_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zuchka/ding/internal/evaluator"
	"github.com/zuchka/ding/internal/notifier"
)

// Mirror unexported Teams types for test assertions.
type teamsTestTextBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Weight   string `json:"weight,omitempty"`
	Size     string `json:"size,omitempty"`
	Wrap     bool   `json:"wrap,omitempty"`
	IsSubtle bool   `json:"isSubtle,omitempty"`
}

type teamsTestFact struct {
	Title string `json:"title"`
	Value string `json:"value"`
}

type teamsTestFactSet struct {
	Type  string          `json:"type"`
	Facts []teamsTestFact `json:"facts"`
}

type teamsTestCard struct {
	Schema  string            `json:"$schema"`
	Type    string            `json:"type"`
	Version string            `json:"version"`
	Body    []json.RawMessage `json:"body"`
}

type teamsTestAttachment struct {
	ContentType string        `json:"contentType"`
	ContentURL  *string       `json:"contentUrl"`
	Content     teamsTestCard `json:"content"`
}

type teamsTestMessage struct {
	Type        string                `json:"type"`
	Attachments []teamsTestAttachment `json:"attachments"`
}

func parseTeamsPayload(t *testing.T, body []byte) teamsTestMessage {
	t.Helper()
	var msg teamsTestMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		t.Fatalf("invalid Teams JSON: %v\nbody: %s", err, body)
	}
	return msg
}

func findFactSet(t *testing.T, body []json.RawMessage) teamsTestFactSet {
	t.Helper()
	for _, raw := range body {
		var el struct {
			Type string `json:"type"`
		}
		json.Unmarshal(raw, &el)
		if el.Type == "FactSet" {
			var fs teamsTestFactSet
			if err := json.Unmarshal(raw, &fs); err != nil {
				t.Fatalf("unmarshal FactSet: %v", err)
			}
			return fs
		}
	}
	t.Fatal("no FactSet found in card body")
	return teamsTestFactSet{}
}

func TestTeamsNotifier_Send_success(t *testing.T) {
	delivered := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		delivered <- buf[:n]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := notifier.NewTeamsNotifier(srv.URL, 3, 1*time.Millisecond, nil)
	defer n.Stop()

	if err := n.Send(makeRunAlert()); err != nil {
		t.Fatal(err)
	}

	select {
	case body := <-delivered:
		var msg map[string]json.RawMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			t.Fatalf("received invalid JSON: %v\nbody: %s", err, body)
		}
		if _, ok := msg["attachments"]; !ok {
			t.Errorf("expected 'attachments' key in Teams payload, got: %s", body)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Teams delivery")
	}
}

func TestTeamsNotifier_Send_retries5xx(t *testing.T) {
	var count atomic.Int32
	delivered := make(chan struct{}, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := count.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
			select {
			case delivered <- struct{}{}:
			default:
			}
		}
	}))
	defer srv.Close()

	n := notifier.NewTeamsNotifier(srv.URL, 5, 1*time.Millisecond, nil)
	defer n.Stop()

	if err := n.Send(makeRunAlert()); err != nil {
		t.Fatal(err)
	}

	select {
	case <-delivered:
		if got := count.Load(); got != 3 {
			t.Errorf("expected 3 requests (2 failures + 1 success), got %d", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for delivery; got %d requests", count.Load())
	}
}

func TestTeamsNotifier_Send_drops4xx(t *testing.T) {
	var count atomic.Int32
	done := make(chan struct{}, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		select {
		case done <- struct{}{}:
		default:
		}
	}))
	defer srv.Close()

	n := notifier.NewTeamsNotifier(srv.URL, 3, 1*time.Millisecond, nil)
	defer n.Stop()

	if err := n.Send(makeRunAlert()); err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for first request")
	}
	time.Sleep(50 * time.Millisecond)

	if got := count.Load(); got != 1 {
		t.Errorf("expected exactly 1 POST for 4xx, got %d", got)
	}
}

func TestTeamsNotifier_Stop(t *testing.T) {
	n := notifier.NewTeamsNotifier("http://127.0.0.1:1", 3, 1*time.Millisecond, nil)

	stopped := make(chan struct{})
	go func() {
		n.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("Stop() blocked unexpectedly")
	}
}

func TestTeamsNotifier_Drain(t *testing.T) {
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := notifier.NewTeamsNotifier(srv.URL, 3, 1*time.Millisecond, nil)

	for i := 0; i < 5; i++ {
		_ = n.Send(makeRunAlert())
	}
	n.Drain(2 * time.Second)

	if got := count.Load(); got != 5 {
		t.Errorf("expected 5 deliveries after Drain, got %d", got)
	}
}

func TestBuildTeamsPayload_runContext(t *testing.T) {
	delivered := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 8192)
		n, _ := r.Body.Read(buf)
		delivered <- buf[:n]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := notifier.NewTeamsNotifier(srv.URL, 3, 1*time.Millisecond, nil)
	defer n.Stop()

	if err := n.Send(makeRunAlert()); err != nil {
		t.Fatal(err)
	}

	select {
	case body := <-delivered:
		msg := parseTeamsPayload(t, body)

		if msg.Type != "message" {
			t.Errorf("message type: want \"message\", got %q", msg.Type)
		}
		if len(msg.Attachments) != 1 {
			t.Fatalf("expected 1 attachment, got %d", len(msg.Attachments))
		}
		att := msg.Attachments[0]
		if att.ContentType != "application/vnd.microsoft.card.adaptive" {
			t.Errorf("contentType: want adaptive card, got %q", att.ContentType)
		}
		if att.ContentURL != nil {
			t.Errorf("contentUrl: want nil, got %v", att.ContentURL)
		}

		card := att.Content
		if card.Type != "AdaptiveCard" {
			t.Errorf("card type: want AdaptiveCard, got %q", card.Type)
		}
		if card.Version != "1.5" {
			t.Errorf("card version: want 1.5, got %q", card.Version)
		}

		// First body element: header TextBlock
		var header teamsTestTextBlock
		if err := json.Unmarshal(card.Body[0], &header); err != nil {
			t.Fatalf("unmarshal header: %v", err)
		}
		if header.Text != "test_failure" {
			t.Errorf("header text: want test_failure, got %q", header.Text)
		}
		if header.Weight != "Bolder" {
			t.Errorf("header weight: want Bolder, got %q", header.Weight)
		}
		if header.Wrap {
			t.Error("header TextBlock: Wrap should not be set on header")
		}

		// Second body element: message TextBlock
		var msgBlock teamsTestTextBlock
		if err := json.Unmarshal(card.Body[1], &msgBlock); err != nil {
			t.Fatalf("unmarshal message block: %v", err)
		}
		if msgBlock.Text != "Tests failed on branch main" {
			t.Errorf("message text: want %q, got %q", "Tests failed on branch main", msgBlock.Text)
		}
		if !msgBlock.Wrap {
			t.Error("message TextBlock: want Wrap=true")
		}

		// FactSet: verify key facts
		fs := findFactSet(t, card.Body)
		factMap := make(map[string]string, len(fs.Facts))
		for _, f := range fs.Facts {
			factMap[f.Title] = f.Value
		}
		for title, want := range map[string]string{
			"Metric":    "`run.exit`",
			"Value":     "`1`",
			"exit code": "`1`",
			"duration":  "`42.5s`",
			"branch":    "`main`",
			"commit":    "`abc1234`",
			"repo":      "`acme/api`",
		} {
			if got := factMap[title]; got != want {
				t.Errorf("fact %q: want %q, got %q", title, want, got)
			}
		}

		// Footer: last body element
		var footer teamsTestTextBlock
		if err := json.Unmarshal(card.Body[len(card.Body)-1], &footer); err != nil {
			t.Fatalf("unmarshal footer: %v", err)
		}
		if !footer.IsSubtle {
			t.Error("footer: want IsSubtle=true")
		}
		if footer.Size != "Small" {
			t.Errorf("footer size: want Small, got %q", footer.Size)
		}
		if footer.Text == "" {
			t.Error("footer text is empty")
		}

	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for delivery")
	}
}

func TestBuildTeamsPayload_noRunContext(t *testing.T) {
	delivered := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		delivered <- buf[:n]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := notifier.NewTeamsNotifier(srv.URL, 3, 1*time.Millisecond, nil)
	defer n.Stop()

	alert := evaluator.Alert{
		Rule:    "cpu_spike",
		Metric:  "cpu_usage",
		Value:   97,
		Labels:  map[string]string{"host": "web-01"},
		FiredAt: time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC),
	}

	if err := n.Send(alert); err != nil {
		t.Fatal(err)
	}

	select {
	case body := <-delivered:
		msg := parseTeamsPayload(t, body)

		if len(msg.Attachments) != 1 {
			t.Fatalf("expected 1 attachment, got %d", len(msg.Attachments))
		}
		card := msg.Attachments[0].Content

		// No message TextBlock: header + FactSet + footer = 3 elements
		if len(card.Body) != 3 {
			t.Errorf("expected 3 body elements (header + FactSet + footer), got %d", len(card.Body))
		}

		var header teamsTestTextBlock
		json.Unmarshal(card.Body[0], &header)
		if header.Text != "cpu_spike" {
			t.Errorf("header text: want cpu_spike, got %q", header.Text)
		}

		// Only metric and value in FactSet
		fs := findFactSet(t, card.Body)
		if len(fs.Facts) != 2 {
			t.Errorf("expected 2 facts (metric + value only), got %d: %v", len(fs.Facts), fs.Facts)
		}
		if fs.Facts[0].Title != "Metric" {
			t.Errorf("first fact: want Metric, got %q", fs.Facts[0].Title)
		}
		if fs.Facts[1].Title != "Value" {
			t.Errorf("second fact: want Value, got %q", fs.Facts[1].Title)
		}

		var footer teamsTestTextBlock
		json.Unmarshal(card.Body[len(card.Body)-1], &footer)
		if footer.Text == "" {
			t.Error("footer text is empty")
		}

	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for delivery")
	}
}
