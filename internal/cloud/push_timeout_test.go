package cloud

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/watchrun"
)

func TestPushPreparationTimeoutIsRetryable(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		w := httptest.NewRecorder()
		new(Server).pushError(w, fmt.Errorf("%w: %w", watchrun.ErrInput, cause))
		if w.Code != 503 || !strings.Contains(w.Body.String(), `"acceptance_uncertain"`) {
			t.Fatalf("transient preparation failure became permanent: %d %s", w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	new(Server).pushError(w, fmt.Errorf("%w: malformed JSON", watchrun.ErrInput))
	if w.Code != 400 || !strings.Contains(w.Body.String(), `"invalid_input"`) {
		t.Fatalf("malformed input classification changed: %d %s", w.Code, w.Body.String())
	}
}
