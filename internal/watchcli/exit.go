package watchcli

import "errors"

// ExitStatus preserves the status of an explicitly wrapped application.
type ExitStatus struct{ Code int }

func (e *ExitStatus) Error() string { return "application exited" }

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var status *ExitStatus
	if errors.As(err, &status) && status.Code > 0 && status.Code < 256 {
		return status.Code
	}
	return 1
}
