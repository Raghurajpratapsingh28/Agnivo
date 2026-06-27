package main

import (
	"fmt"
	"os"

	"github.com/agnivo/agnivo/apps/cron/internal/app"
	"github.com/agnivo/agnivo/packages/platform/bootstrap"
)

func main() {
	if err := bootstrap.Run("cron", app.Register); err != nil {
		fmt.Fprintf(os.Stderr, "cron: fatal: %v\n", err)
		os.Exit(1)
	}
}
