package main

import (
	"fmt"
	"os"

	"github.com/Raghurajpratapsingh28/Agnivo/apps/api/internal/app"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/bootstrap"
)

func main() {
	if err := bootstrap.Run("api", app.Register); err != nil {
		fmt.Fprintf(os.Stderr, "api: fatal: %v\n", err)
		os.Exit(1)
	}
}
