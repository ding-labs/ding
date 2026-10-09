package plan

import (
	"encoding/json"
	"fmt"
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
	}
	if c.Operator == "changed" || c.Operator == "new-event" {
		d.Policy = "Emit a " + c.Operator + " event for each qualifying input."
	} else if s.Policy.Trigger == "transition" {
		d.Policy = fmt.Sprintf("Open an incident after %d consecutive matches; recover after %d nonmatching inputs.", s.Policy.Consecutive, s.Policy.RecoverAfter)
	} else {
		d.Policy = fmt.Sprintf("Fire after %d consecutive matches, limited to one firing per %s.", s.Policy.Consecutive, s.Policy.Interval)
	}
	d.Policy += " Unknown inputs: " + s.Policy.OnUnknown + "."
	return d
}
