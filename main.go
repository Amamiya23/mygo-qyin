package main

import (
	"log"

	"qqmusic-mygo/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
