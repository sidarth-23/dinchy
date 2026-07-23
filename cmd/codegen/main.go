// Command codegen generates i18n, event, and permission source from their
// hand-written catalogs.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "i18n":
		err = runI18n(os.Args[2:])
	case "event":
		err = runEvent(os.Args[2:])
	case "permission":
		err = runPermission(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		err = fmt.Errorf("unknown subcommand %q", os.Args[1])
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "codegen: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: codegen <i18n|event|permission> [flags]")
}
