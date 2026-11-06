package consolecontract

import (
	"bytes"
	"os"
	"testing"
)

func TestGeneratedContractsCurrent(t *testing.T) {
	b, err := os.ReadFile("../../web/console/src/api/contracts.ts")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, Generate()) {
		t.Fatal("console contracts changed: run go run ./cmd/console-contract")
	}
}

func TestEmbeddedReadinessMatchesJSONContract(t *testing.T) {
	generated := Generate()
	if !bytes.Contains(generated, []byte("interface WatchrunDestinationSetupStatus extends StoreDestinationReadiness {")) || bytes.Contains(generated, []byte("DestinationReadiness: StoreDestinationReadiness")) {
		t.Fatal("embedded readiness must be flattened like encoding/json, not nested")
	}
}
