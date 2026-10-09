package migrate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ding-labs/ding/internal/watch"
)

// Write uses a new private directory. Existing output is never overwritten, and
// a failed export removes only files created by this invocation.
func Write(dir string, result Result) error {
	if err := os.Mkdir(dir, 0700); err != nil {
		return fmt.Errorf("migration output must be a new directory in an existing writable parent")
	}
	created := []string{}
	complete := false
	defer func() {
		if !complete {
			for _, path := range created {
				_ = os.Remove(path)
			}
			_ = os.Remove(dir)
		}
	}()
	files := append([]File(nil), result.Files...)
	report, err := json.MarshalIndent(watch.Envelope{APIVersion: watch.APIVersion, Data: result.Report}, "", "  ")
	if err != nil {
		return err
	}
	files = append(files, File{"report.json", append(report, '\n')})
	for _, file := range files {
		if filepath.Base(file.Name) != file.Name || file.Name == "." || file.Name == ".." {
			return fmt.Errorf("invalid migration output name")
		}
		path := filepath.Join(dir, file.Name)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		created = append(created, path)
		_, err = f.Write(file.Content)
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	complete = true
	return nil
}
