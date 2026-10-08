package control

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
)

type Credentials struct {
	Admin  string `json:"admin"`
	Ingest string `json:"ingest"`
}
type Connection struct {
	URL string `json:"url"`
}

func PrivateCredentials(dir string) (Credentials, error) {
	path := filepath.Join(dir, "tokens.json")
	var c Credentials
	data, err := os.ReadFile(path)
	if err == nil {
		info, err := os.Stat(path)
		if err != nil {
			return c, err
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
			return c, fmt.Errorf("token file must be private (0600)")
		}
		if json.Unmarshal(data, &c) != nil || len(c.Admin) < 32 || len(c.Ingest) < 32 || c.Admin == c.Ingest {
			return c, fmt.Errorf("invalid token file; preserve and repair it explicitly")
		}
		return c, nil
	}
	if !os.IsNotExist(err) {
		return c, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return c, err
	}
	token := func() (string, error) {
		var bytes [32]byte
		if _, err := rand.Read(bytes[:]); err != nil {
			return "", err
		}
		return hex.EncodeToString(bytes[:]), nil
	}
	c.Admin, err = token()
	if err != nil {
		return c, err
	}
	c.Ingest, err = token()
	if err != nil {
		return c, err
	}
	data, _ = json.Marshal(c)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if os.IsExist(err) {
		return PrivateCredentials(dir)
	}
	if err != nil {
		return c, err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	return c, err
}
func ValidateListen(address string, allowRemote bool) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("listen must be an explicit IP:port")
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("listen must be an explicit IP:port")
	}
	if !ip.IsLoopback() && !allowRemote {
		return fmt.Errorf("remote binding requires --allow-remote and a TLS proxy")
	}
	return nil
}
func SaveConnection(dir, address string) error {
	data, _ := json.Marshal(Connection{URL: address})
	f, err := os.CreateTemp(dir, ".connection-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(dir, "connection.json"))
}
