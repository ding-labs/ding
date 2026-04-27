package config

import (
	"os"
	"strings"
	"testing"
)

func TestExpandEnvVars(t *testing.T) {
	cases := []struct {
		name          string
		input         string
		env           map[string]string // vars to set with t.Setenv
		unset         []string          // vars to ensure unset for this case
		want          string            // expected output (only checked when wantErr==false)
		wantErr       bool
		errParts      []string       // substrings that must appear in err.Error() if wantErr
		wantErrCounts map[string]int // exact occurrence counts of substrings in err.Error() (for dedup proof)
	}{
		{
			name:  "plain text passes through",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "single substitution",
			input: "hi ${NAME}",
			env:   map[string]string{"NAME": "bob"},
			want:  "hi bob",
		},
		{
			name:  "multiple distinct vars",
			input: "${A}-${B}",
			env:   map[string]string{"A": "x", "B": "y"},
			want:  "x-y",
		},
		{
			name:  "repeated var",
			input: "${A} ${A}",
			env:   map[string]string{"A": "x"},
			want:  "x x",
		},
		{
			name:  "empty value (set to empty string) is allowed",
			input: "x=${A}",
			env:   map[string]string{"A": ""},
			want:  "x=",
		},
		{
			name:     "unset var produces error",
			input:    "${T2A_TEST_MISSING}",
			unset:    []string{"T2A_TEST_MISSING"},
			wantErr:  true,
			errParts: []string{"unset env vars", "T2A_TEST_MISSING"},
		},
		{
			name:     "multiple unset vars accumulated",
			input:    "${T2A_TEST_A} ${T2A_TEST_B}",
			unset:    []string{"T2A_TEST_A", "T2A_TEST_B"},
			wantErr:  true,
			errParts: []string{"T2A_TEST_A", "T2A_TEST_B"},
		},
		{
			name:          "unset listed once even if repeated",
			input:         "${T2A_TEST_X} ${T2A_TEST_X} ${T2A_TEST_X}",
			unset:         []string{"T2A_TEST_X"},
			wantErr:       true,
			errParts:      []string{"T2A_TEST_X"},
			wantErrCounts: map[string]int{"T2A_TEST_X": 1}, // dedup proof: var name appears exactly once
		},
		{
			name:     "error message lists unset vars sorted",
			input:    "${T2A_TEST_ZEBRA} ${T2A_TEST_APPLE}",
			unset:    []string{"T2A_TEST_ZEBRA", "T2A_TEST_APPLE"},
			wantErr:  true,
			errParts: []string{"T2A_TEST_APPLE, T2A_TEST_ZEBRA"}, // sorted alpha
			// dedup proof: each var appears exactly once even though both could
			// theoretically be repeated by a buggy implementation
			wantErrCounts: map[string]int{"T2A_TEST_APPLE": 1, "T2A_TEST_ZEBRA": 1},
		},
		{
			name:  "invalid name (dash) passes through unchanged",
			input: "${A-B}",
			want:  "${A-B}",
		},
		{
			name:  "empty braces pass through unchanged",
			input: "${}",
			want:  "${}",
		},
		{
			name:  "bare $VAR not expanded (braces required)",
			input: "hi $NAME",
			env:   map[string]string{"NAME": "bob"},
			want:  "hi $NAME",
		},
		{
			name:  "adjacent text",
			input: "https://${HOST}/x",
			env:   map[string]string{"HOST": "ex.com"},
			want:  "https://ex.com/x",
		},
		{
			name:  "YAML special chars in value preserved verbatim (documents footgun)",
			input: "k: ${V}",
			env:   map[string]string{"V": "a: b\nc: d"},
			want:  "k: a: b\nc: d",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, name := range tc.unset {
				// t.Setenv registers cleanup that restores the prior value;
				// we then unset within the test body. The cleanup will restore
				// whatever the var was before the test ran (typically unset).
				t.Setenv(name, "sentinel")
				if err := os.Unsetenv(name); err != nil {
					t.Fatalf("os.Unsetenv(%q): %v", name, err)
				}
			}
			for k, v := range tc.env {
				t.Setenv(k, v)
			}

			got, err := expandEnvVars([]byte(tc.input))

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil; got=%q", string(got))
				}
				for _, part := range tc.errParts {
					if !strings.Contains(err.Error(), part) {
						t.Errorf("err missing %q; got: %v", part, err)
					}
				}
				for substr, want := range tc.wantErrCounts {
					if got := strings.Count(err.Error(), substr); got != want {
						t.Errorf("err count of %q: got %d, want %d (err: %v)", substr, got, want, err)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("got %q, want %q", string(got), tc.want)
			}
		})
	}
}
