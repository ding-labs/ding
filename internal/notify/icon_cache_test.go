package notify

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ding-labs/ding/design"
	"github.com/ding-labs/ding/internal/notificationstyle"
)

func TestToastImagesRemainAvailableAfterConcurrentDelivery(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "icons with spaces")
	for _, status := range []notificationstyle.Status{notificationstyle.Warning, notificationstyle.Error, notificationstyle.Recovered} {
		var senders sync.WaitGroup
		for range 8 {
			senders.Go(func() {
				path := cacheNotificationIcon(directory, status)
				data, err := os.ReadFile(path)
				if path == "" || err != nil || !bytes.Equal(data, design.NotificationIcon(status.IconColor())) {
					t.Errorf("unavailable toast image for %s: %q, %v", status, path, err)
				}
			})
		}
		senders.Wait()
	}
	files, err := os.ReadDir(directory)
	if err != nil || len(files) != 3 {
		t.Fatal("temporary image files leaked", files, err)
	}
}

func TestToastImageFailuresFallBackToText(t *testing.T) {
	directory := t.TempDir()
	for _, status := range []notificationstyle.Status{"", "../../red"} {
		if path := cacheNotificationIcon(directory, status); path != "" {
			t.Fatal("unrecognized status selected a file", path)
		}
	}
	blocked := filepath.Join(directory, "file")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	if path := cacheNotificationIcon(filepath.Join(blocked, "icons"), notificationstyle.Error); path != "" {
		t.Fatal("unwritable cache returned a broken image path", path)
	}
}
