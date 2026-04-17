package main

import (
	"flag"
	"log"

	"easyvpn/internal/config"
	"easyvpn/internal/server"
)

func main() {
	configPath := flag.String("config", "server.json", "path to server config")
	flag.Parse()

	cfg, err := config.LoadServerConfig(*configPath)
	if err != nil {
		log.Fatalf("load server config: %v", err)
	}

	srv, err := server.New(cfg)
	if err != nil {
		log.Fatalf("create server: %v", err)
	}
	if err := srv.Run(); err != nil {
		log.Fatalf("server exited: %v", err)
	}
}
