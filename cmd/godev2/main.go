package main

import "os"

func main() {
	os.Exit(run(os.Args[1:], startWithConfig, stopWithConfig, downWithConfig, os.Stdout, os.Stderr))
}
