package main

import (
	"os"

	"github.com/zeulewan/applship/internal/applship"
)

func main() {
	os.Exit(applship.Main(os.Args[1:]))
}
