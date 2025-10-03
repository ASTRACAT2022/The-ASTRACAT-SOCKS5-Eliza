package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	statsFilePath = "/var/lib/astra_socks_eliza/stats.json"
	listenAddr    = "0.0.0.0:8080"
)

// GlobalStats defines the structure for the statistics data.
type GlobalStats struct {
	TotalUploadBytes   int64                    `json:"totalUploadBytes"`
	TotalDownloadBytes int64                    `json:"totalDownloadBytes"`
	ActiveConnections  int32                    `json:"activeConnections"`
	UserStats          map[string]UserTraffic   `json:"userStats"`
	CountryStats       map[string]*CountryStats `json:"countryStats"`
	IPStats            map[string]IPStats       `json:"ipStats"`
	LastUpdateTime     time.Time                `json:"lastUpdateTime"`
}

// UserTraffic represents traffic statistics for a user.
type UserTraffic struct {
	UploadBytes   int64 `json:"uploadBytes"`
	DownloadBytes int64 `json:"downloadBytes"`
}

// CountryStats represents statistics for a country.
type CountryStats struct {
	UploadBytes   int64 `json:"uploadBytes"`
	DownloadBytes int64 `json:"downloadBytes"`
	Connections   int64 `json:"connections"`
}

// IPStats represents traffic statistics for an IP address.
type IPStats struct {
	UploadBytes   int64 `json:"uploadBytes"`
	DownloadBytes int64 `json:"downloadBytes"`
}

// cachedStats holds the latest statistics read from the file.
var cachedStats GlobalStats

func main() {
	// Periodically update the cached statistics.
	go func() {
		for {
			updateStatsCache()
			time.Sleep(5 * time.Second)
		}
	}()

	// Serve the static frontend files.
	fs := http.FileServer(http.Dir("./dashboard/static"))
	http.Handle("/", fs)

	// API endpoint to get the latest statistics.
	http.HandleFunc("/api/stats", statsHandler)

	log.Printf("Dashboard server starting on %s", listenAddr)
	if err := http.ListenAndServe(listenAddr, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// statsHandler serves the cached statistics as JSON.
func statsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(cachedStats); err != nil {
		http.Error(w, "Failed to encode statistics", http.StatusInternalServerError)
		log.Printf("Error encoding statistics: %v", err)
	}
}

// updateStatsCache reads the statistics file and updates the cache.
func updateStatsCache() {
	// Ensure the directory exists.
	if err := os.MkdirAll(filepath.Dir(statsFilePath), 0755); err != nil {
		log.Printf("Could not create stats directory: %v", err)
		return
	}

	// Open the statistics file.
	file, err := os.Open(statsFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("Statistics file does not exist yet. Waiting for it to be created.")
		} else {
			log.Printf("Error opening stats file: %v", err)
		}
		return
	}
	defer file.Close()

	// Decode the JSON data into the cachedStats variable.
	var stats GlobalStats
	if err := json.NewDecoder(file).Decode(&stats); err != nil {
		log.Printf("Error decoding stats file: %v", err)
		return
	}
	cachedStats = stats
}