package notify

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/ding-labs/ding/internal/notificationstyle"
	"github.com/godbus/dbus/v5"
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

func TestDesktopBellSurvivesDBusWireEncoding(t *testing.T) {
	hints := imageHints(notificationstyle.Error)
	want := hints["image-data"].Value().(notificationImage)
	message := dbus.Message{
		Type: dbus.TypeMethodCall,
		Headers: map[dbus.HeaderField]dbus.Variant{
			dbus.FieldPath:      dbus.MakeVariant(dbus.ObjectPath("/org/freedesktop/Notifications")),
			dbus.FieldMember:    dbus.MakeVariant("Notify"),
			dbus.FieldSignature: dbus.MakeVariant(dbus.SignatureOf(hints)),
		},
		Body: []any{hints},
	}
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		var wire bytes.Buffer
		if err := message.EncodeTo(&wire, order); err != nil {
			t.Fatal(err)
		}
		received, err := dbus.DecodeMessage(&wire)
		if err != nil {
			t.Fatal(err)
		}
		variant := received.Body[0].(map[string]dbus.Variant)["image-data"]
		if variant.Signature().String() != "(iiibiiay)" {
			t.Fatal("wrong image signature on the wire", variant.Signature())
		}
		var got notificationImage
		if err := dbus.Store([]any{variant.Value()}, &got); err != nil {
			t.Fatal(err)
		}
		if got.Width != want.Width || got.Height != want.Height || got.RowStride != want.RowStride || got.HasAlpha != want.HasAlpha || got.BitsPerSample != want.BitsPerSample || got.Channels != want.Channels || !bytes.Equal(got.Data, want.Data) {
			t.Fatal("status image changed during D-Bus transport")
		}
	}
}
