package logger

import (
	"io"
	"os"
)

// stdout is indirected so tests can capture output if needed.
func stdout() io.Writer { return os.Stdout }
