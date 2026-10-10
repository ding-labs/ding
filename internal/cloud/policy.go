package cloud

import (
	"github.com/ding-labs/ding/internal/hostingpolicy"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/watchrun"
)

const MaxManifestBytes = hostingpolicy.MaxManifestBytes

func Limits() watchrun.Limits         { return hostingpolicy.Limits() }
func Policy(bundle plan.Bundle) error { return hostingpolicy.Policy(bundle) }
