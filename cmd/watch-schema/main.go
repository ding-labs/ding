// Command watch-schema regenerates the structural authoring schema.
package main

import (
	"github.com/ding-labs/ding/internal/plan"
	"os"
)

func main() {
	data, err := plan.JSONSchema()
	if err != nil {
		panic(err)
	}
	os.Stdout.Write(append(data, '\n'))
}
