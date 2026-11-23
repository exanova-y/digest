// Command scan reads recent X posts (top search results and/or the user's
// following) and has Claude distill them into one of two outputs:
//
//	scan stocks   notable stocks  -> stocks.json
//	scan reads    papers, books…  -> reads.xml (RSS 2.0)
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
)

func main() {
	if err := loadDotEnv(".env"); err != nil {
		log.Fatalf(".env: %v", err)
	}
	cmd, args := "stocks", os.Args[1:] // bare `scan` keeps its original meaning
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	ctx := context.Background()
	var err error
	switch cmd {
	case "stocks":
		err = runStocks(ctx, args)
	case "reads":
		err = runReads(ctx, args)
	default:
		err = fmt.Errorf("unknown command %q (want stocks or reads)", cmd)
	}
	if err != nil {
		log.Fatal(err)
	}
}
