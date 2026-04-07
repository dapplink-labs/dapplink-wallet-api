package flags

import "github.com/urfave/cli/v2"

const envVarPrefix = "WALLET_API"

func prefixEnvVars(name string) []string {
	return []string{envVarPrefix + "_" + name}
}

var (
	LevelDbPathFlag = &cli.StringFlag{
		Name:    "yaml-config",
		Usage:   "The path of the yaml config file",
		EnvVars: prefixEnvVars("YAML_CONFIG"),
		Value:   "./",
	}
)

var requireFlags = []cli.Flag{
	LevelDbPathFlag,
}

var optionalFlags = []cli.Flag{}

var Flags []cli.Flag

func init() {
	Flags = append(requireFlags, optionalFlags...)
}
