package main

import (
	"context"
	"flag"
	"log/slog"
)

func main() {
	autobuy := flag.Bool("autobuy", true, "auto buy")
	flag.Parse()

	cfg, err := LoadConfig("")
	if err != nil {
		slog.Error("failed to load config", "error", err)
		return
	}

	client, err := NewTelegramClient(cfg.Telegram)
	if err != nil {
		slog.Error("failed to create telegram client", "error", err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := client.Run(ctx, *autobuy); err != nil {
		slog.Error("failed to run telegram client", "error", err)
		return
	}

	slog.Info("telegram client finished")
}
