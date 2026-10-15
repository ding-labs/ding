package state

import "context"

func (d *DB) InstanceID(ctx context.Context) (string, error) {
	const sqliteInsert = "INSERT INTO settings(key,value) VALUES('instance_id',?) ON CONFLICT(key) DO NOTHING"
	const postgresInsert = "INSERT INTO ding_control.settings(key,value) VALUES('instance_id',$1) ON CONFLICT(key) DO NOTHING"
	if _, err := d.exec(ctx, statement{sqliteInsert, postgresInsert}, ID()); err != nil {
		return "", err
	}
	var id string
	err := d.sql.QueryRowContext(ctx, d.query(statement{"SELECT value FROM settings WHERE key='instance_id'", "SELECT value FROM ding_control.settings WHERE key='instance_id'"})).Scan(&id)
	return id, err
}
