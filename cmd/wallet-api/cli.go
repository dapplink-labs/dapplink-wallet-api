package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/ethereum/go-ethereum/log"
	"github.com/urfave/cli/v2"

	"github.com/dapplink-labs/dapplink-wallet-api/common/cliapp"
	"github.com/dapplink-labs/dapplink-wallet-api/config"
	flags2 "github.com/dapplink-labs/dapplink-wallet-api/flags"
	"github.com/dapplink-labs/dapplink-wallet-api/services/grpc"
)

func runRpc(ctx *cli.Context, shutdown context.CancelCauseFunc) (cliapp.Lifecycle, error) {
	fmt.Println("running grpc services...")
	var f = flag.String("c", "config.yml", "config path")
	flag.Parse()
	cfg, err := config.NewConfig(*f)
	if err != nil {
		log.Error("new config fail", "err", err)
		return nil, err
	}
	return grpc.NewRpcService(cfg)
}

func runApi(ctx *cli.Context, _ context.CancelCauseFunc) (cliapp.Lifecycle, error) {
	return nil, nil
}

func NewCli() *cli.App {
	flags := flags2.Flags
	return &cli.App{
		Version:              "v0.0.1-beta",
		Description:          "wallet chain api gateway",
		EnableBashCompletion: true,
		Commands: []*cli.Command{
			{
				Name:        "rpc",
				Flags:       flags,
				Description: "Run rpc services",
				Action:      cliapp.LifecycleCmd(runRpc),
			},
			{
				Name:        "api",
				Flags:       flags,
				Description: "Run api services",
				Action:      cliapp.LifecycleCmd(runApi),
			},
			{
				Name:        "version",
				Description: "Show project version",
				Action: func(ctx *cli.Context) error {
					cli.ShowVersion(ctx)
					return nil
				},
			},
		},
	}
}
