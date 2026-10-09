package main

import (
	"testing"
	"time"
)

func TestLatencyHistogramUpperBound(t *testing.T) {
	var h histogram
	for i := 0; i < 95; i++ {
		h.add(4200 * time.Microsecond)
	}
	for i := 0; i < 5; i++ {
		h.add(time.Second)
	}
	if h.p95() != 5 {
		t.Fatal(h.p95())
	}
	h.add(time.Hour)
	if h.Counts[10000] != 1 {
		t.Fatal("overflow not bounded")
	}
}
func TestMedian(t *testing.T) {
	if median([]int64{90, 10, 20}) != 20 {
		t.Fatal("median")
	}
}

func TestMemoryMeasuresHighWaterMark(t *testing.T) {
	rss, peak, err := parseMemory("Name:\tding\nVmHWM:\t40000 kB\nVmRSS:\t30000 kB\n")
	if err != nil || rss != 30000*1024 || peak != 40000*1024 {
		t.Fatal(rss, peak, err)
	}
	for _, bad := range []string{"", "VmRSS: 10 kB", "VmRSS: 20 kB\nVmHWM: 10 kB", "VmRSS: text kB\nVmHWM: 30 kB"} {
		if _, _, err := parseMemory(bad); err == nil {
			t.Fatal("invalid memory accepted", bad)
		}
	}
}
