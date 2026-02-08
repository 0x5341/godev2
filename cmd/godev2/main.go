package main

import "os"

func main() {
	os.Exit(run(os.Args[1:], startWithConfig, os.Stdout, os.Stderr))
}
