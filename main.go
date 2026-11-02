package main

import (
	"context"
	"log"
	"os"

	"github.com/egoist/mygo"

	"lilt/internal/app"
	"lilt/internal/control"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "ctl":
			os.Exit(control.CLI(os.Args[2:]))
		case "mcp":
			if err := control.ServeMCP(context.Background(), mygo.App.Version()); err != nil {
				log.Fatal(err)
			}
			return
		}
	}
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
