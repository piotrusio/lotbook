// Command lotbook is the one binary of Lotbook, started in one of its roles
// (ADR-0001).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"lotbook.org/lotbook/internal/platform/server"
)

const usage = `usage: lotbook <role> [flags]

roles:
  ingest   accepts source records and writes them to the journal
  worker   follows the journal and interprets it
  query    holds the graph and serves it

lotbook <role> -h lists the role's flags.
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// run starts the role named by args[0] and serves until ctx is done. It
// returns the process's exit code: 0 after a clean stop, 1 when the role
// fails, 2 when it is started wrongly.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return refuse(stderr, "")
	}
	role := args[0]
	switch role {
	case "ingest", "worker", "query":
	default:
		return refuse(stderr, fmt.Sprintf("lotbook: unknown role %q\n\n", role))
	}

	flags := flag.NewFlagSet("lotbook "+role, flag.ContinueOnError)
	flags.SetOutput(stderr)
	addr := flags.String("addr", ":8080", "address to serve on")
	drain := flags.Duration("drain", 5*time.Second, "how long to report not ready before stopping")
	grace := flags.Duration("grace", 20*time.Second, "how long requests in flight may take to finish")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	log := slog.New(slog.NewJSONHandler(stdout, nil)).With("role", role)
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", *addr)
	if err != nil {
		log.Error("cannot listen", "addr", *addr, "err", err)
		return 1
	}
	log.Info("serving", "addr", ln.Addr().String())
	if err := server.Run(ctx, ln, server.Config{Drain: *drain, Grace: *grace}); err != nil {
		log.Error("stopped with an error", "err", err)
		return 1
	}
	log.Info("stopped")
	return 0
}

// refuse prints why lotbook was started wrongly and how to start it, and
// returns the exit code that says so.
func refuse(stderr io.Writer, reason string) int {
	if _, err := io.WriteString(stderr, reason+usage); err != nil {
		return 1
	}
	return 2
}
