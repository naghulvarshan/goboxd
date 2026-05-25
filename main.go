package main

import (
	"flag"

	server "github.com/thesouldev/goboxd/internal/server"
)

func main() {
	var port string
	flag.StringVar(&port, "port", "8080", "--port 8080")
	flag.Parse()
	server.Serve(port)
}
