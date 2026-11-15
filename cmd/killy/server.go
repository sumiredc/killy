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
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sumiredc/killy/internal/clock"
	"github.com/sumiredc/killy/internal/server"
	"github.com/sumiredc/killy/internal/server/command"
	"github.com/sumiredc/killy/internal/server/live"
	"github.com/sumiredc/killy/internal/server/pairing"
	"github.com/sumiredc/killy/internal/server/push"
	"github.com/sumiredc/killy/internal/server/ratelimit"
	"github.com/sumiredc/killy/internal/server/sse"
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
	lv := live.New(c, st)
	pushes := push.New(st, lv, &push.Expo{AccessToken: os.Getenv("EXPO_ACCESS_TOKEN")}, log)
	defer pushes.Close()
	srv := &server.Server{
		Store:               st,
		Codes:               pairing.New(c),
		FirstRuns:           ratelimit.New(server.FirstRunLimit, server.FirstRunPeriod, c),
		DeviceRegistrations: ratelimit.New(server.DeviceRegistrationLimit, server.DeviceRegistrationPeriod, c),
		FailedClaims:        ratelimit.New(server.FailedClaimLimit, server.FailedClaimPeriod, c),
		Kills:               ratelimit.New(server.KillLimit, server.KillPeriod, c),
		Commands:            command.New(c, time.After),
		Live:                lv,
		Broker:              sse.NewBroker(),
		Pushes:              pushes,
		DemoAgentID:         demoAgentID(os.Getenv("KILLY_DEMO_AGENT_ID"), log),
		Log:                 log,
	}
	go func() {
		t := time.NewTicker(server.ConnectionCheckInterval)
		defer t.Stop()
		srv.WatchConnections(ctx, t.C)
	}()
	err = serve(ctx, log,
		func(ctx context.Context) error {
			v, err := store.Migrate(ctx, pool)
			if err == nil {
				log.Info("database migrated", "version", v)
			}
			return err
		},
		func() (net.Listener, error) { return net.Listen("tcp", *listen) },
		srv.Handler(), srv.StopStreams)
	if err != nil {
		log.Error("killy server stopped", "err", err)
		return 1
	}
	return 0
}

// serve migrates the database, then listens and serves until ctx is done. It does not listen
// when the migration fails, so no request ever meets an old or half-migrated schema.
// onShutdown runs when shutdown starts, to end the long-lived streams Shutdown would wait for.
func serve(ctx context.Context, log *slog.Logger, migrate func(context.Context) error, listen func() (net.Listener, error), h http.Handler, onShutdown func()) error {
	if err := migrate(ctx); err != nil {
		return err
	}
	ln, err := listen()
	if err != nil {
		return err
	}
	httpServer := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	httpServer.RegisterOnShutdown(onShutdown)
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

// demoAgentID reads KILLY_DEMO_AGENT_ID. A value that is not a UUID is logged and treated as unset.
func demoAgentID(raw string, log *slog.Logger) string {
	if raw == "" {
		return ""
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		log.Error("KILLY_DEMO_AGENT_ID is not a UUID; no demo server is offered", "value", raw)
		return ""
	}
	return id.String()
}
