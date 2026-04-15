# 🛴 Fleet Tracker

A production-grade, event-driven geospatial microservices architecture designed to ingest, process, and visualize high-frequency vehicle telemetry data in real-time.

Built as a robust portfolio piece demonstrating scalable backend engineering, this project mimics the core infrastructure used by modern ride-share and micro-mobility companies (e.g., Uber, Lime). It handles decoupled data ingestion via Kafka, real-time caching and Pub/Sub via Redis, long-term spatial storage via PostgreSQL/PostGIS, and live frontend updates via WebSockets—all orchestrated within a local Kubernetes cluster.

## 🏗️ Architecture & Data Flow

1. **Ingestion**: A vehicle sends GPS coordinates to the **Go API** (`POST /location`).
2. **Message Queue**: To prevent database bottlenecks during traffic spikes, the API instantly publishes the payload to an **Apache Kafka** topic (`incoming-locations`) and returns a `202 Accepted` response.
3. **Background Processing**: A separate **Go Worker** consumes messages from Kafka at its own pace.
4. **Dual-Write Storage**:
   * The Worker updates the vehicle's "live" location in **Redis** for lightning-fast reads.
   * The Worker saves the historical record into **PostgreSQL** (with **PostGIS**), enabling complex geospatial queries (like geofencing).
5. **Real-Time Broadcasting**: Upon saving, the Worker publishes the data to a **Redis Pub/Sub** channel (`live-locations`).
6. **WebSocket Hub & UI**: The Go API listens to the Redis Pub/Sub channel and broadcasts the coordinates over a **WebSocket** connection to an HTML/JS frontend, which dynamically moves the vehicle icons on a **Leaflet.js** map.
7. **Orchestration**: The entire stack is containerized and managed by **Kubernetes** (`kind`), utilizing Deployments, Services (NodePorts), and Persistent Volume Claims (PVCs) for stateful data.

## 🛠️ Tech Stack

* **Languages:** Go (Golang) 1.22, JavaScript, HTML/CSS
* **Message Broker:** Apache Kafka
* **Databases / Caches:** PostgreSQL, PostGIS, Redis
* **Networking / Real-Time:** WebSockets (Gorilla), Redis Pub/Sub
* **Infrastructure & DevOps:** Docker, Kubernetes (`kind`), KubeDNS
* **Frontend:** Leaflet.js, OpenStreetMap

## 🚀 Getting Started (Local Kubernetes / Dev Container)

This project is designed to run locally using Docker-outside-of-Docker (DooD) and `kind` (Kubernetes IN Docker).

### 1. Provision the Infrastructure
Start your cluster and bridge the networks so your host can resolve Kubernetes DNS:
```bash
kind create cluster --name fleet-cluster
docker network connect kind $HOSTNAME
```

### 2. Build and Load Docker Images
Build the Go microservices and push them into the `kind` cluster's internal registry:
```bash
docker build -t fleet-api:1.0 -f Dockerfile.api .
docker build -t fleet-worker:1.0 -f Dockerfile.worker .

kind load docker-image fleet-api:1.0 --name fleet-cluster
kind load docker-image fleet-worker:1.0 --name fleet-cluster
```

### 3. Deploy to Kubernetes
Apply the infrastructure (Postgres PVC, Redis, Kafka) and the application manifests:
```bash
kubectl apply -f k8s/infrastructure.yaml
kubectl apply -f k8s/apps.yaml
```
*(Note: If the Go pods start before Kafka is fully ready, Kubernetes will automatically self-heal and restart them. You can manually force a sync with `kubectl rollout restart deployment fleet-api fleet-worker`)*.

### 4. Initialize the Spatial Database
Exec into the Postgres pod and create the PostGIS extension, tables, and a test Geofence:
```bash
kubectl exec -it deployment/db -- psql -U fleet_admin -d fleet_db
```
```sql
CREATE EXTENSION IF NOT EXISTS postgis;

CREATE TABLE geofences (id SERIAL PRIMARY KEY, name VARCHAR(50) NOT NULL, geom geometry(Polygon, 4326));
CREATE TABLE vehicle_history (id SERIAL PRIMARY KEY, vehicle_id VARCHAR(50) NOT NULL, location geometry(Point, 4326), timestamp TIMESTAMPTZ DEFAULT NOW());

INSERT INTO geofences (name, geom) VALUES ('Downtown', ST_SetSRID(ST_MakePolygon(ST_GeomFromText('LINESTRING(-49.38 0.00, -49.38 0.05, -49.33 0.05, -49.33 0.00, -49.38 0.00)')), 4326));
```

## 🗺️ Live Mission Control Map

The project includes a real-time web UI to visualize data streaming through the system.

1. Expose the API to your host machine:
   ```bash
   kubectl port-forward --address 0.0.0.0 service/fleet-api 8080:8080
   ```
2. Open `index.html` in your web browser.
3. Simulate a fleet of moving vehicles using this bash loop:
   ```bash
   for i in {1..9}; do curl -s -X POST http://localhost:8080/location -H "Content-Type: application/json" -d "{\\"vehicle_id\\": \\"scooter-ghost\\", \\"lat\\": -20.81$i, \\"lon\\": -49.37$i}"; sleep 1; done
   ```
4. Watch the scooter icon drive across your map in real-time!

## 📡 API Endpoints

**1. Ingest Location (`POST /location`)**
Drops GPS telemetry into Kafka.
```bash
curl -X POST http://localhost:8080/location \\
     -H "Content-Type: application/json" \\
     -d '{"vehicle_id": "scooter-001", "lat": -20.81, "lon": -49.37}'
```

**2. Get Latest Location (`GET /latest-location`)**
Lightning-fast cache lookup via Redis.
```bash
curl "http://localhost:8080/latest-location?vehicle_id=scooter-001"
```

**3. Check Geofence (`GET /check-zone`)**
PostGIS spatial math (`ST_Intersects`) to verify if a vehicle is inside a polygon.
```bash
curl "http://localhost:8080/check-zone?vehicle_id=scooter-001&zone=Downtown"
```

**4. WebSocket Hub (`ws://localhost:8080/ws`)**
Maintains a persistent connection to broadcast Redis Pub/Sub events directly to browsers.
