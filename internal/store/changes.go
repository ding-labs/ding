package store

// ChangeVersion and Changes are process-local hints for the sole execution
// owner. Capture Changes BEFORE reading work so a racing commit cannot be lost.
func (s *Store) ChangeVersion() uint64 { return s.version.Load() }
func (s *Store) Changes() <-chan struct{} {
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	return s.wake
}

func (s *Store) signalCommit(changed bool) {
	if !changed {
		return
	}
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	// A commit error may have an uncertain durable outcome. Wake all readers to
	// recheck SQL even then. A shared buffered token would wake only one worker.
	s.version.Add(1)
	close(s.wake)
	s.wake = make(chan struct{})
}
