package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"copypaste.hk/platform/internal/nativeruntime"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if run(ctx, os.Args[1:]) != nil {
		fmt.Fprintln(os.Stderr, "Native agent operation refused; inspect the private operator configuration and kernel evidence.")
		os.Exit(1)
	}
}
func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("a native operation is required")
	}
	f := flag.NewFlagSet("agent-native", flag.ContinueOnError)
	f.SetOutput(os.Stderr)
	config := f.String("config", "/etc/copypaste/agent-native.json", "root-owned broker configuration")
	socket := f.String("socket", "/run/copypaste-agent-native/control.sock", "trusted Unix socket")
	node := f.String("node", "", "pinned node executable")
	entry := f.String("entry", "", "pinned worker module")
	bridge := f.String("bridge", "", "private company relay")
	model := f.String("model", "", "private model relay")
	work := f.String("work", "", "private tmpfs directory")
	output := f.String("output", "", "new private key path")
	release := f.String("release", "", "root-owned dedicated release")
	if f.Parse(args[1:]) != nil || f.NArg() != 0 {
		return fmt.Errorf("invalid arguments")
	}
	switch args[0] {
	case "serve":
		return nativeruntime.Serve(ctx, *config)
	case "check":
		return nativeruntime.Check(*config)
	case "launch":
		return nativeruntime.Client(ctx, *socket, "launch", *node, *entry, os.Stdin, os.Stdout)
	case "probe":
		return nativeruntime.Client(ctx, *socket, "prove", "", "", os.Stdin, os.Stdout)
	case "child":
		return nativeruntime.Child(ctx, *node, *entry, *bridge, *model, *work, os.Stdin)
	case "keygen":
		return nativeruntime.GenerateKey(*output)
	case "public-key":
		value, e := nativeruntime.PublicKey(*config)
		if e != nil {
			return e
		}
		fmt.Println(value)
		return nil
	case "release-digest":
		value, e := nativeruntime.ReleaseDigest(*release)
		if e != nil {
			return e
		}
		fmt.Println(value)
		return nil
	default:
		return fmt.Errorf("unsupported native operation")
	}
}
