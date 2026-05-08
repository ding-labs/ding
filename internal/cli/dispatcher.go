package cli

import (
	"log"

	"github.com/ding-labs/ding/internal/evaluator"
	"github.com/ding-labs/ding/internal/notifier"
)

// Dispatcher routes alerts to their final destination. Implementations live
// here (NotifierDispatcher — production sends) and in internal/dryrun
// (LoggingDispatcher — preview-only, no sends).
type Dispatcher interface {
	Dispatch(alerts []evaluator.Alert)
}

// NotifierDispatcher is the production Dispatcher: writes each alert to the
// alert log (if configured) then calls Send on each named notifier.
type NotifierDispatcher struct {
	Notifiers   map[string]notifier.Notifier
	AlertLogger *notifier.AlertLogger
}

func (d *NotifierDispatcher) Dispatch(alerts []evaluator.Alert) {
	for _, alert := range alerts {
		if d.AlertLogger != nil {
			if err := d.AlertLogger.Log(alert); err != nil {
				log.Printf("ding: alert log write error: %v", err)
			}
		}
		for _, name := range alert.Notifiers {
			n, ok := d.Notifiers[name]
			if !ok {
				log.Printf("ding: unknown notifier %q for rule %q", name, alert.Rule)
				continue
			}
			if err := n.Send(alert); err != nil {
				log.Printf("ding: notifier %q error: %v", name, err)
			}
		}
	}
}
