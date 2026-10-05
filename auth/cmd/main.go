package main

import (
	"fmt"
	"os"

	"github.com/elug3/dupli1/shared/pkg/sentrymon"
)

func main() {
	defer sentrymon.Init("dupli1-auth")()

	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
