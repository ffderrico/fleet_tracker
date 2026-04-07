package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
)

type LocationUpdate struct {
	VehicleID string  `json:"vehicle_id"`
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
}

func main() {
	fmt.Println("👷 Starting background worker...")

	// 1. Connect to Postgres & Redis (Same as API)
	db, err := sql.Open("postgres", "postgres://fleet_admin:fleet_password@db:5432/fleet_db?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	rdb := redis.NewClient(&redis.Options{Addr: "redis:6379"})

	// 2. Set up the Kafka Consumer (Reader)
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{"kafka:9092"},
		Topic:   "incoming-locations",
		GroupID: "fleet-worker-group", // GroupID allows us to run multiple workers safely
	})
	defer reader.Close()

	ctx := context.Background()
	fmt.Println("🎧 Listening for locations on Kafka topic: incoming-locations...")

	// 3. Infinite loop to process messages as they arrive
	for {
		m, err := reader.ReadMessage(ctx)
		if err != nil {
			log.Printf("Failed to read message: %v\n", err)
			continue
		}

		var loc LocationUpdate
		if err := json.Unmarshal(m.Value, &loc); err != nil {
			log.Printf("Invalid JSON: %v\n", err)
			continue
		}

		// Save to Postgres
		pgQuery := `INSERT INTO vehicle_history (vehicle_id, location) VALUES ($1, ST_SetSRID(ST_MakePoint($2, $3), 4326))`
		_, err = db.Exec(pgQuery, loc.VehicleID, loc.Lon, loc.Lat)

		if err != nil {
			log.Printf("❌ Postgres Error for %s: %v\n", loc.VehicleID, err)
			continue // Skip the rest of the loop so we don't pretend it succeeded
		}

		// Save to Redis
		cacheKey := fmt.Sprintf("vehicle:%s", loc.VehicleID)
		err = rdb.HSet(ctx, cacheKey, "lat", loc.Lat, "lon", loc.Lon).Err()

		if err != nil {
			log.Printf("⚠️ Redis Error for %s: %v\n", loc.VehicleID, err)
		}

		fmt.Printf("✅ Processed and saved location for %s\n", loc.VehicleID)
	}
}
