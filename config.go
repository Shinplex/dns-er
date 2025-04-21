package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/fsnotify/fsnotify"
)

// Config holds the DNS server configuration
type Config struct {
	Server    ServerConfig            `toml:"server"`
	Upstreams map[string]UpstreamConfig `toml:"upstreams"`
	
	// Added mutex for thread safety
	mu sync.RWMutex
}

// ServerConfig contains DNS server settings
type ServerConfig struct {
	Listen     string `toml:"listen"`
	Port       int    `toml:"port"`
	LogQueries bool   `toml:"log_queries"`
	// Path to the records file
	RecordsFile string `toml:"records_file"`
}

// UpstreamConfig contains configuration for an upstream DNS server
type UpstreamConfig struct {
	Address  string `toml:"address"`
	Port     int    `toml:"port"`
	Protocol string `toml:"protocol"` // "udp" or "tcp"
}

// RecordsConfig contains all DNS record entries
type RecordsConfig struct {
	Records []RecordEntry `toml:"records"`
	
	// Added mutex for thread safety
	mu sync.RWMutex
}

// RecordEntry represents a single DNS record entry
type RecordEntry struct {
	Domain string `toml:"domain"`
	Type   string `toml:"type"`
	Value  string `toml:"value"`
	TTL    int    `toml:"ttl"`
}

// Global records configuration
var Records = &RecordsConfig{
	Records: []RecordEntry{},
}

// LoadConfig loads configuration from a TOML file
func LoadConfig(filePath string) (*Config, error) {
	config := &Config{}
	
	if _, err := toml.DecodeFile(filePath, config); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	
	// Set defaults if not specified
	if config.Server.Port == 0 {
		config.Server.Port = 53
	}
	
	if config.Server.Listen == "" {
		config.Server.Listen = "0.0.0.0"
	}
	
	// Set default records file if not specified
	if config.Server.RecordsFile == "" {
		config.Server.RecordsFile = "configs/records.toml"
	}
	
	// Validate config
	if len(config.Upstreams) == 0 {
		return nil, fmt.Errorf("no upstream DNS servers configured")
	}
	
	// Try to load records
	if err := LoadRecords(config.Server.RecordsFile); err != nil {
		log.Printf("Warning: Failed to load records file: %v", err)
		// Not returning error to allow server to start without records
	}
	
	// Start watching for config file changes
	go WatchConfigFile(filePath)
	
	// Start watching for records file changes
	go WatchRecordsFile(config.Server.RecordsFile)
	
	return config, nil
}

// LoadRecords loads DNS records from a TOML file
func LoadRecords(filePath string) error {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// Create an empty records file if it doesn't exist
		if err := SaveRecords(filePath, &RecordsConfig{}); err != nil {
			return fmt.Errorf("failed to create records file: %w", err)
		}
	}

	newRecords := &RecordsConfig{}
	if _, err := toml.DecodeFile(filePath, newRecords); err != nil {
		return fmt.Errorf("failed to load records: %w", err)
	}
	
	// Update records with lock to ensure thread safety
	Records.mu.Lock()
	Records.Records = newRecords.Records
	Records.mu.Unlock()
	
	log.Printf("Loaded %d records from %s", len(newRecords.Records), filePath)
	return nil
}

// SaveConfig saves the current configuration to a TOML file
func SaveConfig(config *Config, filePath string) error {
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer f.Close()
	
	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(config); err != nil {
		return fmt.Errorf("failed to encode config: %w", err)
	}
	
	return nil
}

// SaveRecords saves the current records to a TOML file
func SaveRecords(filePath string, records *RecordsConfig) error {
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create records file: %w", err)
	}
	defer f.Close()
	
	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(records); err != nil {
		return fmt.Errorf("failed to encode records: %w", err)
	}
	
	return nil
}

// WatchConfigFile watches for changes to the config file and reloads it
func WatchConfigFile(filePath string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("Error setting up config file watcher: %v", err)
		return
	}
	defer watcher.Close()

	// Add the directory containing the config file to the watcher
	dir := filepath.Dir(filePath)
	if err := watcher.Add(dir); err != nil {
		log.Printf("Error watching config directory: %v", err)
		return
	}

	filename := filepath.Base(filePath)
	log.Printf("Watching for changes to config file: %s", filePath)

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			// Only process the config file we're interested in
			if filepath.Base(event.Name) != filename {
				continue
			}

			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				// Wait a short time to ensure the file is fully written
				time.Sleep(100 * time.Millisecond)
				
				log.Printf("Config file changed: %s", filePath)
				
				_, err := LoadConfig(filePath)
				if err != nil {
					log.Printf("Error reloading config: %v", err)
					continue
				}
				
				log.Printf("Config reloaded successfully")
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Config watcher error: %v", err)
		}
	}
}

// WatchRecordsFile watches for changes to the records file and reloads it
func WatchRecordsFile(filePath string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("Error setting up records file watcher: %v", err)
		return
	}
	defer watcher.Close()

	// Add the directory containing the records file to the watcher
	dir := filepath.Dir(filePath)
	if err := watcher.Add(dir); err != nil {
		log.Printf("Error watching records directory: %v", err)
		return
	}

	filename := filepath.Base(filePath)
	log.Printf("Watching for changes to records file: %s", filePath)

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			// Only process the records file we're interested in
			if filepath.Base(event.Name) != filename {
				continue
			}

			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				// Wait a short time to ensure the file is fully written
				time.Sleep(100 * time.Millisecond)
				
				log.Printf("Records file changed: %s", filePath)
				
				if err := LoadRecords(filePath); err != nil {
					log.Printf("Error reloading records: %v", err)
					continue
				}
				
				log.Printf("Records reloaded successfully")
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Records watcher error: %v", err)
		}
	}
}

// MatchDomain checks if a domain matches a pattern, supporting wildcards
// The _** pattern represents unlimited levels of subdomains
func MatchDomain(pattern, domain string) bool {
	// Remove trailing dots
	pattern = strings.TrimSuffix(pattern, ".")
	domain = strings.TrimSuffix(domain, ".")
	
	// Case insensitive comparison
	pattern = strings.ToLower(pattern)
	domain = strings.ToLower(domain)
	
	// Exact match check
	if pattern == domain {
		return true
	}
	
	// Handle unlimited subdomain wildcard (_**)
	if strings.Contains(pattern, "_**") {
		parts := strings.SplitN(pattern, "_**", 2)
		base := parts[1]
		
		// Check if domain ends with the base part
		return strings.HasSuffix(domain, base)
	}
	
	// Handle individual wildcards (*)
	if strings.Contains(pattern, "*") {
		// Convert the pattern to a regex-like pattern
		regexPattern := strings.ReplaceAll(pattern, ".", "\\.")
		regexPattern = strings.ReplaceAll(regexPattern, "*", "[^.]*")
		
		// Match the pattern
		matched, _ := filepath.Match(regexPattern, domain)
		return matched
	}
	
	// No match
	return false
}

// FindMatchingRecord looks for a matching record for the given domain and type
func FindMatchingRecord(domain string, recordType string) *RecordEntry {
	Records.mu.RLock()
	defer Records.mu.RUnlock()
	
	// Remove trailing dot from domain if present
	domain = strings.TrimSuffix(domain, ".")
	
	for _, record := range Records.Records {
		if MatchDomain(record.Domain, domain) && record.Type == recordType {
			return &record
		}
	}
	
	return nil
}