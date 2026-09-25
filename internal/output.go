package internal

import (
	"fmt"
	"io"
	"os"
)

// stdout is the writer all user-facing output goes through. Tests swap it via
// SetOutput to capture what a command prints.
var stdout io.Writer = os.Stdout

// SetOutput redirects user-facing output and returns the previous writer.
func SetOutput(w io.Writer) io.Writer {
	prev := stdout
	stdout = w
	return prev
}

func outf(format string, a ...any) { fmt.Fprintf(stdout, format, a...) }
func outln(a ...any)               { fmt.Fprintln(stdout, a...) }
func out(a ...any)                 { fmt.Fprint(stdout, a...) }
