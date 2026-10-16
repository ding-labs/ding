package store

// ChangeVersion and Changes are process-local scheduling hints for the sole
// execution owner. They never replace database state or authorization. Read the
// version BEFORE the snapshot it describes, so a concurrent commit forces refresh.
// Restart and periodic reconciliation reconstruct scheduling from durable rows.
func (s *Store) ChangeVersion() uint64    { return s.version.Load() }
func (s *Store) Changes() <-chan struct{} { return s.wake }

func (s *Store) signalCommit(changed bool) {
	if !changed {
		return
	}
	// A commit error can have an uncertain durable outcome. Wake readers in that
	// case too; they will read SQL, never assume this hint means the write succeeded.
	s.version.Add(1)
	select {
	case s.wake <- struct{}{}:
	default:
	}
}
