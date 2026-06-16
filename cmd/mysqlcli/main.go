package main

import (
	"context"
	"fmt"
	"os"

	"github.com/linchongguang/mysqlcli/internal/app"
	"github.com/linchongguang/mysqlcli/internal/config"
)

var version = "dev"

func main() {
	appConfig, err := config.Parse(os.Args[1:], os.Stdin, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "参数错误: %v\n", err)
		os.Exit(2)
	}

	if appConfig.ShowVersion {
		fmt.Printf("mysqlcli %s\n", version)
		return
	}
	if appConfig.ShowHelp {
		config.PrintUsage(os.Stdout)
		return
	}

	if err := app.Run(context.Background(), appConfig, os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "mysqlcli: %v\n", err)
		os.Exit(1)
	}
}
