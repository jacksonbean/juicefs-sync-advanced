package main

import (
	"os"

	"github.com/jacksonbean/juicefs-sync-advanced/cmd"
	"github.com/jacksonbean/juicefs-sync-advanced/pkg/utils"
)

var logger = utils.GetLogger("juicefs")

func main() {
	err := cmd.Main(os.Args, webDistFS())
	if err != nil {
		logger.Fatal(err)
	}
}
