// ding-local-load drives only the isolated localhost stress test binary.
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ding-labs/ding/internal/cloud"
)

func localURL(raw string) bool {
	return fixtureURL(raw, "")
}

func fixtureURL(raw, allowed string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	ip := net.ParseIP(u.Hostname())
	return (ip != nil && ip.IsLoopback()) || (strings.HasSuffix(allowed, ".railway.internal") && u.Hostname() == allowed)
}

func run() error {
	bootstrap := flag.String("bootstrap", "", "private fixture bootstrap file")
	endpoint := flag.String("url", "", "published loopback URL")
	output := flag.String("output", "", "private evidence directory")
	rate := flag.Int("rate", 100, "offered requests/second")
	seconds := flag.Int("seconds", 180, "measured seconds")
	workers := flag.Int("workers", 512, "generator workers")
	seed := flag.Uint64("shuffle-seed", 0, "nonzero seed interleaves the weighted producer cycle")
	flag.Parse()
	allowed := os.Getenv("DING_SCALE_PRIVATE_HOST")
	if allowed != "" && (os.Getenv("DING_TEST_RAILWAY_PROJECT") == "" || os.Getenv("DING_TEST_RAILWAY_PROJECT") != os.Getenv("RAILWAY_PROJECT_ID")) {
		return fmt.Errorf("private load target requires matching Railway project authorization")
	}
	if *output == "" {
		return fmt.Errorf("output directory required")
	}
	raw, err := os.ReadFile(*bootstrap)
	if err != nil {
		return err
	}
	var b struct {
		URL, Admin, CA string
		Producers      []cloud.PushProducer
		Tokens         map[string]string
	}
	if err := json.Unmarshal(raw, &b); err != nil {
		return err
	}
	if *endpoint != "" {
		b.URL = *endpoint
	}
	if !fixtureURL(b.URL, allowed) {
		return fmt.Errorf("only an HTTPS loopback fixture is allowed")
	}
	for i := range b.Producers {
		rebased, err := rebaseFixtureProducer(b.URL, b.Producers[i].URL, allowed)
		if err != nil {
			return err
		}
		b.Producers[i].URL = rebased
		if err := os.Setenv(b.Producers[i].TokenEnv, b.Tokens[b.Producers[i].TokenEnv]); err != nil {
			return err
		}
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(b.CA)) {
		return fmt.Errorf("fixture CA invalid")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 512
	transport.MaxIdleConnsPerHost = 512
	transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}
	defer transport.CloseIdleConnections()
	// Baseline snapshots for a large cohort can be slow on a deliberately
	// constrained database. This deadline applies only to lab coordination;
	// offered ingestion requests keep the client deadline above.
	controlClient := *client
	controlClient.Timeout = 3 * time.Minute
	control := func(path string, value any) error {
		body, err := json.Marshal(value)
		if err != nil {
			return err
		}
		req, err := http.NewRequest("POST", b.URL+path, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+b.Admin)
		req.Header.Set("Content-Type", "application/json")
		r, err := controlClient.Do(req)
		if err != nil {
			return err
		}
		r.Body.Close()
		if r.StatusCode != 204 {
			return fmt.Errorf("fixture control returned %d", r.StatusCode)
		}
		return nil
	}
	options := cloud.PushBenchmarkOptions{Producers: b.Producers, Client: client, Rate: min(*rate, 100), Duration: 5 * time.Second, Workers: *workers, Ledger: filepath.Join(*output, "warmup.jsonl"), LedgerBatch: 32, CompactLedger: true, PreplannedLedger: true, NoRetry: true}
	options.ShuffleSeed = *seed
	if _, err := cloud.BenchmarkPush(context.Background(), options); err != nil {
		return err
	}
	options.BeforeStart = func() error { return control("/lab/start", nil) }
	options.Rate = *rate
	options.Duration = time.Duration(*seconds) * time.Second
	options.Ledger = filepath.Join(*output, "requests.jsonl")
	result, loadErr := cloud.BenchmarkPush(context.Background(), options)
	if allowed != "" {
		ledger, err := os.Open(options.Ledger)
		if err != nil {
			return err
		}
		defer ledger.Close()
		req, err := http.NewRequest("PUT", b.URL+"/lab/ledger", ledger)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+b.Admin)
		response, err := controlClient.Do(req)
		if err != nil {
			return err
		}
		response.Body.Close()
		if response.StatusCode != 204 {
			return fmt.Errorf("journal transfer returned %d", response.StatusCode)
		}
	}
	if err := control("/lab/finish", result); err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return err
	}
	return loadErr
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func rebaseProducer(origin, producer string) (string, error) {
	return rebaseFixtureProducer(origin, producer, "")
}

func rebaseFixtureProducer(origin, producer, allowed string) (string, error) {
	if !fixtureURL(origin, allowed) || !fixtureURL(producer, allowed) {
		return "", fmt.Errorf("producer and published origin must be loopback fixtures")
	}
	base, _ := url.Parse(origin)
	original, _ := url.Parse(producer)
	id := strings.TrimPrefix(original.Path, "/v1/ingest/")
	if (base.Path != "" && base.Path != "/") || id == original.Path || id == "" || strings.ContainsAny(id, "/%\\") || original.RawPath != "" {
		return "", fmt.Errorf("invalid producer fixture path")
	}
	base.Path = original.Path
	return base.String(), nil
}
