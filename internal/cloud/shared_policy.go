package cloud

import (
	"fmt"
	"slices"

	"github.com/ding-labs/ding/internal/plan"
)

func sharedPolicy(commands []string) func(plan.Bundle) error {
	return func(b plan.Bundle) error {
		for _, w := range b.Watches {
			switch w.Definition.Spec.Source.Type {
			case "http", "push":
			case "command":
				if !slices.Contains(commands, w.Revision) {
					return fmt.Errorf("command revision requires host-operator approval")
				}
			default:
				return fmt.Errorf("source type is not approved for shared execution")
			}
		}
		for _, d := range b.Destinations {
			if !slices.Contains([]string{"console", "webhook", "slack", "discord"}, d.Definition.Spec.Type) {
				return fmt.Errorf("destination type is unavailable on a shared server")
			}
		}
		return nil
	}
}
