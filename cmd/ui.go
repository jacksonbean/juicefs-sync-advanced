package cmd

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/jacksonbean/juicefs-sync-advanced/pkg/api"
	"github.com/urfave/cli/v2"
)

func cmdUI(uiFS fs.FS) *cli.Command {
	return &cli.Command{
		Name:     "ui",
		Usage:    "Start the web dashboard UI",
		Category: "",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  "port",
				Value: 9567,
				Usage: "HTTP server port",
			},
			&cli.StringFlag{
				Name:  "history-db",
				Value: filepath.Join(os.Getenv("HOME"), ".juicefs_sync_history.db"),
				Usage: "History database path",
			},
			&cli.StringFlag{
				Name:  "schedule-db",
				Value: filepath.Join(os.Getenv("HOME"), ".juicefs_sync_schedule.db"),
				Usage: "Schedule database path",
			},
			&cli.BoolFlag{
				Name:  "no-scheduler",
				Usage: "Disable background scheduler",
			},
		},
		Action: func(c *cli.Context) error {
			port := c.Int("port")
			historyDB := c.String("history-db")
			scheduleDB := c.String("schedule-db")
			noScheduler := c.Bool("no-scheduler")

			srv := api.NewServer(port, historyDB, scheduleDB, uiFS)
			if !noScheduler {
				srv.StartScheduler()
			}
			return srv.ListenAndServe()
		},
	}
}
