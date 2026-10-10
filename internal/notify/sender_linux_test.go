package notify

import (
	"bufio"
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

func TestNotificationActionsAreBoundedAndBoundToTheirSender(t *testing.T) {
	now := time.Now()
	s := &desktopSender{owner: ":1.42", pending: make(map[uint32]pendingClick)}
	target := Target{t.TempDir(), "my-watch"}
	s.remember(5, target, now)
	signal := &dbus.Signal{Sender: ":1.99", Path: "/org/freedesktop/Notifications", Name: desktopBus + ".ActionInvoked", Body: []any{uint32(5), "default"}}
	if _, ok := s.action(nil, signal, now); ok {
		t.Fatal("accepted forged desktop sender")
	}
	signal.Sender = s.owner
	signal.Body[1] = "arbitrary-command"
	if _, ok := s.action(nil, signal, now); ok {
		t.Fatal("accepted unrecognized action")
	}
	signal.Body[1] = "default"
	if got, ok := s.action(nil, signal, now); !ok || got != target {
		t.Fatal("click did not open the original target")
	}
	if _, ok := s.action(nil, signal, now); ok {
		t.Fatal("duplicate click opened again")
	}
	s.remember(5, target, now)
	if _, ok := s.action(nil, signal, now.Add(25*time.Hour)); ok {
		t.Fatal("expired notification opened")
	}
	s.remember(5, target, now)
	signal.Name = desktopBus + ".NotificationClosed"
	signal.Body[1] = uint32(2)
	s.action(nil, signal, now)
	if len(s.pending) != 0 {
		t.Fatal("dismissed notification retained")
	}
	for i := uint32(1); i <= 2000; i++ {
		s.remember(i, target, now.Add(time.Duration(i)*time.Second))
	}
	if len(s.pending) != 1024 {
		t.Fatal("unbounded notification targets")
	}
	s.remember(3000, target, now.Add(48*time.Hour))
	if len(s.pending) != 1 {
		t.Fatal("expired targets retained")
	}
}

type notificationFixture struct{}

func (notificationFixture) GetCapabilities() ([]string, *dbus.Error) {
	return []string{"actions", "body"}, nil
}
func (notificationFixture) Notify(_ string, _ uint32, _ string, _ string, _ string, _ []string, _ map[string]dbus.Variant, _ int32) (uint32, *dbus.Error) {
	return 42, nil
}

func TestDesktopActionAfterDeliveryReturnsOnPrivateBus(t *testing.T) {
	path, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("private D-Bus test needs dbus-daemon")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	process := exec.CommandContext(ctx, path, "--session", "--nofork", "--print-address=1")
	stdout, err := process.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Process.Kill(); _ = process.Wait() }()
	address, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	address = strings.TrimSpace(address)
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", address)
	server, err := dbus.Connect(address, dbus.WithContext(ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if _, err := server.RequestName(desktopBus, dbus.NameFlagDoNotQueue); err != nil {
		t.Fatal(err)
	}
	if err := server.Export(notificationFixture{}, "/org/freedesktop/Notifications", desktopBus); err != nil {
		t.Fatal(err)
	}
	opened := make(chan Target, 2)
	s := &desktopSender{ctx: ctx, state: t.TempDir(), pending: make(map[uint32]pendingClick), open: func(_ context.Context, target Target) { opened <- target }}
	if err := s.send(ctx, Message{ID: "fixture", WatchID: "my-watch", Title: "Fixture"}); err != nil {
		t.Fatal(err)
	}
	defer s.conn.Close()
	// Delivery has returned; its original short-lived context is unnecessary.
	if err := server.Emit("/org/freedesktop/Notifications", desktopBus+".ActionInvoked", uint32(42), "default"); err != nil {
		t.Fatal(err)
	}
	select {
	case target := <-opened:
		if target.StateDir != s.state || target.WatchID != "my-watch" {
			t.Fatal("wrong activation target")
		}
	case <-ctx.Done():
		t.Fatal("action did not arrive after delivery")
	}
}
