/*
 * JuiceFS, Copyright 2020 Juicedata, Inc.
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
	"net/http"
	_ "net/http/pprof"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/erikdubbelboer/gspt"
	"github.com/google/uuid"
	"github.com/grafana/pyroscope-go"
	_ "github.com/grafana/pyroscope-go/godeltaprof/http/pprof"
	"github.com/jacksonbean/juicefs-sync-advanced/pkg/utils"
	"github.com/jacksonbean/juicefs-sync-advanced/pkg/version"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"go.uber.org/automaxprocs/maxprocs"
)

var logger = utils.GetLogger("juicefs")
var debugAgent string
var debugAgentOnce sync.Once

func Main(args []string) error {
	gspt.SetProcTitle(strings.Join(os.Args, " "))
	cli.VersionFlag = &cli.BoolFlag{
		Name: "version", Aliases: []string{"V"},
		Usage: "print version only",
	}
	app := &cli.App{
		Name:                 "juicefs-sync-advanced",
		Usage:                "A sync tool for object storage",
		Version:              version.Version(),
		Copyright:            "Apache License 2.0",
		HideHelpCommand:      true,
		EnableBashCompletion: true,
		Flags:                globalFlags(),
		Commands: []*cli.Command{
			cmdSync(),
			cmdScan(),
		},
	}
	err := app.Run(reorderOptions(app, args))
	if errno, ok := err.(syscall.Errno); ok && errno == 0 {
		err = nil
	}
	return err
}

func isFlag(flags []cli.Flag, option string) (bool, bool) {
	if !strings.HasPrefix(option, "-") {
		return false, false
	}
	option = strings.TrimLeft(option, "-")
	for _, flag := range flags {
		_, isBool := flag.(*cli.BoolFlag)
		for _, name := range flag.Names() {
			if option == name || strings.HasPrefix(option, name+"=") {
				return true, !isBool && !strings.Contains(option, "=")
			}
		}
	}
	return false, false
}

func reorderOptions(app *cli.App, args []string) []string {
	var newArgs = []string{args[0]}
	var others []string
	globalFlags := append(app.Flags, cli.VersionFlag)
	for i := 1; i < len(args); i++ {
		option := args[i]
		if ok, hasValue := isFlag(globalFlags, option); ok {
			newArgs = append(newArgs, option)
			if hasValue {
				i++
				if i >= len(args) {
					logger.Fatalf("option %s requires value", option)
				}
				newArgs = append(newArgs, args[i])
			}
		} else {
			others = append(others, option)
		}
	}
	if len(others) == 0 {
		return newArgs
	}
	cmdName := others[0]
	var cmd *cli.Command
	for _, c := range app.Commands {
		if c.Name == cmdName {
			cmd = c
			break
		}
	}
	if cmd == nil {
		return append(newArgs, others...)
	}
	newArgs = append(newArgs, cmdName)
	args, others = others[1:], nil
	cmdFlags := append(cmd.Flags, cli.HelpFlag)
	for i := 0; i < len(args); i++ {
		option := args[i]
		if ok, hasValue := isFlag(cmdFlags, option); ok {
			newArgs = append(newArgs, option)
			if hasValue && len(args[i+1:]) > 0 {
				i++
				newArgs = append(newArgs, args[i])
			}
		} else {
			if strings.HasPrefix(option, "-") && !utils.StringContains(args, "--generate-bash-completion") {
				logger.Fatalf("unknown option: %s", option)
			}
			others = append(others, option)
		}
	}
	return append(newArgs, others...)
}

func setup(c *cli.Context, n int) {
	setup0(c, n, n)
}

func setup0(c *cli.Context, min, max int) {
	if c.NArg() < min {
		fmt.Printf("ERROR: This command requires at least %d arguments\n", min)
		fmt.Printf("USAGE:\n   juicefs-sync-advanced %s [command options] %s\n", c.Command.Name, c.Command.ArgsUsage)
		os.Exit(1)
	} else if max > 0 && c.NArg() > max {
		fmt.Printf("ERROR: This command accept at most %d arguments but got %+v\n", max, c.Args().Slice())
		fmt.Printf("USAGE:\n   juicefs-sync-advanced %s [command options] %s\n", c.Command.Name, c.Command.ArgsUsage)
		logger.Exit(1)
	}

	switch c.String("log-level") {
	case "trace":
		utils.SetLogLevel(logrus.TraceLevel)
	case "debug":
		utils.SetLogLevel(logrus.DebugLevel)
	case "info":
		utils.SetLogLevel(logrus.InfoLevel)
	case "warn":
		utils.SetLogLevel(logrus.WarnLevel)
	case "error":
		utils.SetLogLevel(logrus.ErrorLevel)
	case "fatal":
		utils.SetLogLevel(logrus.FatalLevel)
	case "panic":
		utils.SetLogLevel(logrus.PanicLevel)
	default:
		if c.Bool("trace") {
			utils.SetLogLevel(logrus.TraceLevel)
		} else if c.Bool("verbose") {
			utils.SetLogLevel(logrus.DebugLevel)
		} else if c.Bool("quiet") {
			utils.SetLogLevel(logrus.WarnLevel)
		} else {
			utils.SetLogLevel(logrus.InfoLevel)
		}
	}
	if c.Bool("no-color") {
		utils.DisableLogColor()
	}
	if undo, err := maxprocs.Set(maxprocs.Logger(logger.Debugf)); err != nil {
		undo()
	}

	logID := c.String("log-id")
	if logID != "" {
		if logID == "random" {
			logID = uuid.New().String()
		}
		utils.SetLogID("[" + logID + "] ")
	}

	if !c.Bool("no-agent") {
		go debugAgentOnce.Do(func() {
			for port := 6060; port < 6100; port++ {
				debugAgent = fmt.Sprintf("127.0.0.1:%d", port)
				logger.Debugf("Debug agent listening on %s", debugAgent)
				_ = http.ListenAndServe(debugAgent, nil)
			}
		})
	}

	if c.IsSet("pyroscope") {
		tags := make(map[string]string)
		appName := fmt.Sprintf("juicefs.%s", c.Command.Name)
		if hostname, err := os.Hostname(); err == nil {
			tags["hostname"] = hostname
		}
		tags["pid"] = strconv.Itoa(os.Getpid())
		tags["version"] = version.Version()

		types := []pyroscope.ProfileType{pyroscope.ProfileCPU, pyroscope.ProfileInuseObjects, pyroscope.ProfileAllocObjects,
			pyroscope.ProfileInuseSpace, pyroscope.ProfileAllocSpace, pyroscope.ProfileGoroutines, pyroscope.ProfileMutexCount,
			pyroscope.ProfileMutexDuration, pyroscope.ProfileBlockCount, pyroscope.ProfileBlockDuration}
		if _, err := pyroscope.Start(pyroscope.Config{
			ApplicationName: appName,
			ServerAddress:   c.String("pyroscope"),
			Logger:          logger,
			Tags:            tags,
			AuthToken:       os.Getenv("PYROSCOPE_AUTH_TOKEN"),
			ProfileTypes:    types,
		}); err != nil {
			logger.Errorf("start pyroscope agent: %v", err)
		}
	}
}

func removePassword(uris ...string) {
	args := make([]string, len(os.Args))
	copy(args, os.Args)
	var idx int
	for _, uri := range uris {
		uri2 := utils.RemovePassword(uri)
		if uri2 != uri {
			for i := idx; i < len(os.Args); i++ {
				if os.Args[i] == uri {
					args[i] = uri2
					idx = i + 1
					break
				}
			}
		}
	}
	gspt.SetProcTitle(strings.Join(args, " "))
}
