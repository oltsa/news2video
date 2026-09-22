# Social Video Automation — Orchestrate · Render · Distribute

A modular, event-driven system for **automated short-form video generation** and **distribution**. Built for multi-tenant use, it takes simple data payloads, renders them into videos using flexible JSON templates, and distributes them to configured endpoints.

## What it does

- **Orchestrator (API):** Authenticates requests, applies rate limits, persists jobs in PostgreSQL, and enqueues work to Redis Streams. It also provides a full-featured API for a web dashboard.
- **Renderer (worker):** A Go-based worker that consumes render jobs, uses `ffmpeg` and `imagemagick` to generate videos from JSON templates, and enqueues a distribution job.
- **Distributor (worker):** Consumes distribution jobs and uploads video artifacts to connections such as S3, FTP or posts them directly on e.g Facebook, Tiktok, Instagram or Youtube.
- **Admin CLI:** A command-line tool for managing tenants (organizations, projects), users, templates, and dead-letter queues.

---

## Architecture

- **Queues:** Redis Streams (`render_jobs`, `distribution_jobs`) with consumer groups per service.
- **DB:** PostgreSQL (orgs, projects, users, templates, jobs, connections).
- **Storage:** MinIO/S3 for all assets (fonts, logos, images) and final video outputs.
- **Reliability:** Workers feature graceful shutdowns, dead-letter queues (DLQs) for failed jobs, and a message reclaim system (`XPENDING`/`XCLAIM`) to recover jobs from crashed workers.
- **Observability:** Structured JSON logging across all services and `/health` endpoints for monitoring.

```
API Client → Orchestrator → (render_jobs) → Renderer → Shared Volume
                                  ↘ status in Postgres ↙
           Distributor ← (distribution_jobs) ←──────────
                   ↓
           Connection adapters
```

---

## Prerequisites

- Docker & Docker Compose
- Ports used: `8080`, `8081`, `8082`, `5432`, `6379`, `9000`, `9001`

---

## Configure Environment

Create a `.env` file in the project root.

```env
# .env

# --- Core Service Addresses ---
JOB_QUEUE_ADDRESS=redis:6379
DATABASE_URL=postgres://user:password@postgres:5432/videogen

# --- Authentication ---
JWT_SECRET=a-very-secure-secret-for-local-development-only

# --- CORS for Web Dashboard ---
# Comma-separated list of allowed origins for the web dashboard API.
CORS_ALLOWED_ORIGINS=http://localhost:3000,http://127.0.0.1:3000

# --- Postgres Configuration ---
POSTGRES_USER=user
POSTGRES_PASSWORD=password
POSTGRES_DB=videogen

# --- MinIO/S3 Configuration ---
S3_ENDPOINT=http://minio:9000
S3_PUBLIC_ENDPOINT=http://localhost:9000 # Endpoint accessible from your browser
S3_ACCESS_KEY_ID=minioadmin
S3_SECRET_ACCESS_KEY=minioadmin
S3_BUCKET_NAME=videos
MINIO_ROOT_USER=minioadmin
MINIO_ROOT_PASSWORD=minioadmin

# --- Shared Directory for Renderer/Distributor ---
# Ensure the 'work' directory exists in your project root.
WORK_DIR=./work
```

---

## Run Locally

1.  **Start all services:**
    ```bash
    docker compose up --build -d
    ```

2.  **Seed initial data:**
    Use the running `cli` container to execute admin commands.
    ```bash
    # Create a demo organization and project
    docker exec news2video-cli-1 admin-cli orgs add demo-org default
    docker exec news2video-cli-1 admin-cli projects add project-a --org demo-org

    # The command will output the API Key for 'project-a'. Save it.

    # Upload all templates from the db/templates directory
    docker exec news2video-cli-1 admin-cli templates add kitchen_sink /app/db/templates/kitchen_sink.json --system
    docker exec news2video-cli-1 admin-cli templates add news_breaking /app/db/templates/news_breaking.json --system
    docker exec news2video-cli-1 admin-cli templates add news_feature_quote /app/db/templates/news_feature_quote.json --system
    docker exec news2video-cli-1 admin-cli templates add news_readmore /app/db/templates/news_readmore.json --system
    ```

3.  **Run a Smoke Test:**
    Replace `<YOUR_PROJECT_API_KEY>` with the key from the `projects add` command.

    ```bash
    # 1) Submit a render job
    API_KEY="<YOUR_PROJECT_API_KEY>"
    JOB_ID=$(curl -s -X POST http://localhost:8080/render \
      -H "Content-Type: application/json" \
      -H "X-API-Key: $API_KEY" \
      -d '{
            "templateId": "news_breaking",
            "payload": {
              "topic": "LIVE DEMO",
              "title": "System is Operational"
            }
          }' | jq -r .jobId)

    echo "Job submitted with ID: $JOB_ID"

    # 2) Poll for status (optional)
    echo "Polling for status... (Ctrl+C to stop)"
    while true; do
      curl -s -H "X-API-Key: $API_KEY" http://localhost:8080/jobs/$JOB_ID | jq .
      sleep 2
    done
    ```

On success, the final status poll will include a presigned `output_url`.

---

## Operational Behavior

- **Retries & DLQ:** Transient failures in workers are handled with retries. After exhausting retries, jobs are moved to a Dead-Letter Queue (`render_jobs_dlq` or `distribution_jobs_dlq`) for manual inspection.
- **Reclaim:** On startup, workers automatically reclaim "stuck" jobs from other workers that may have crashed, ensuring no work is lost.
- **Cleanup:** The distributor removes the job's working directory from the shared volume after successful distribution.
```