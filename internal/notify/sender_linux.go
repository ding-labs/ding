package notify

import (
	"context"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const desktopBus = "org.freedesktop.Notifications"

type pendingClick struct {
	target Target
	at     time.Time
}

type desktopSender struct {
	mu      sync.Mutex
	ctx     context.Context
	state   string
	conn    *dbus.Conn
	owner   string
	pending map[uint32]pendingClick
	open    func(context.Context, Target)
}

// Keep one session connection for the daemon lifetime so ActionInvoked can
// arrive after the outbox delivery call has returned. Headless startup does not
// require a bus: connection is lazy and delivery failures remain visible.
func NewSender(ctx context.Context, stateDir string) (func(context.Context, Message) error, func()) {
	ctx, cancel := context.WithCancel(ctx)
	executable, _ := os.Executable()
	executable, _ = filepath.EvalSymlinks(executable)
	s := &desktopSender{ctx: ctx, state: stateDir, pending: make(map[uint32]pendingClick)}
	s.open = func(ctx context.Context, target Target) {
		if executable == "" {
			return
		}
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		// No shell and no handoff token in the desktop message or child logs.
		_ = exec.CommandContext(ctx, executable, "notification-open", "--", ActivationURL(target)).Run()
	}
	return s.send, func() {
		cancel()
		s.mu.Lock()
		conn := s.conn
		s.mu.Unlock()
		if conn != nil {
			_ = conn.Close()
		}
	}
}

func (s *desktopSender) connect(ctx context.Context) error {
	if s.conn != nil && s.conn.Connected() {
		return nil
	}
	address := os.Getenv("DBUS_SESSION_BUS_ADDRESS")
	if address == "" {
		address = fmt.Sprintf("unix:path=/run/user/%d/bus", os.Getuid())
	}
	conn, err := dbus.Connect(address, dbus.WithContext(s.ctx))
	if err != nil {
		return fmt.Errorf("desktop session unavailable")
	}
	if err := conn.AddMatchSignalContext(ctx, dbus.WithMatchSender(desktopBus), dbus.WithMatchObjectPath("/org/freedesktop/Notifications"), dbus.WithMatchInterface(desktopBus)); err != nil {
		conn.Close()
		return fmt.Errorf("desktop notification actions unavailable")
	}
	s.conn = conn
	s.pending = make(map[uint32]pendingClick)
	signals := make(chan *dbus.Signal, 32)
	conn.Signal(signals)
	go func() {
		for signal := range signals {
			s.mu.Lock()
			target, ok := s.action(conn, signal, time.Now())
			s.mu.Unlock()
			if ok {
				s.open(s.ctx, target)
			}
		}
	}()
	return nil
}

func (s *desktopSender) send(ctx context.Context, m Message) error {
	if err := validate(m); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.connect(ctx); err != nil {
		return err
	}
	var owner string
	if err := s.conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.GetNameOwner", 0, desktopBus).Store(&owner); err != nil {
		return fmt.Errorf("desktop notification server unavailable")
	}
	if s.owner != owner {
		s.pending = make(map[uint32]pendingClick)
		s.owner = owner
	}
	object := s.conn.Object(desktopBus, "/org/freedesktop/Notifications")
	var capabilities []string
	_ = object.CallWithContext(ctx, desktopBus+".GetCapabilities", 0).Store(&capabilities)
	var actions []string
	target := Target{s.state, m.WatchID}
	if target.valid() && slices.Contains(capabilities, "actions") {
		actions = []string{"default", "Open watch"}
	}
	var id uint32
	err := object.CallWithContext(ctx, desktopBus+".Notify", 0, "Ding", uint32(0), "", m.Title, html.EscapeString(m.Body), actions, map[string]dbus.Variant{}, int32(-1)).Store(&id)
	if err != nil || id == 0 {
		return fmt.Errorf("desktop notification was not accepted")
	}
	if len(actions) > 0 {
		s.remember(id, target, time.Now())
	}
	return nil
}

func (s *desktopSender) remember(id uint32, target Target, now time.Time) {
	var oldest uint32
	for key, item := range s.pending {
		if now.Sub(item.at) > 24*time.Hour {
			delete(s.pending, key)
			continue
		}
		if oldest == 0 || item.at.Before(s.pending[oldest].at) {
			oldest = key
		}
	}
	if len(s.pending) >= 1024 {
		delete(s.pending, oldest)
	}
	s.pending[id] = pendingClick{target, now}
}

func (s *desktopSender) action(conn *dbus.Conn, signal *dbus.Signal, now time.Time) (Target, bool) {
	if s.conn != conn || signal == nil || signal.Sender != s.owner || signal.Path != "/org/freedesktop/Notifications" || len(signal.Body) != 2 {
		return Target{}, false
	}
	id, ok := signal.Body[0].(uint32)
	if !ok {
		return Target{}, false
	}
	item, exists := s.pending[id]
	switch signal.Name {
	case desktopBus + ".NotificationClosed":
		delete(s.pending, id)
	case desktopBus + ".ActionInvoked":
		if signal.Body[1] == "default" && exists {
			delete(s.pending, id)
			return item.target, now.Sub(item.at) <= 24*time.Hour
		}
	}
	return Target{}, false
}
