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

func TestPublishedOriginPreservesProducerWatch(t *testing.T) {
	got, err := rebaseProducer("https://127.0.0.1:444", "https://127.0.0.1:8443/v1/ingest/ci-2")
	if err != nil || got != "https://127.0.0.1:444/v1/ingest/ci-2" {
		t.Fatal("producer was routed to a different watch", got, err)
	}
	for _, producer := range []string{"https://external.example/v1/ingest/ci", "https://127.0.0.1/lab/start", "https://127.0.0.1/v1/ingest/ci?token=hidden", "https://127.0.0.1/v1/ingest/a%2fb"} {
		if _, err = rebaseProducer("https://127.0.0.1:444", producer); err == nil {
			t.Fatal("invalid producer route accepted")
		}
	}
}
