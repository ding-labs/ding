package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var ErrOwned = errors.New("PostgreSQL instance already has an active server or operator")
var ErrOwnershipLost = errors.New("PostgreSQL instance ownership lost")

// Ownership keeps a dedicated direct connection, separate from the bounded
// application query pool. The owner row fences in-flight writes across a lost
// session and replacement owner; a second process cannot enable its workers.
type Ownership struct {
	pool      *sql.DB
	conn      *sql.Conn
	pid       int
	nonce     string
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	lost      atomic.Bool
	closeOnce sync.Once
	closeErr  error
}

// TakeOwnership takes responsibility for closing dedicated, a pool with one
// connection. Do not pass the application's query pool or a transaction pooler.
func TakeOwnership(ctx context.Context, dedicated *sql.DB) (*Ownership, error) {
	conn, err := dedicated.Conn(ctx)
	if err != nil {
		dedicated.Close()
		return nil, SafeError("acquire ownership connection", err)
	}
	ok := false
	defer func() {
		if !ok {
			conn.Close()
			dedicated.Close()
		}
	}()
	var locked bool
	var pid int
	if err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(68247,5319),pg_backend_pid()").Scan(&locked, &pid); err != nil {
		return nil, SafeError("claim ownership", err)
	}
	if !locked {
		return nil, ErrOwned
	}
	var token [16]byte
	if _, err = rand.Read(token[:]); err != nil {
		return nil, err
	}
	nonce := hex.EncodeToString(token[:])
	result, err := conn.ExecContext(ctx, "UPDATE ding_meta.owner SET pid=$1,nonce=$2 WHERE singleton=1", pid, nonce)
	if err != nil {
		return nil, SafeError("publish ownership", err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return nil, ErrOwnershipLost
	}
	owned, cancel := context.WithCancel(ctx)
	o := &Ownership{pool: dedicated, conn: conn, pid: pid, nonce: nonce, ctx: owned, cancel: cancel, done: make(chan struct{})}
	ok = true
	go o.maintain()
	return o, nil
}

func (o *Ownership) Context() context.Context { return o.ctx }

func (o *Ownership) maintain() {
	defer close(o.done)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-o.ctx.Done():
			return
		case <-ticker.C:
			probe, cancel := context.WithTimeout(o.ctx, 2*time.Second)
			var pid int
			err := o.conn.QueryRowContext(probe, "SELECT pg_backend_pid()").Scan(&pid)
			cancel()
			if err != nil || pid != o.pid {
				o.lost.Store(true)
				o.cancel()
				return
			}
		}
	}
}

// Fence acquires the owner row before mutable control/workspace rows. A new
// owner must update this row, so it waits for every admitted write to finish.
// Recheck before commit; a lost connection cannot authorize later work.
func (o *Ownership) Fence(ctx context.Context, tx *sql.Tx, write bool) error {
	if o == nil {
		return nil
	}
	if o.lost.Load() {
		return ErrOwnershipLost
	}
	query := "SELECT pid,nonce FROM ding_meta.owner WHERE singleton=1"
	if write {
		query += " FOR SHARE"
	}
	var pid int
	var nonce string
	if err := tx.QueryRowContext(ctx, query).Scan(&pid, &nonce); err != nil {
		return SafeError("check ownership", err)
	}
	if pid != o.pid || nonce != o.nonce {
		return ErrOwnershipLost
	}
	var active bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND classid=68247 AND objid=5319 AND objsubid=2 AND pid=$1 AND granted)`, o.pid).Scan(&active); err != nil {
		return SafeError("check ownership session", err)
	}
	if !active {
		return ErrOwnershipLost
	}
	return nil
}

func (o *Ownership) Close() error {
	if o == nil {
		return nil
	}
	o.closeOnce.Do(func() {
		o.lost.Store(true)
		o.cancel()
		<-o.done
		// Closing the dedicated pool terminates its session and releases the lock.
		o.closeErr = errors.Join(o.conn.Close(), o.pool.Close())
	})
	return o.closeErr
}
