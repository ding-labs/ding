//go:build !windows

package update

import "os"

func syncAssets(paths ...string) error {
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		err = f.Sync()
		closed := f.Close()
		if err != nil {
			return err
		}
		if closed != nil {
			return closed
		}
	}
	return nil
}
