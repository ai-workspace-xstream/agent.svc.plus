package xrayembed

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/xtls/xray-core/main/commands/base"

	// Auxiliary xray subcommands (uuid, x25519, tls, api, convert, wg, ...).
	// These live in importable packages; only `run`/`version` are stuck in
	// xray-core's non-importable package main, so we provide our own below.
	_ "github.com/xtls/xray-core/main/commands/all"
)

// RunCLI implements the k3s-style `agent xray ...` passthrough: it makes the
// monolith behave exactly like the standalone `xray` binary. os.Args must
// already be rewritten so that os.Args[0] is the program name and os.Args[1]
// is the xray subcommand (e.g. "run", "uuid", "version").
//
// We can't import xray-core's own `run` command (it sits in its package
// main), so we register an equivalent built on the same core.Instance path
// as the supervised runtime, then hand control to xray's command dispatcher
// which already knows about every auxiliary command via the blank import.
func RunCLI() {
	base.RootCommand.Long = "Embedded Xray (agent-svc-plus monolith)."
	base.RootCommand.Commands = append(
		[]*base.Command{cmdRun, cmdVersion},
		base.RootCommand.Commands...,
	)
	base.Execute()
}

var cmdVersion = &base.Command{
	UsageLine: "{{.Exec}} version",
	Short:     "print xray-core version",
	Run: func(_ *base.Command, _ []string) {
		os.Stdout.WriteString("xray-core " + Version() + "\n")
	},
}

var cmdRun = &base.Command{
	UsageLine: "{{.Exec}} run [-c config.json]",
	Short:     "run xray with a config file",
}

// Set in init to avoid an initialization cycle between cmdRun, executeRun,
// and the flag var that reads from cmdRun.Flag (mirrors xray-core's own
// pattern).
func init() { cmdRun.Run = executeRun }

var runConfigPath = cmdRun.Flag.String("c", "config.json", "config file path")

func executeRun(_ *base.Command, _ []string) {
	data, err := os.ReadFile(*runConfigPath)
	if err != nil {
		base.Fatalf("read config: %v", err)
	}

	r := New("cli")
	if err := r.Apply(data); err != nil {
		base.Fatalf("%v", err)
	}
	defer r.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
}
