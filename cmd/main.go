//cmd/main.go

package main

import (
	"fmt"
	"os"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/installer"
	"github.com/ripsline/virtual-private-node/internal/welcome"
)

var version = "dev"

func main() {
	installer.SetVersion(version)

	if !installer.NeedsInstall() {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			cfg = config.Default()
		}
		welcome.Show(cfg, version)
		return
	}

	// Initial install — runs via sudo from the TUI
	if err := installer.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "\n  Failed: %v\n", err)
		os.Exit(1)
	}
	cfg, err := config.Load()
	if err != nil {
		cfg = config.Default()
	}
	welcome.Show(cfg, version)
}
