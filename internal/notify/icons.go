package notify

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"

	"github.com/ding-labs/ding/design"
	"github.com/ding-labs/ding/internal/notificationstyle"
	"github.com/godbus/dbus/v5"
)

type notificationImage struct {
	Width, Height, RowStride int32
	HasAlpha                 bool
	BitsPerSample, Channels  int32
	Data                     []byte
}

// The freedesktop image-data hint carries raw RGBA, not compressed PNG bytes.
// Embedding it avoids network access and keeps desktop alerts usable offline.
func imageHints(status notificationstyle.Status) map[string]dbus.Variant {
	hints := map[string]dbus.Variant{}
	icon := design.NotificationIcon(status.IconColor())
	if len(icon) == 0 {
		return hints
	}
	decoded, err := png.Decode(bytes.NewReader(icon))
	if err != nil {
		return hints
	}
	pixels := image.NewNRGBA(decoded.Bounds())
	draw.Draw(pixels, pixels.Bounds(), decoded, decoded.Bounds().Min, draw.Src)
	hints["image-data"] = dbus.MakeVariant(notificationImage{int32(pixels.Rect.Dx()), int32(pixels.Rect.Dy()), int32(pixels.Stride), true, 8, 4, pixels.Pix})
	return hints
}
