package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/marcgauthier/Crypto/internal/scanner"
)

func main() {
	if err := scanner.Run(context.Background(), os.Stdout, os.Args[1:], time.Now().UTC()); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
