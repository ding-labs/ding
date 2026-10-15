package pgtest

import "testing"

func TestFixtureRejectsRemoteTransportAndOverrides(t *testing.T) {
	for _, value := range []string{"postgres://u:secret@db.example/ding?sslmode=verify-full", "postgres://u:secret@127.0.0.1/ding?host=db.example&sslmode=verify-full", "host=localhost,db.example user=u password=secret dbname=ding sslmode=verify-full"} {
		if validateLocalFixture(value) == nil {
			t.Fatal("fixture accepted a remote target")
		}
	}
	for _, value := range []string{"postgres://u:secret@127.0.0.1/ding?sslmode=disable", "postgres://u:secret@[::1]/ding?sslmode=disable"} {
		if err := validateLocalFixture(value); err != nil {
			t.Fatal(err)
		}
	}
}
