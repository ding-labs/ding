package plan

import (
	"encoding/json"
	"fmt"
	"time"
)

type Description struct {
	ID        string `json:"id"`
	Source    string `json:"source"`
	Condition string `json:"condition"`
	Policy    string `json:"policy"`
}

func Describe(p Compiled) Description {
	s := p.Definition.Spec
	d := Description{ID: p.Definition.Metadata.ID}
	switch s.Source.Type {
	case "logs":
		d.Source = fmt.Sprintf("Receive live application output summaries over %s; require %d records. Start capture with ding logs exec or ding logs ingest.", s.Source.Logs.Window, s.Source.Logs.MinEvents)
		switch i := s.Source.Logs.Input; i.Type {
		case "file":
			d.Source = fmt.Sprintf("Follow %q read-only on the daemon host, starting at its current end. Rebuild the live window after rotation or interruption.", i.Path)
		case "command":
			d.Source = fmt.Sprintf("Run log reader %q in %q on the daemon host; reconnect after %s. Pause stops the reader. Configure the reader for live output only.", i.Argv, i.Directory, i.ReconnectEvery)
		}
	case "http":
		target := s.Source.URL
		if s.Source.URLRef != nil {
			target = "URL from " + s.Source.URLRef.Env
		}
		d.Source = fmt.Sprintf("Check %s every %s (timeout %s).", target, s.Source.Every, s.Source.Timeout)
	case "command":
		d.Source = fmt.Sprintf("Run %q on the daemon host every %s (timeout %s).", s.Source.Argv, s.Source.Every, s.Source.Timeout)
	case "push":
		d.Source = "Receive JSON posted to /v1/ingest/" + d.ID + "."
	}
	c := s.Condition
	v, _ := json.Marshal(c.Value)
	switch {
	case c.Numeric != "":
		d.Condition = c.Numeric
	case c.MissingFor != "":
		d.Condition = "No accepted observation arrives for " + c.MissingFor + "."
	case c.Operator == "changed":
		d.Condition = "The typed value of " + c.Field + " changes from its baseline."
	case c.Operator == "new-event":
		d.Condition = "A new " + c.Field + " value is seen; deduplicate for " + c.DedupFor + "."
	default:
		words := map[string]string{"eq": "equals", "ne": "does not equal", "gt": "is greater than", "gte": "is at least", "lt": "is less than", "lte": "is at most"}
		op := words[c.Operator]
		if op == "" {
			op = c.Operator
		}
		d.Condition = fmt.Sprintf("%s %s %s.", c.Field, op, v)
		if s.Source.Type == "http" && c.Field == "http.status" && c.Operator == "gte" && string(v) == "500" {
			d.Condition = "HTTP status is at least 500 (server errors). Responses such as 401 or 404 do not match; a nonmatch does not prove service health."
		}
	}
	if c.Operator == "changed" || c.Operator == "new-event" {
		d.Policy = "Emit a " + c.Operator + " event for each qualifying input."
	} else if s.Policy.Trigger == "transition" {
		d.Policy = fmt.Sprintf("Open an incident after %d consecutive matches; recover after %d nonmatching inputs.", s.Policy.Consecutive, s.Policy.RecoverAfter)
	} else {
		d.Policy = fmt.Sprintf("Fire after %d consecutive matches, limited to one firing per %s.", s.Policy.Consecutive, s.Policy.Interval)
	}
	if s.Policy.OnUnknown == "hold-incident" {
		d.Policy += " Unknown inputs preserve the incident state; they do not count as successful checks."
	} else {
		d.Policy += " Unknown inputs: " + s.Policy.OnUnknown + "."
	}
	if s.Source.Type == "http" {
		d.Source += " Timeouts and unreachable endpoints are reported as source errors separately."
		if s.Policy.Trigger == "transition" && c.Numeric == "" && c.MissingFor == "" && s.Policy.Consecutive > 1 {
			if every, err := time.ParseDuration(s.Source.Every); err == nil {
				d.Policy += fmt.Sprintf(" At this cadence, %d matching checks span %s from the first to the last. Waiting for the next check, response time, scheduling, and notification delivery can add delay.", s.Policy.Consecutive, (time.Duration(s.Policy.Consecutive-1) * every).String())
			}
		}
	}
	return d
}
