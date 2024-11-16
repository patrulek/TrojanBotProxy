package main

import (
	"context"
	"flag"
	"log/slog"
	"time"

	"github.com/patrulek/trojanbotproxy/config"
)

func main() {
	autobuy := flag.Bool("autobuy", true, "auto buy")
	flag.Parse()

	cfg, err := config.Load("")
	if err != nil {
		slog.Error("failed to load config", "error", err)
		return
	}

	ds, err := NewHttpDataSource(cfg.DataSource)
	var dsv DataSource = ds
	if err != nil {
		slog.Warn("failed to create http data source", "error", err)
		dsv = nil
	}

	client, err := NewTelegramClient(cfg.Telegram, dsv)
	if err != nil {
		slog.Error("failed to create telegram client", "error", err)
		return
	}

	var srvs []srv
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer stopCancel()

		for _, s := range srvs {
			s.Stop(stopCtx)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if ds != nil {
		if err := ds.Start(ctx); err != nil {
			slog.Error("failed to run http data source", "error", err)
			return
		}

		srvs = append(srvs, ds)
	}

	if err := client.Start(ctx, *autobuy); err != nil {
		slog.Error("failed to run telegram client", "error", err)
		return
	}
}

type srv interface {
	Stop(ctx context.Context)
}
