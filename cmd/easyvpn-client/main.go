package main

import (
	"flag"
	"log"

	"easyvpn/internal/client"
	"easyvpn/internal/config"
)

func main() {
	configPath := flag.String("config", "client.json", "path to client config")
	flag.Parse()

	cfg, err := config.LoadClientConfig(*configPath)
	if err != nil {
		log.Fatalf("load client config: %v", err)
	}

	c := client.New(cfg)
	if err := c.Run(); err != nil {
		log.Fatalf("client exited: %v", err)
	}
}
