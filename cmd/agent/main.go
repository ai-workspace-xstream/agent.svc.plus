package main

import (
	"fmt"
	"os"
	"path/filepath"

	caddycmd "github.com/caddyserver/caddy/v2/cmd"

	"agent.svc.plus/internal/caddyembed"
	"agent.svc.plus/internal/xrayembed"

	// Register Caddy's standard modules + our plugin set for the `caddy`
	// personality. Xray's modules are registered inside the xrayembed
	// package via its own blank import.
	_ "github.com/caddy-dns/alidns"
	_ "github.com/caddy-dns/cloudflare"
	_ "github.com/caddyserver/caddy/v2/modules/standard"
	_ "github.com/mholt/caddy-l4"
)

// This binary is a k3s-style multicall executable. A single compiled file
// carries three "personalities" — the agent supervisor, an embedded Xray,
// and an embedded Caddy — and picks one at startup from either the invoked
// name (argv[0], via symlink) or the first subcommand. That lets the release
// ship one artifact plus two symlinks instead of three separate binaries,
// while keeping `xray ...` / `caddy ...` invocations working unchanged for
// existing scripts and systemd units.
var (
	serviceName  = "agent-svc-plus"
	gitCommit    = "unknown"
	buildDate    = "unknown"
	caddyVersion = "unknown"
	xrayVersion  = "unknown"
)

func main() {
	// 1. argv[0] dispatch (symlink personalities): /usr/local/bin/xray and
	//    /usr/local/bin/caddy can both be symlinks to this binary.
	switch filepath.Base(os.Args[0]) {
	case "xray":
		xrayembed.RunCLI()
		return
	case "caddy":
		caddycmd.Main()
		return
	}

	// 2. Subcommand dispatch: `agent xray ...`, `agent caddy ...`.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "xray":
			// Shift argv so the delegated CLI sees itself as os.Args[0].
			os.Args = append([]string{"xray"}, os.Args[2:]...)
			xrayembed.RunCLI()
			return
		case "caddy":
			os.Args = append([]string{"caddy"}, os.Args[2:]...)
			caddycmd.Main()
			return
		case "version", "-v", "--version":
			printVersion()
			return
		case "server", "run", "agent":
			os.Args = append(os.Args[:1], os.Args[2:]...)
			runAgent()
			return
		}
	}

	// 3. Default personality: the agent supervisor.
	runAgent()
}

func printVersion() {
	fmt.Printf("%s %s %s\n", serviceName, gitCommit, buildDate)
	fmt.Printf("caddy=%s (linked %s)\n", caddyVersion, caddyembed.Version())
	fmt.Printf("xray=%s (linked %s)\n", xrayVersion, xrayembed.Version())
}
