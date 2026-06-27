package main

import (
	"fmt"
	"os"

	"github.com/Raghurajpratapsingh28/Agnivo/apps/deployer/internal/app"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/bootstrap"
)

func main() {
	if err := bootstrap.Run("deployer", app.Register); err != nil {
		fmt.Fprintf(os.Stderr, "deployer: fatal: %v\n", err)
		os.Exit(1)
	}
}
