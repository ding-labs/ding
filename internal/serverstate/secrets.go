package state

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/ding-labs/ding/internal/source"
)

var secretName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

// Vault requires a separately supplied key. Never store it in the SQLite volume,
// container image, an ordinary backup, or a process-global tenant environment.
type Vault struct {
	db     *DB
	master []byte
}

func NewVault(db *DB, key []byte) (*Vault, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("cloud secret encryption requires a 32-byte external key")
	}
	return &Vault{db, append([]byte(nil), key...)}, nil
}

func (v *Vault) aead(account string) (cipher.AEAD, error) {
	key, err := hkdf.Key(sha256.New, v.master, nil, "ding.cloud.secrets.v1/"+account, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCMWithRandomNonce(block)
}

func (v *Vault) Put(ctx context.Context, account, name, value string) error {
	if !secretName.MatchString(name) || value == "" || len(value) > 16<<10 {
		return fmt.Errorf("credential name or size is invalid")
	}
	aead, err := v.aead(account)
	if err != nil {
		return err
	}
	encrypted := aead.Seal(nil, nil, []byte(value), []byte(account+"\x00"+name))
	tx, err := v.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var count, exists int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*),COUNT(CASE WHEN name=? THEN 1 END) FROM secrets WHERE account=?", name, account).Scan(&count, &exists); err != nil {
		return err
	}
	if count >= 20 && exists == 0 {
		return fmt.Errorf("workspace allows at most twenty credentials")
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO secrets(account,name,ciphertext) VALUES(?,?,?) ON CONFLICT(account,name) DO UPDATE SET ciphertext=excluded.ciphertext", account, name, encrypted); err != nil {
		return err
	}
	return tx.Commit()
}

func (v *Vault) Get(ctx context.Context, account, name string) (string, error) {
	value, _, err := v.GetWithRevision(ctx, account, name)
	return value, err
}

func (v *Vault) GetWithRevision(ctx context.Context, account, name string) (string, string, error) {
	var encrypted []byte
	if err := v.db.sql.QueryRowContext(ctx, "SELECT ciphertext FROM secrets WHERE account=? AND name=?", account, name).Scan(&encrypted); err != nil {
		return "", "", err
	}
	aead, err := v.aead(account)
	if err != nil {
		return "", "", err
	}
	plain, err := aead.Open(nil, nil, encrypted, []byte(account+"\x00"+name))
	if err != nil {
		return "", "", fmt.Errorf("credential decryption failed")
	}
	return string(plain), TokenHash(string(encrypted)), nil
}

func (v *Vault) Names(ctx context.Context, account string) ([]string, error) {
	rows, err := v.db.sql.QueryContext(ctx, "SELECT name FROM secrets WHERE account=? ORDER BY name", account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func (v *Vault) Delete(ctx context.Context, account, name string) error {
	_, err := v.db.sql.ExecContext(ctx, "DELETE FROM secrets WHERE account=? AND name=?", account, name)
	return err
}

func (v *Vault) Lookup(account string) source.Lookup {
	// The account binding is captured once and cannot be changed by a request.
	return func(name string) (string, bool) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		value, err := v.Get(ctx, account, name)
		return value, err == nil && value != ""
	}
}

func MissingSecret(err error) bool { return errors.Is(err, sql.ErrNoRows) }
