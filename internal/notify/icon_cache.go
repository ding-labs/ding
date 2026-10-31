package notify

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ding-labs/ding/design"
	"github.com/ding-labs/ding/internal/notificationstyle"
)

var notificationIconCacheMu sync.Mutex

func notificationIconPath(status notificationstyle.Status) string {
	root, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return cacheNotificationIcon(filepath.Join(root, "Ding", "notifications"), status)
}

// Toasts may load their image after the helper returns. Keep a persistent local
// copy, named by content so new artwork cannot leave an older cached bell.
// Cache failures degrade to a text notification rather than blocking delivery.
func cacheNotificationIcon(directory string, status notificationstyle.Status) string {
	data := design.NotificationIcon(status.IconColor())
	if len(data) == 0 {
		return ""
	}
	// Windows readers can prevent replacement of an open file. Publish each
	// image once before concurrent deliveries start opening the cached path.
	notificationIconCacheMu.Lock()
	defer notificationIconCacheMu.Unlock()
	digest := sha256.Sum256(data)
	path := filepath.Join(directory, fmt.Sprintf("%s-%x.png", status.IconColor(), digest[:8]))
	if cached, err := os.ReadFile(path); err == nil && bytes.Equal(cached, data) {
		return path
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return ""
	}
	file, err := os.CreateTemp(directory, ".bell-*.png")
	if err != nil {
		return ""
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return ""
	}
	if err := file.Close(); err != nil {
		return ""
	}
	if err := os.Rename(file.Name(), path); err != nil {
		// Another process may have published the same complete image while this
		// one was being written, or may currently have that image open.
		if cached, readErr := os.ReadFile(path); readErr != nil || !bytes.Equal(cached, data) {
			return ""
		}
	}
	return path
}
