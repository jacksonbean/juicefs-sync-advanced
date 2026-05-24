package cmd

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/jacksonbean/juicefs-sync-advanced/pkg/utils"
	"github.com/jacksonbean/juicefs-sync-advanced/pkg/version"
	"github.com/urfave/cli/v2"
)

var logger = utils.GetLogger("juicefs")

func Main(args []string, uiFS fs.FS) error {
	app := cli.NewApp()
	app.Name = filepath.Base(args[0])
	app.Usage = "A sync tool for object storage"
	app.Version = version.Version
	app.Flags = []cli.Flag{
		&cli.BoolFlag{Name: "verbose,debug", Aliases: []string{"v"}, Usage: "enable debug log"},
		&cli.BoolFlag{Name: "quiet", Aliases: []string{"q"}, Usage: "show warning and errors only"},
		&cli.BoolFlag{Name: "trace", Usage: "enable trace log"},
		&cli.StringFlag{Name: "log-id", Usage: "append the given log id in log, use \"random\" to use random uuid"},
	}
	app.Commands = []*cli.Command{
		cmdSync(),
		cmdScan(),
		cmdUI(uiFS),
	}
	app.Before = func(c *cli.Context) error {
		if c.Bool("trace") {
			utils.SetLogLevel("trace")
		} else if c.Bool("verbose") {
			utils.SetLogLevel("debug")
		} else if c.Bool("quiet") {
			utils.SetLogLevel("warn")
		} else {
			utils.SetLogLevel("info")
		}

		parts := make([]string, len(os.Args))
		copy(parts, os.Args)
		for i, p := range parts {
			if strings.Contains(p, "://") {
				if idx := strings.LastIndex(p, "@"); idx >= 0 {
					if authIdx := strings.Index(p, "//"); authIdx >= 0 && authIdx+2 < idx {
						parts[i] = p[:authIdx+2] + "****" + p[idx:]
					}
				}
			}
		}
		utils.SetProcessTitle(strings.Join(parts, " "))
		return nil
	}
	return app.Run(args)
}
