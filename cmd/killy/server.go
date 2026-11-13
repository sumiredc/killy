package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sumiredc/killy/internal/clock"
	"github.com/sumiredc/killy/internal/server"
	"github.com/sumiredc/killy/internal/server/live"
	"github.com/sumiredc/killy/internal/server/pairing"
	"github.com/sumiredc/killy/internal/server/ratelimit"
	"github.com/sumiredc/killy/internal/server/store"
)

func runServer(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("killy server", flag.ContinueOnError)
	fs.SetOutput(stderr)
	listen := fs.String("listen", ":8080", "address to listen on")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	log := slog.New(slog.NewTextHandler(stderr, nil))
	dbURL := os.Getenv("KILLY_DATABASE_URL")
	if dbURL == "" {
		log.Error("KILLY_DATABASE_URL is not set")
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Error("open database", "err", err)
		return 1
	}
	defer pool.Close()

	c := clock.System{}
	st := store.New(pool)
	srv := &server.Server{
		Store:               st,
		Codes:               pairing.New(c),
		FirstRuns:           ratelimit.New(server.FirstRunLimit, server.FirstRunPeriod, c),
		DeviceRegistrations: ratelimit.New(server.DeviceRegistrationLimit, server.DeviceRegistrationPeriod, c),
		FailedClaims:        ratelimit.New(server.FailedClaimLimit, server.FailedClaimPeriod, c),
		Live:                live.New(c, st),
		Log:                 log,
	}
	err = serve(ctx, log,
		func(ctx context.Context) error { return store.Migrate(ctx, pool) },
		func() (net.Listener, error) { return net.Listen("tcp", *listen) },
		srv.Handler())
	if err != nil {
		log.Error("killy server stopped", "err", err)
		return 1
	}
	return 0
}

// serve migrates the database, then listens and serves until ctx is done. It does not listen
// when the migration fails, so no request ever meets an old or half-migrated schema.
func serve(ctx context.Context, log *slog.Logger, migrate func(context.Context) error, listen func() (net.Listener, error), h http.Handler) error {
	if err := migrate(ctx); err != nil {
		return err
	}
	ln, err := listen()
	if err != nil {
		return err
	}
	httpServer := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- httpServer.Serve(ln) }()
	log.Info("listening", "addr", ln.Addr().String())

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
