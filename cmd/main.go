//cmd/main.go

package main

import (
	"fmt"
	"os"

	"github.com/virtualprivatenode/vpn/internal/config"
	"github.com/virtualprivatenode/vpn/internal/installer"
	"github.com/virtualprivatenode/vpn/internal/paths"
	"github.com/virtualprivatenode/vpn/internal/welcome"
)

var version = "dev"

// Explicit dispatch (IA-1-8's fix): what the binary does is
// decided by what the operator TYPED, never by sniffing box
// state. `vpn` is the node console; `sudo vpn install` is the
// installer; nothing infers one from the other.
func main() {
	installer.SetVersion(version)

	cmd, opts, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n\n%s", err, usage())
		os.Exit(2)
	}

	switch cmd {
	case cmdInstall:
		if err := installer.RunInstall(opts); err != nil {
			fmt.Fprintf(os.Stderr, "\n  Failed: %v\n", err)
			os.Exit(1)
		}
	case cmdVersion:
		fmt.Println(version)
	case cmdHelp:
		fmt.Print(usage())
	case cmdConsole:
		runConsole()
	}
}

// runConsole is the bare `vpn` path: the node console for the
// admin user. Fail-stop on an unloadable config (IA-1-C1): the
// error names the file and the reason, and Default() is NEVER
// substituted — a TUI running on defaults would render a
// mainnet node's screens over a testnet4 node's services and
// write the wrong answers back on its first save.
func runConsole() {
	if os.Geteuid() == 0 {
		fmt.Fprintf(os.Stderr,
			"  The node console runs as the %q user, not root.\n"+
				"  Connect with: ssh %s@<your-server-ip>\n"+
				"  (To install or reinstall: sudo vpn install)\n",
			paths.AdminUser, paths.AdminUser)
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr,
				"  No configuration found at %s — this node is "+
					"not installed.\n  To install: sudo vpn install\n",
				config.DefaultPath)
		} else {
			fmt.Fprintf(os.Stderr,
				"  Cannot start: configuration at %s is "+
					"unreadable:\n    %v\n"+
					"  Refusing to run with default settings in its "+
					"place — fix or restore the file.\n",
				config.DefaultPath, err)
		}
		os.Exit(1)
	}
	welcome.Show(cfg, version)
}

type command int

const (
	cmdConsole command = iota
	cmdInstall
	cmdVersion
	cmdHelp
)

// parseArgs maps the command line to a command. Pure —
// unit-tested.
func parseArgs(
	args []string,
) (command, installer.InstallOptions, error) {
	var opts installer.InstallOptions
	if len(args) == 0 {
		return cmdConsole, opts, nil
	}
	switch args[0] {
	case "install":
		for _, a := range args[1:] {
			switch a {
			case "--testnet4":
				opts.Network = "testnet4"
			case "--unattended":
				opts.Unattended = true
			case "--until=bake":
				opts.UntilBake = true
			default:
				return 0, opts, fmt.Errorf(
					"unknown install flag %q", a)
			}
		}
		return cmdInstall, opts, nil
	case "version", "--version", "-v":
		return cmdVersion, opts, nil
	case "help", "--help", "-h":
		return cmdHelp, opts, nil
	default:
		return 0, opts, fmt.Errorf("unknown command %q", args[0])
	}
}

func usage() string {
	return `Virtual Private Node

Usage:
  vpn                open the node console (run as the ` +
		paths.AdminUser + ` user)
  sudo vpn install   install or reinstall the node
      --testnet4     use testnet4 instead of mainnet
      --unattended   no prompts (keys auto-copied from the box,
                     login password generated and printed once)
  vpn version        print the version
`
}
