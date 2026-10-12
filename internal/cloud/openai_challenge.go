package cloud

import (
	"fmt"
	"io"
	"net/http"
)

// The submission portal requires one exact plain-text token at this origin.
// This public proof of domain control is configured by the operator, never by
// a tenant or an incoming request. It is not an authentication credential.
func validateOpenAIChallenge(token string) error {
	if len(token) > 4096 {
		return fmt.Errorf("OpenAI domain challenge exceeds 4096 bytes")
	}
	for _, c := range token {
		if c < 33 || c > 126 {
			return fmt.Errorf("OpenAI domain challenge must contain only printable non-whitespace ASCII")
		}
	}
	return nil
}

func (s *Server) openAIChallenge(w http.ResponseWriter, r *http.Request) {
	if s.OpenAIAppsChallenge == "" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if r.Method == http.MethodGet {
		_, _ = io.WriteString(w, s.OpenAIAppsChallenge)
	}
}
