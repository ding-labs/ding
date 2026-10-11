package logs

// Statistics are cumulative for the current collector session, including the
// partial final second that has not entered a completed publication window.
type Statistics struct {
	Records      int64 `json:"records"`
	Warnings     int64 `json:"warnings"`
	Errors       int64 `json:"errors"`
	Unclassified int64 `json:"unclassified"`
	Invalid      int64 `json:"invalid"`
	Dropped      int64 `json:"dropped"`
}

func (c *Collector) Statistics() Statistics {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.statistics
}
