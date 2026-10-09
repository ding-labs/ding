package watchcli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestConsoleCommandInventory(t *testing.T) {
	data, err := os.ReadFile("../../testdata/console/parity.json")
	if err != nil {
		t.Fatal(err)
	}
	var inventory map[string]string
	if err := json.Unmarshal(data, &inventory); err != nil {
		t.Fatal(err)
	}
	root := Root("test")
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		if c.Runnable() {
			path := strings.TrimPrefix(c.CommandPath(), "ding ")
			if inventory[path] == "" {
				t.Errorf("command %q needs a console parity decision", path)
			}
			delete(inventory, path)
		}
		for _, child := range c.Commands() {
			walk(child)
		}
	}
	walk(root)
	for path := range inventory {
		t.Errorf("stale console command %q", path)
	}
}
