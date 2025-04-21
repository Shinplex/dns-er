package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// Define command line flags
	configPath := flag.String("config", "configs/config.toml", "Path to the configuration file")
	flag.Parse()

	// Load configuration
	config, err := LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create and start DNS server
	server := NewDNSServer(config)

	// Handle OS signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in a goroutine
	go func() {
		if err := server.Start(); err != nil {
			log.Fatalf("Failed to start DNS server: %v", err)
		}
	}()

	log.Println("DNS server started. Press Ctrl+C to stop.")

	// Wait for termination signal
	<-sigChan
	log.Println("Shutting down DNS server...")

	// Gracefully stop the server
	if err := server.Stop(); err != nil {
		log.Printf("Error stopping DNS server: %v", err)
	}

	log.Println("DNS server stopped.")
}
