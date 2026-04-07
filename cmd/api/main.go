package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	_ "github.com/lib/pq" // The Postgres driver
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
)

// LocationUpdate represents the JSON we expect from a vehicle
type LocationUpdate struct {
	VehicleID string  `json:"vehicle_id"`
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
}

var (
	db  *sql.DB
	rdb *redis.Client
	ctx = context.Background() // Required by the Redis client
)

var kafkaWriter *kafka.Writer

func main() {
	// Connect to the PostgreSQL container
	connStr := "postgres://fleet_admin:fleet_password@db:5432/fleet_db?sslmode=disable"
	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	// Connect to Redis
	rdb = redis.NewClient(&redis.Options{
		Addr:     "redis:6379", // Matches the service name in docker-compose
		Password: "",           // No password set
		DB:       0,            // Default DB
	})

	// Test the Redis connection
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatal("Failed to connect to Redis:", err)
	}

	// Set up Kafka Writer (Producer) to send messages to the worker
	kafkaWriter = &kafka.Writer{
		Addr:     kafka.TCP("kafka:9092"),
		Topic:    "incoming-locations",
		Balancer: &kafka.LeastBytes{},
	}
	defer kafkaWriter.Close()

	// Set up routes
	mux := http.NewServeMux()
	mux.HandleFunc("POST /location", saveLocation)
	mux.HandleFunc("GET /check-zone", checkZone)
	mux.HandleFunc("GET /latest-location", getLatestLocation)

	// Start the server
	fmt.Println("🚀 Go Engine running on port 8080...")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

// Save the incoming GPS coordinates
func saveLocation(w http.ResponseWriter, r *http.Request) {
	var loc LocationUpdate
	if err := json.NewDecoder(r.Body).Decode(&loc); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Turn the struct back into a byte slice to send over Kafka
	payload, _ := json.Marshal(loc)

	// Step A: Publish message to Kafka and return IMMEDIATELY
	err := kafkaWriter.WriteMessages(context.Background(),
		kafka.Message{
			Key:   []byte(loc.VehicleID), // Using VehicleID as key ensures ordered processing per vehicle
			Value: payload,
		},
	)

	if err != nil {
		http.Error(w, "Failed to publish to Kafka", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted) // 202 Accepted (we accepted it, but processing happens later)
	fmt.Fprintf(w, "Location queued for %s!\n", loc.VehicleID)
}

// Check if a vehicle is inside a specific geofence
func checkZone(w http.ResponseWriter, r *http.Request) {
	vehicleID := r.URL.Query().Get("vehicle_id")
	zoneName := r.URL.Query().Get("zone")

	// PostGIS spatial math: ST_Intersects checks if the Point is inside the Polygon
	query := `
		SELECT ST_Intersects(
			(SELECT location FROM vehicle_history WHERE vehicle_id = $1 ORDER BY timestamp DESC LIMIT 1),
			(SELECT geom FROM geofences WHERE name = $2)
		)
	`

	var isInside bool
	err := db.QueryRow(query, vehicleID, zoneName).Scan(&isInside)
	if err != nil {
		http.Error(w, "Could not calculate spatial intersection", http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "Is vehicle %s inside %s? %v\n", vehicleID, zoneName, isInside)
}

// Fetch instantly from Redis without touching PostgreSQL
func getLatestLocation(w http.ResponseWriter, r *http.Request) {
	vehicleID := r.URL.Query().Get("vehicle_id")
	cacheKey := fmt.Sprintf("vehicle:%s", vehicleID)

	// Fetch all fields of the Hash from Redis
	data, err := rdb.HGetAll(ctx, cacheKey).Result()
	if err != nil {
		http.Error(w, "Redis error", http.StatusInternalServerError)
		return
	}

	// HGetAll returns an empty map if the key doesn't exist
	if len(data) == 0 {
		http.Error(w, "Location not found in cache", http.StatusNotFound)
		return
	}

	fmt.Fprintf(w, "Latest location for %s - Lat: %s, Lon: %s (Served instantly from Redis!)\n", vehicleID, data["lat"], data["lon"])
}
