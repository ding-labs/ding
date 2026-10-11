package logs

import "os"

func openLogFile(string) (*os.File, error) { return nil, os.ErrInvalid }
func fileIdentity(os.FileInfo) string      { return "unsupported" }
