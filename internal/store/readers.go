package store

import (
	"context"
	"database/sql"
	"net/url"
	"time"
)

// Bound simultaneous snapshots across stores as well as per workspace. Each
// idle reader has a small page cache; 100 workspaces must not multiply large pools.
var snapshotSlots = make(chan struct{}, 16)

func openSnapshotReaders(ctx context.Context, uri url.URL) (*sql.DB, error) {
	q := uri.Query()
	q.Set("mode", "ro")
	q.Add("_pragma", "query_only(1)")
	q.Add("_pragma", "cache_size(-256)")
	uri.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxIdleTime(time.Minute)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
