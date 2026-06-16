package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/linchongguang/mysqlcli/internal/config"
	"github.com/linchongguang/mysqlcli/internal/database"
	"github.com/linchongguang/mysqlcli/internal/output"
	"github.com/linchongguang/mysqlcli/internal/query"
	"github.com/linchongguang/mysqlcli/internal/repl"
)

func Run(ctx context.Context, appConfig config.Config, input io.Reader, outputWriter io.Writer, errorWriter io.Writer) error {
	client, err := database.Open(ctx, appConfig)
	if err != nil {
		return err
	}
	defer client.Close()

	renderer := output.NewRenderer(outputWriter, output.Options{
		Batch:           appConfig.Batch,
		SkipColumnNames: appConfig.SkipColumnNames,
		Silent:          appConfig.Silent,
	})

	if appConfig.Execute != "" {
		result, err := query.Execute(ctx, client.DB(), strings.TrimSpace(appConfig.Execute))
		if err != nil {
			return err
		}
		return renderer.Render(result)
	}

	interactive := isTerminal(input)
	if interactive && !appConfig.Silent {
		info, err := client.SessionInfo(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintf(outputWriter, "Welcome to mysqlcli. Connection id: %d, MySQL: %s\nType \\? for help.\n\n", info.ConnectionID, info.Version)
	}
	return repl.New(client, renderer, input, outputWriter, errorWriter, interactive, appConfig.HistoryFile, appConfig.HistoryEnabled, appConfig.CustomCommands).Run(ctx)
}

func isTerminal(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
