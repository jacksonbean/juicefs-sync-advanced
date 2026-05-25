/*
 * JuiceFS, Copyright 2022 Juicedata, Inc.
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
	"net/url"
	"os"
	"strings"

	"github.com/urfave/cli/v2"
)

func globalFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:    "verbose",
			Aliases: []string{"debug", "v"},
			Usage:   "enable debug log",
		},
		&cli.BoolFlag{
			Name:    "quiet",
			Aliases: []string{"q"},
			Usage:   "show warning and errors only",
		},
		&cli.BoolFlag{
			Name:  "trace",
			Usage: "enable trace log",
		},
		&cli.StringFlag{
			Name:   "log-level",
			Usage:  "set log level (trace, debug, info, warn, error, fatal, panic)",
			Hidden: true,
		},
		&cli.StringFlag{
			Name:  "log-id",
			Usage: "append the given log id in log, use \"random\" to use random uuid",
		},
		&cli.BoolFlag{
			Name:  "no-agent",
			Usage: "disable pprof (:6060) agent",
		},
		&cli.StringFlag{
			Name:  "pyroscope",
			Usage: "pyroscope address",
		},
		&cli.BoolFlag{
			Name:  "no-color",
			Usage: "disable colors",
		},
	}
}

func addCategory(f cli.Flag, cat string) {
	switch ff := f.(type) {
	case *cli.StringFlag:
		ff.Category = cat
	case *cli.BoolFlag:
		ff.Category = cat
	case *cli.IntFlag:
		ff.Category = cat
	case *cli.Int64Flag:
		ff.Category = cat
	case *cli.Uint64Flag:
		ff.Category = cat
	case *cli.Float64Flag:
		ff.Category = cat
	case *cli.StringSliceFlag:
		ff.Category = cat
	default:
		panic(f)
	}
}

func addCategories(cat string, flags []cli.Flag) []cli.Flag {
	for _, f := range flags {
		addCategory(f, cat)
	}
	return flags
}

func expandFlags(compoundFlags ...[]cli.Flag) []cli.Flag {
	var flags []cli.Flag
	for _, flag := range compoundFlags {
		flags = append(flags, flag...)
	}
	return flags
}

// setup validates the command arguments and does basic setup.
// minArgs: minimum number of positional arguments required.
func setup(c *cli.Context, minArgs int) {
	if c.Args().Len() < minArgs {
		logger.Fatalf("wrong number of arguments: expected at least %d, got %d", minArgs, c.Args().Len())
	}
}

// removePassword removes password from URLs in arguments.
func removePassword(srcURL, dstURL string) {
	for _, uri := range []*string{&srcURL, &dstURL} {
		if u, err := url.Parse(*uri); err == nil {
			if _, ok := u.User.Password(); ok {
				u.User = url.UserPassword(u.User.Username(), "")
				*uri = u.String()
			}
		}
	}
	// Also remove from os.Args for process title
	for i, arg := range os.Args {
		if strings.Contains(arg, "://") {
			if u, err := url.Parse(arg); err == nil {
				if _, ok := u.User.Password(); ok {
					u.User = url.UserPassword(u.User.Username(), "")
					os.Args[i] = u.String()
				}
			}
		}
	}
}
