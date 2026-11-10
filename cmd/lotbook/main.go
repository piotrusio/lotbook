// Command lotbook is the one binary of Lotbook, started in one of its roles
// (ADR-0001) or run as one of its life-cycle commands (ADR-0016).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"lotbook.org/lotbook/internal/ingest"
	"lotbook.org/lotbook/internal/journal"
	"lotbook.org/lotbook/internal/platform/server"
	"lotbook.org/lotbook/internal/platform/telemetry"
	"lotbook.org/lotbook/internal/store"
)

const usage = `usage: lotbook <role|command> [flags]

roles:
  ingest   accepts source records and writes them to the journal
  worker   follows the journal and takes it into PostgreSQL
  query    holds the graph and serves it

commands:
  migrate  brings the PostgreSQL schema up to date, then exits

lotbook <role|command> -h lists its flags.
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// options are the flags every role and command takes; each uses its own.
type options struct {
	addr         string
	drain, grace time.Duration
	kafkaBrokers string
	journalTopic string
	workerGroup  string
	postgresURL  string
}

// needs names, for each role and command, the options it cannot start
// without.
var needs = map[string][]string{
	"ingest":  {"-kafka-brokers"},
	"worker":  {"-postgres-url", "-kafka-brokers"},
	"query":   nil,
	"migrate": {"-postgres-url"},
}

// run starts the role or command named by args[0]. A role serves until ctx is
// done. It returns the process's exit code: 0 after a clean stop, 1 when it
// fails, 2 when it is started wrongly.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return refuse(stderr, "")
	}
	name := args[0]
	required, known := needs[name]
	if !known {
		return refuse(stderr, fmt.Sprintf("lotbook: unknown role or command %q\n\n", name))
	}

	var o options
	flags := flag.NewFlagSet("lotbook "+name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&o.addr, "addr", ":8080", "address to serve on")
	flags.DurationVar(&o.drain, "drain", 5*time.Second, "how long to report not ready before stopping")
	flags.DurationVar(&o.grace, "grace", 20*time.Second, "how long requests in flight may take to finish")
	flags.StringVar(&o.kafkaBrokers, "kafka-brokers", "", "Kafka brokers to start from, comma-separated (ingest, worker)")
	flags.StringVar(&o.journalTopic, "journal-topic", "lotbook.journal", "the journal's Kafka topic (ingest, worker)")
	flags.StringVar(&o.workerGroup, "worker-group", "lotbook.worker", "the workers' Kafka consumer group (worker)")
	flags.StringVar(&o.postgresURL, "postgres-url", "", "PostgreSQL connection URL (worker, migrate)")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	for _, need := range required {
		if flags.Lookup(strings.TrimPrefix(need, "-")).Value.String() == "" {
			return refuse(stderr, fmt.Sprintf("lotbook %s: %s is required\n\n", name, need))
		}
	}

	log := telemetry.Logger(stdout).With("role", name)
	if name == "migrate" {
		return migrate(ctx, log, o)
	}
	return serve(ctx, log, name, o)
}

// migrate runs the migrate command.
func migrate(ctx context.Context, log *slog.Logger, o options) int {
	pool, err := pgxpool.New(ctx, o.postgresURL)
	if err != nil {
		log.Error("cannot use the PostgreSQL URL", "err", err)
		return 1
	}
	defer pool.Close()
	if err := store.Migrate(ctx, pool); err != nil {
		log.Error("cannot migrate", "err", err)
		return 1
	}
	log.Info("migrated")
	return 0
}

// serve runs a role until ctx is done, or until the work behind it fails.
func serve(ctx context.Context, log *slog.Logger, role string, o options) int {
	tp, err := telemetry.Tracing(ctx, role)
	if err != nil {
		log.Error("cannot set up tracing", "err", err)
		return 1
	}
	defer func() {
		flushing, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := tp.Shutdown(flushing); err != nil {
			log.Error("cannot flush traces", "err", err)
		}
	}()

	ctx, stop := context.WithCancel(ctx)
	defer stop()
	routes := http.NewServeMux()
	worked := make(chan error, 1)
	switch role {
	case "ingest":
		j, err := journal.NewKafka(strings.Split(o.kafkaBrokers, ","), o.journalTopic)
		if err != nil {
			log.Error("cannot reach the journal", "err", err)
			return 1
		}
		defer j.Close()
		ingest.Routes(routes, j)
		worked <- nil
	case "worker":
		pool, err := pgxpool.New(ctx, o.postgresURL)
		if err != nil {
			log.Error("cannot use the PostgreSQL URL", "err", err)
			return 1
		}
		defer pool.Close()
		if err := store.Check(ctx, pool); err != nil {
			log.Error("cannot start on this database", "err", err)
			return 1
		}
		go func() {
			err := journal.Follow(ctx, strings.Split(o.kafkaBrokers, ","), o.journalTopic, o.workerGroup, store.New(pool))
			worked <- err
			stop()
		}()
	default:
		worked <- nil
	}

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", o.addr)
	if err != nil {
		log.Error("cannot listen", "addr", o.addr, "err", err)
		return 1
	}
	log.Info("serving", "addr", ln.Addr().String())
	cfg := server.Config{Handler: telemetry.HTTP(tp, routes), Drain: o.drain, Grace: o.grace}
	served := server.Run(ctx, ln, cfg)
	stop()
	if err := errors.Join(served, <-worked); err != nil {
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
