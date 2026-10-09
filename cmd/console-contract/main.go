package main

import (
	"github.com/ding-labs/ding/internal/consolecontract"
	"log"
	"os"
)

func main() {
	if err := os.MkdirAll("web/console/src/api", 0755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("web/console/src/api/contracts.ts", consolecontract.Generate(), 0644); err != nil {
		log.Fatal(err)
	}
}
