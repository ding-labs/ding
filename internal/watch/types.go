// Package watch defines the portable, versioned watch contract. It owns no I/O.
package watch

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
	"unicode/utf8"
)

const APIVersion = "ding.ing/v1alpha1"

type Metadata struct {
	ID   string `json:"id" yaml:"id"`
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
}
type SecretRef struct {
	Env string `json:"env" yaml:"env"`
}
type Definition struct {
	APIVersion string   `json:"apiVersion" yaml:"apiVersion"`
	Kind       string   `json:"kind" yaml:"kind"`
	Metadata   Metadata `json:"metadata" yaml:"metadata"`
	Spec       Spec     `json:"spec" yaml:"spec"`
}
type Spec struct {
	Source       Source         `json:"source" yaml:"source"`
	Condition    Condition      `json:"condition" yaml:"condition"`
	Policy       Policy         `json:"policy" yaml:"policy"`
	GroupBy      []string       `json:"groupBy,omitempty" yaml:"groupBy,omitempty"`
	Match        map[string]any `json:"match,omitempty" yaml:"match,omitempty"`
	Message      string         `json:"message,omitempty" yaml:"message,omitempty"`
	Destinations []Target       `json:"destinations" yaml:"destinations"`
	Limits       Limits         `json:"limits" yaml:"limits"`
}
type Source struct {
	Type      string               `json:"type" yaml:"type"`
	URL       string               `json:"url,omitempty" yaml:"url,omitempty"`
	URLRef    *SecretRef           `json:"urlRef,omitempty" yaml:"urlRef,omitempty"`
	Every     string               `json:"every,omitempty" yaml:"every,omitempty"`
	Timeout   string               `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	Headers   map[string]SecretRef `json:"headers,omitempty" yaml:"headers,omitempty"`
	Fields    map[string]string    `json:"fields,omitempty" yaml:"fields,omitempty"`
	JQ        string               `json:"jq,omitempty" yaml:"jq,omitempty"`
	Argv      []string             `json:"argv,omitempty" yaml:"argv,omitempty"`
	Directory string               `json:"directory,omitempty" yaml:"directory,omitempty"`
	Env       map[string]SecretRef `json:"env,omitempty" yaml:"env,omitempty"`
}
type Condition struct {
	Field      string `json:"field,omitempty" yaml:"field,omitempty"`
	Operator   string `json:"operator,omitempty" yaml:"operator,omitempty"`
	Value      any    `json:"value" yaml:"value"`
	Numeric    string `json:"numeric,omitempty" yaml:"numeric,omitempty"`
	MissingFor string `json:"missingFor,omitempty" yaml:"missingFor,omitempty"`
	DedupFor   string `json:"dedupFor,omitempty" yaml:"dedupFor,omitempty"`
}
type Policy struct {
	Trigger      string `json:"trigger" yaml:"trigger"`
	Consecutive  int    `json:"consecutive" yaml:"consecutive"`
	RecoverAfter int    `json:"recoverAfter" yaml:"recoverAfter"`
	OnUnknown    string `json:"onUnknown" yaml:"onUnknown"`
	Interval     string `json:"interval,omitempty" yaml:"interval,omitempty"`
	MaxGap       string `json:"maxGap,omitempty" yaml:"maxGap,omitempty"`
}
type Target struct {
	Ref    string   `json:"ref" yaml:"ref"`
	Events []string `json:"events" yaml:"events"`
}
type Limits struct {
	MaxEntities int    `json:"maxEntities" yaml:"maxEntities"`
	MaxSamples  int    `json:"maxSamples" yaml:"maxSamples"`
	MaxBytes    int    `json:"maxBytes" yaml:"maxBytes"`
	MaxOutputs  int    `json:"maxOutputs" yaml:"maxOutputs"`
	IdleTTL     string `json:"idleTTL" yaml:"idleTTL"`
}
type Destination struct {
	APIVersion string          `json:"apiVersion" yaml:"apiVersion"`
	Kind       string          `json:"kind" yaml:"kind"`
	Metadata   Metadata        `json:"metadata" yaml:"metadata"`
	Spec       DestinationSpec `json:"spec" yaml:"spec"`
}
type DestinationSpec struct {
	Type           string               `json:"type" yaml:"type"`
	URLRef         *SecretRef           `json:"urlRef,omitempty" yaml:"urlRef,omitempty"`
	Headers        map[string]SecretRef `json:"headers,omitempty" yaml:"headers,omitempty"`
	MaxAttempts    int                  `json:"maxAttempts" yaml:"maxAttempts"`
	MaxAge         string               `json:"maxAge" yaml:"maxAge"`
	InitialBackoff string               `json:"initialBackoff" yaml:"initialBackoff"`
}

// Revision is independent of map iteration order. Callers must normalize first.
func Revision(value any) (string, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(b)), nil
}

// Entity identity includes only declared grouping fields, preserving scalar type
// and field presence. Nested objects/arrays are rejected by the compiler/runtime.
func EntityKey(groupBy []string, fields map[string]any) (string, error) {
	type part struct {
		Field   string `json:"field"`
		Present bool   `json:"present"`
		Value   any    `json:"value"`
	}
	parts := make([]part, 0, len(groupBy))
	for _, field := range groupBy {
		value, present := fields[field]
		switch v := value.(type) {
		case nil, bool, int, int64, float64, json.Number:
		case string:
			if !utf8.ValidString(v) {
				return "", fmt.Errorf("group value is not valid UTF-8")
			}
		default:
			return "", fmt.Errorf("group fields must be scalar values")
		}
		parts = append(parts, part{field, present, value})
	}
	b, err := json.Marshal(parts)
	return string(b), err
}

// AcceptedAt, Sequence and InputID are assigned at the transactional boundary.
// ObservedAt is provider evidence and never substitutes for accepted time.
type Observation struct {
	Sequence   int64          `json:"sequence"`
	InputID    string         `json:"inputId"`
	AcceptedAt time.Time      `json:"acceptedAt"`
	ObservedAt *time.Time     `json:"observedAt,omitempty"`
	Fields     map[string]any `json:"fields,omitempty"`
	Health     string         `json:"health"`           // ok, unknown, unchanged, gap, timer
	Detail     string         `json:"detail,omitempty"` // redacted diagnostic code
}

type Event struct {
	ID       string         `json:"id"`
	Sequence int64          `json:"sequence"`
	WatchID  string         `json:"watchId"`
	Revision string         `json:"revision"`
	Entity   string         `json:"entity"`
	Type     string         `json:"type"`
	At       time.Time      `json:"at"`
	Message  string         `json:"message"`
	Fields   map[string]any `json:"fields,omitempty"`
	Evidence []int64        `json:"evidence,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type Envelope struct {
	APIVersion string `json:"apiVersion"`
	Data       any    `json:"data,omitempty"`
	Error      *Error `json:"error,omitempty"`
}
