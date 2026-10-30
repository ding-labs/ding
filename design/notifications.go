// Package design embeds the shared notification artwork for offline desktops.
package design

import "embed"

//go:embed assets/notifications/*.png
var notificationIcons embed.FS

func NotificationIcon(color string) []byte {
	switch color {
	case "green", "yellow", "red":
		data, _ := notificationIcons.ReadFile("assets/notifications/" + color + ".png")
		return data
	}
	return nil
}
