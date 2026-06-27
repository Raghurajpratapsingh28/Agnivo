package main

import (
	"fmt"
	"os"

	"github.com/agnivo/agnivo/apps/proxy-manager/internal/app"
	"github.com/agnivo/agnivo/packages/platform/bootstrap"
)

func main() {
	if err := bootstrap.Run("proxy-manager", app.Register); err != nil {
		fmt.Fprintf(os.Stderr, "proxy-manager: fatal: %v\n", err)
		os.Exit(1)
	}
}
