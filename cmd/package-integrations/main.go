package main

import (
	"flag"
	"fmt"
	"github.com/ding-labs/ding/internal/pluginpackage"
	"os"
)

func main() {
	var o pluginpackage.Options
	flag.StringVar(&o.Root, "root", ".", "Repository root")
	flag.StringVar(&o.Mode, "mode", "", "native-claude, remote-claude, or remote-chatgpt")
	flag.StringVar(&o.Runtime, "runtime", "", "Directory containing the Go ding-mcp executable")
	flag.StringVar(&o.Target, "target", "", "Native OS-architecture")
	flag.StringVar(&o.Endpoint, "endpoint", "", "Self-hosted HTTPS /mcp endpoint")
	flag.StringVar(&o.Output, "output", "", "New artifact output directory")
	flag.StringVar(&o.Version, "version", "0.1.0", "Plugin version matching the native binary")
	flag.Parse()
	archive, err := pluginpackage.Build(o)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Built", archive)
	fmt.Println("Unsigned qualification artifact; marketplace approval remains a release gate.")
}
