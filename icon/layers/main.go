// Command layers writes the SVG files used by AppIcon.icon and
// AppIcon-dev.icon. It runs anywhere, so the layered macOS artwork can be
// updated before opening the documents in Icon Composer.
package main

import (
	"log"

	"lilt/icon/art"
)

func main() {
	if err := art.WriteDocuments(); err != nil {
		log.Fatal(err)
	}
}
