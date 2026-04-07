package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	_ "github.com/lib/pq" // The Postgres driver
)

// LocationUpdate represents the JSON we expect from a vehicle
type LocationUpdate struct {
	VehicleID string  `json:"vehicle_id"`
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
}

var db *sql.DB

func main() {
	// 1. Connect to the PostgreSQL container
	connStr := "postgres://fleet_admin:fleet_password@db:5432/fleet_db?sslmode=disable"
	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	// 2. Set up our API routes (Using Go 1.22's new router syntax)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /location", saveLocation)
	mux.HandleFunc("GET /check-zone", checkZone)

	// 3. Start the server
	fmt.Println("🚀 Go Engine running on port 8080...")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

// Route 1: Save the incoming GPS coordinates
func saveLocation(w http.ResponseWriter, r *http.Request) {
	var loc LocationUpdate
	if err := json.NewDecoder(r.Body).Decode(&loc); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// PostGIS spatial math: ST_MakePoint takes (Longitude, Latitude)
	// ST_SetSRID sets the spatial reference system to standard GPS (4326)
	query := `
		INSERT INTO vehicle_history (vehicle_id, location) 
		VALUES ($1, ST_SetSRID(ST_MakePoint($2, $3), 4326))
	`
	_, err := db.Exec(query, loc.VehicleID, loc.Lon, loc.Lat)
	if err != nil {
		http.Error(w, "Failed to save to database", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "Location saved for %s!\n", loc.VehicleID)
}

// Route 2: Check if a vehicle is inside a specific geofence
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
