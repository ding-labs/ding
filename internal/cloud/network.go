package cloud

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/ding-labs/ding/internal/cloud/state"
)

type budgetTransport struct {
	next    http.RoundTripper
	db      *state.DB
	account string
}

func (b budgetTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method != "GET" && r.Method != "POST" {
		return nil, fmt.Errorf("cloud outbound method denied")
	}
	if r.ContentLength < 0 || r.ContentLength > 64<<10 || headerSize(r.Header) > 8<<10 {
		return nil, fmt.Errorf("cloud outbound request exceeds limit")
	}
	reservation, err := b.db.Reserve(r.Context(), b.account, r.Method == "POST", time.Now())
	if err != nil {
		return nil, err
	}
	finish := func(used int64) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		// A failed refund leaves the conservative reservation charged, never free I/O.
		_ = b.db.Finish(ctx, reservation, used)
	}
	used := r.ContentLength + headerSize(r.Header) + 1024
	response, err := b.next.RoundTrip(r)
	if err != nil {
		finish(used)
		return nil, err
	}
	used += headerSize(response.Header)
	if used > 96<<10 {
		response.Body.Close()
		finish(state.RequestReservation)
		return nil, fmt.Errorf("cloud response headers exceed limit")
	}
	response.Body = &chargedBody{ReadCloser: response.Body, used: used, finish: finish}
	return response, nil
}

func headerSize(h http.Header) int64 {
	var size int64
	for name, values := range h {
		for _, value := range values {
			size += int64(len(name) + len(value) + 4)
		}
	}
	return size
}

type chargedBody struct {
	io.ReadCloser
	read, used int64
	once       sync.Once
	finish     func(int64)
}

func (b *chargedBody) Read(p []byte) (int, error) {
	remaining := int64(64<<10) + 1 - b.read
	if remaining <= 0 {
		return 0, fmt.Errorf("cloud response body exceeds 64 KiB")
	}
	if int64(len(p)) > remaining {
		p = p[:remaining]
	}
	n, err := b.ReadCloser.Read(p)
	b.read += int64(n)
	b.used += int64(n)
	if b.read > 64<<10 {
		return n, fmt.Errorf("cloud response body exceeds 64 KiB")
	}
	return n, err
}
func (b *chargedBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(func() { b.finish(b.used) })
	return err
}
