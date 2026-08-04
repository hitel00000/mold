package main

import (
	"log"

	"github.com/hitel00000/mold/runtime"
)

func main() {
	app, err := runtime.New(runtime.Config{
		ResourceDir: "./resources",
		DBPath:      "./mold-quickstart-basic.db",
	})
	if err != nil {
		log.Fatalf("failed to start Mold: %v", err)
	}
	defer app.Close()

	log.Println("listening on http://localhost:8080")
	if err := app.Listen(":8080"); err != nil {
		log.Fatal(err)
	}
}
