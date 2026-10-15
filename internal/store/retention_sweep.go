package store

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type retentionFamily struct {
	name                      string
	numeric, secondKey        bool
	selectQuery, deletePrefix statement
}
type retentionCursor struct {
	Pass       string
	At         int64
	Key, Extra string
	Done       bool
}

// scanRetentionFamily limits rows examined before checking pins. A pinned page
// advances the cursor too. Completed families stay complete for this pass; an
// interrupted pass resumes at its saved key in the next process. A later pass
// revisits earlier rows whose references have since been released.
func (t *Tx) scanRetentionFamily(f retentionFamily, pass string, cutoff int64, limit int) (int64, error) {
	key := "retention_cursor:" + f.name
	var raw string
	cursor := retentionCursor{At: math.MinInt64}
	err := t.sql.QueryRowContext(t.ctx, t.query(qReadRetentionCursor), key).Scan(&raw)
	if err != nil && missing(err) != ErrNotFound {
		return 0, err
	}
	if raw != "" {
		if err = json.Unmarshal([]byte(raw), &cursor); err != nil {
			return 0, fmt.Errorf("invalid retention cursor")
		}
	}
	if cursor.Pass == pass && cursor.Done {
		return 0, nil
	}
	if cursor.Pass != pass && cursor.Done {
		cursor = retentionCursor{At: math.MinInt64}
	}
	cursor.Pass = pass
	var firstKey any = cursor.Key
	if f.numeric {
		firstKey = int64(0)
		if cursor.Key != "" {
			n, e := strconv.ParseInt(cursor.Key, 10, 64)
			if e != nil {
				return 0, fmt.Errorf("invalid numeric retention cursor")
			}
			firstKey = n
		}
	}
	args := []any{cutoff, cursor.At, firstKey}
	if f.secondKey {
		args = append(args, cursor.Extra)
	}
	args = append(args, limit)
	rows, err := t.sql.QueryContext(t.ctx, t.query(f.selectQuery), args...)
	if err != nil {
		return 0, err
	}
	candidates := []retentionCursor{}
	for rows.Next() {
		var c retentionCursor
		if err = rows.Scan(&c.At, &c.Key, &c.Extra); err != nil {
			rows.Close()
			return 0, err
		}
		candidates = append(candidates, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	if len(candidates) > 0 {
		placeholders := make([]string, 0, len(candidates))
		values := []any{}
		for _, c := range candidates {
			var value any = c.Key
			if f.numeric {
				n, e := strconv.ParseInt(c.Key, 10, 64)
				if e != nil {
					return 0, e
				}
				value = n
			}
			bind := t.bind(len(values) + 1)
			values = append(values, value)
			if f.secondKey {
				bind = "(" + bind + "," + t.bind(len(values)+1) + ")"
				values = append(values, c.Extra)
			}
			placeholders = append(placeholders, bind)
		}
		if _, err = t.execSQL(t.query(f.deletePrefix)+strings.Join(placeholders, ",")+")", values...); err != nil {
			return 0, err
		}
		cursor = candidates[len(candidates)-1]
		cursor.Pass = pass
	} else {
		cursor.Done = true
	}
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return 0, err
	}
	_, err = t.exec(qSaveRetentionCursor, key, string(encoded))
	return int64(len(candidates)), err
}

var qReadRetentionCursor = statement{sqlite: `SELECT value FROM metadata WHERE key=?`, postgres: `SELECT value FROM metadata WHERE workspace_id=current_setting('ding.workspace') AND key=$1`}
var qSaveRetentionCursor = statement{sqlite: `INSERT INTO metadata(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, postgres: `INSERT INTO metadata(key,value) VALUES($1,$2) ON CONFLICT(workspace_id,key) DO UPDATE SET value=excluded.value`}
