package main

import "testing"

func TestLocalURLRejectsRemoteTargets(t *testing.T) {
	for _, u := range []string{"https://cloud.ding.ing", "http://127.0.0.1", "https://user@127.0.0.1", "https://127.0.0.1?q=1", "https://127.0.0.1/#x"} {
		if localURL(u) {
			t.Fatal(u)
		}
	}
	for _, u := range []string{"https://127.0.0.1:8443", "https://[::1]:8443"} {
		if !localURL(u) {
			t.Fatal(u)
		}
	}
}
