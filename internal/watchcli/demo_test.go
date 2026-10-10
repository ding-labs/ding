package watchcli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestDemoAcquiresRealTransitionsWithoutUsingInstallationState(t *testing.T) {
	if testing.Short() {
		t.Skip("real scheduler transitions")
	}
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	if err := runDemo(ctx, cmd, false, 5*time.Second); err != nil {
		t.Fatal(err, out.String())
	}
	if !strings.Contains(out.String(), "Demo recorded a firing and a recovery") {
		t.Fatal(out.String())
	}
}
