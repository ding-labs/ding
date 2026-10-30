package notify

import (
	"testing"

	"github.com/ding-labs/ding/internal/notificationstyle"
)

func TestDesktopBellsCarryFreedesktopRGBAImages(t *testing.T) {
	for _, status := range []notificationstyle.Status{notificationstyle.Warning, notificationstyle.Error, notificationstyle.Recovered} {
		t.Run(string(status), func(t *testing.T) {
			hints := imageHints(status)
			v, ok := hints["image-data"]
			if !ok || v.Signature().String() != "(iiibiiay)" {
				t.Fatal("missing or invalid freedesktop image-data", hints)
			}
			pixels := v.Value().(notificationImage)
			if pixels.Width != 512 || pixels.Height != 512 || pixels.RowStride != 2048 || !pixels.HasAlpha || pixels.BitsPerSample != 8 || pixels.Channels != 4 || len(pixels.Data) != 512*512*4 {
				t.Fatal("invalid RGBA image layout")
			}
			// Sample the bell away from its gray center and dark outlines.
			i := 150*int(pixels.RowStride) + 256*4
			r, g, b, a := pixels.Data[i], pixels.Data[i+1], pixels.Data[i+2], pixels.Data[i+3]
			if a != 255 || (status == notificationstyle.Error && r <= g) || (status == notificationstyle.Warning && (r <= b || g <= b)) || (status == notificationstyle.Recovered && (g <= r || g <= b)) {
				t.Fatal("wrong bell color", r, g, b, a)
			}
		})
	}
	for _, status := range []notificationstyle.Status{"", "untrusted/../../icon"} {
		if len(imageHints(status)) != 0 {
			t.Fatal("unrecognized status selected artwork")
		}
	}
}
