package egress

import (
	"io"
	"net/http"
	"sync"
)

// Limit bounds all active outbound requests, including response body reads.
// Tenant engines each have one acquisition and one delivery worker, so a noisy
// tenant cannot occupy every slot in this shared admission queue.
type Limit struct {
	next  http.RoundTripper
	slots chan struct{}
}

func NewLimit(next http.RoundTripper, concurrency int) *Limit {
	if concurrency < 1 {
		panic("egress concurrency must be positive")
	}
	return &Limit{next: next, slots: make(chan struct{}, concurrency)}
}
func (l *Limit) RoundTrip(r *http.Request) (*http.Response, error) {
	select {
	case l.slots <- struct{}{}:
	case <-r.Context().Done():
		return nil, r.Context().Err()
	}
	response, err := l.next.RoundTrip(r)
	if err != nil {
		<-l.slots
		return nil, err
	}
	response.Body = &leasedBody{ReadCloser: response.Body, release: func() { <-l.slots }}
	return response, nil
}

type leasedBody struct {
	io.ReadCloser
	once    sync.Once
	release func()
}

func (b *leasedBody) Close() error { err := b.ReadCloser.Close(); b.once.Do(b.release); return err }
