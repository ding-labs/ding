package cloud

import "testing"

func TestPushClockDiscontinuityQualifiesElapsedTime(t *testing.T) {
	for _, tc := range []struct {
		wall, elapsed float64
		jump          bool
	}{{180, 180, false}, {360, 360, false}, {207, 180, true}, {153, 180, true}, {182, 180, false}} {
		if got := pushClockDiscontinuity(tc.wall, tc.elapsed); got != tc.jump {
			t.Fatalf("wall=%v elapsed=%v jump=%v", tc.wall, tc.elapsed, got)
		}
	}
}
