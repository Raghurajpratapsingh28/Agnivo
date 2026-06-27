package main

import (
	"fmt"
	"os"

	"github.com/Raghurajpratapsingh28/Agnivo/apps/builder/internal/app"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/bootstrap"
)

func main() {
	if err := bootstrap.Run("builder", app.Register); err != nil {
		fmt.Fprintf(os.Stderr, "builder: fatal: %v\n", err)
		os.Exit(1)
	}
}
