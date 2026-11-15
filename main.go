// Command webnak is the Webnak command-line tool.
package main

import (
	"os"

	"github.com/sathninduk/webnak-cli/cmd"
)

var version = "dev"

func main() {
	cmd.Version = version
	os.Exit(cmd.Execute())
}
