package main

import (
	"fmt"
	"os"

	"github.com/Raghurajpratapsingh28/Agnivo/apps/proxy-manager/internal/app"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/bootstrap"
)

func main() {
	if err := bootstrap.Run("proxy-manager", app.Register); err != nil {
		fmt.Fprintf(os.Stderr, "proxy-manager: fatal: %v\n", err)
		os.Exit(1)
	}
}
