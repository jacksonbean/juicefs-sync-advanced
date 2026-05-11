/*
 * JuiceFS, Copyright 2018 Juicedata, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package cmd

import (
	"fmt"
	"time"

	"github.com/jacksonbean/juicefs-sync-advanced/pkg/object"
	"github.com/jacksonbean/juicefs-sync-advanced/pkg/scan"
	"github.com/spf13/cast"
	"github.com/urfave/cli/v2"
)

func cmdScan() *cli.Command {
	return &cli.Command{
		Name:      "scan",
		Action:    doScan,
		Category:  "TOOL",
		Usage:     "Scan object metadata and save to database or export CSV",
		ArgsUsage: "SRC",
		Description: `
Scan all objects from the given storage and record their metadata (key, size, mtime, storage class)
into a database table or export to CSV. Supports time-range filtering and automatic scan tracking.

Examples:
  # Scan all objects and save to SQLite
  $ juicefs-sync-advanced scan --db-type sqlite3 --db-dsn /tmp/inventory.db s3://mybucket/

  # Scan with time range filter and export CSV
  $ juicefs-sync-advanced scan \
      --start "2025-01-01" --end "2025-06-30" \
      --export /tmp/jan_jun_2025.csv \
      s3://mybucket/

  # Re-export existing scan_id to CSV without re-scanning
  $ juicefs-sync-advanced scan --export /tmp/export.csv \
      --db-type sqlite3 --db-dsn /tmp/inventory.db \
      --scan-id scan-1700000000123456789
      `,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "no-https",
				Usage: "don't use HTTPS",
			},
			&cli.StringFlag{
				Name:  "db-type",
				Usage: "database type for saving inventory (mysql, postgres, sqlite3)",
			},
			&cli.StringFlag{
				Name:  "db-dsn",
				Usage: "database DSN for saving inventory",
			},
			&cli.StringFlag{
				Name:  "prefix",
				Usage: "only scan objects with the given prefix",
			},
			&cli.StringFlag{
				Name:  "start",
				Usage: "only include objects modified after start-time (2006-01-02 15:04:05)",
			},
			&cli.StringFlag{
				Name:  "end",
				Usage: "only include objects modified before end-time (2006-01-02 15:04:05)",
			},
			&cli.StringFlag{
				Name:  "export",
				Usage: "export results to CSV file path",
			},
			&cli.Int64Flag{
				Name:  "limit",
				Value: -1,
				Usage: "limit the number of objects to scan (-1 is unlimited)",
			},
			&cli.StringFlag{
				Name:  "scan-id",
				Usage: "scan run ID (auto-generated if not set; when used with existing DB + export, re-exports without scanning)",
			},
			&cli.StringFlag{
				Name:  "diff-from",
				Usage: "first scan_id for diff comparison (requires --diff-to and --export)",
			},
			&cli.StringFlag{
				Name:  "diff-to",
				Usage: "second scan_id for diff comparison (requires --diff-from and --export)",
			},
		},
	}
}

func doScan(c *cli.Context) error {
	setup(c, 0)
	args := c.Args()

	// Diff mode: compare two scan_ids
	diffFrom := c.String("diff-from")
	diffTo := c.String("diff-to")
	if diffFrom != "" || diffTo != "" {
		if diffFrom == "" || diffTo == "" {
			return fmt.Errorf("both --diff-from and --diff-to are required")
		}
		dbType := c.String("db-type")
		dbDsn := c.String("db-dsn")
		export := c.String("export")
		if dbType == "" || dbDsn == "" {
			return fmt.Errorf("--db-type and --db-dsn are required for diff")
		}
		if export == "" {
			return fmt.Errorf("--export is required for diff")
		}
		n, err := scan.DiffScans(dbType, dbDsn, diffFrom, diffTo, export)
		if err != nil {
			return err
		}
		logger.Infof("Diff complete: %d changes exported to %s", n, export)
		return nil
	}

	// Re-export mode: no source, export from existing DB
	if args.Len() == 0 && c.String("scan-id") != "" {
		dbType := c.String("db-type")
		dbDsn := c.String("db-dsn")
		export := c.String("export")
		if dbType == "" || dbDsn == "" {
			return fmt.Errorf("--db-type and --db-dsn are required for re-export")
		}
		if export == "" {
			return fmt.Errorf("--export is required for re-export")
		}
		startStr := ""
		endStr := ""
		if c.IsSet("start") {
			t, err := cast.ToTimeInDefaultLocationE(c.String("start"), time.Local)
			if err != nil {
				return fmt.Errorf("invalid start time: %s", err)
			}
			startStr = t.UTC().Format(time.RFC3339)
		}
		if c.IsSet("end") {
			t, err := cast.ToTimeInDefaultLocationE(c.String("end"), time.Local)
			if err != nil {
				return fmt.Errorf("invalid end time: %s", err)
			}
			endStr = t.UTC().Format(time.RFC3339)
		}
		n, err := scan.ExportCSV(dbType, dbDsn, c.String("scan-id"), startStr, endStr, export)
		if err != nil {
			return err
		}
		logger.Infof("Exported %d records to %s", n, export)
		return nil
	}

	if args.Len() < 1 {
		return fmt.Errorf("requires SRC argument")
	}

	srcURL := args.Get(0)
	removePassword(srcURL)

	cfg := &scan.Config{
		ScanID:  c.String("scan-id"),
		DBType:  c.String("db-type"),
		DBDSN:   c.String("db-dsn"),
		Prefix:  c.String("prefix"),
		Export:  c.String("export"),
		Limit:   c.Int64("limit"),
		Threads: 10,
	}

	// must have at least one output destination
	if cfg.DBType == "" && cfg.Export == "" {
		return fmt.Errorf("at least one of --db-type or --export is required")
	}

	if c.IsSet("start") {
		t, err := cast.ToTimeInDefaultLocationE(c.String("start"), time.Local)
		if err != nil {
			return fmt.Errorf("invalid start time: %s", err)
		}
		cfg.StartTime = t
	}
	if c.IsSet("end") {
		t, err := cast.ToTimeInDefaultLocationE(c.String("end"), time.Local)
		if err != nil {
			return fmt.Errorf("invalid end time: %s", err)
		}
		cfg.EndTime = t
	}

	// reuse the same createSyncStorage used by sync command
	storeConfig := &scanStoreConfig{noHTTPS: c.Bool("no-https")}
	src, err := createSyncStorage(srcURL, storeConfig)
	if err != nil {
		return err
	}
	defer object.Shutdown(src)

	return scan.Run(src, cfg)
}

// scanStoreConfig implements storageConfig for the scan command.
type scanStoreConfig struct {
	noHTTPS bool
}

func (s *scanStoreConfig) GetNoHTTPS() bool { return s.noHTTPS }
