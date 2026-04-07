package main

import (
	"context"
	"os"

	"github.com/ethereum/go-ethereum/log"

	"github.com/dapplink-labs/dapplink-wallet-api/common/opio"
)

func main() {
	log.SetDefault(log.NewLogger(log.NewTerminalHandlerWithLevel(os.Stderr, log.LevelInfo, true)))
	app := NewCli()
	ctx := opio.WithInterruptBlocker(context.Background())
	if err := app.RunContext(ctx, os.Args); err != nil {
		log.Error("Application failed")
		os.Exit(1)
	}
}
