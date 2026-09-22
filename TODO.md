## 🧠 1. Developer Experience (DX)

**Goal:** Anyone can clone the repo and get a fully seeded, running environment in minutes with a single command.

**Do:**
*   **Integrate Database Migrations:** Use a tool like `golang-migrate` to manage schema changes automatically on startup.
*   **Optimize Docker Builds:** Implement multi-stage builds and `.dockerignore` to drastically reduce build times and image sizes.

---

## 📊 2. Observability

**Goal:** Gain instant, deep visibility into system throughput, latency, and error rates without digging through logs.

**Do:**
*   **Add Prometheus Metrics:** Expose a `/metrics` endpoint on all services with essential counters, gauges, and histograms:
    *   `jobs_created_total`, `jobs_completed_total{status="complete|failed"}`
    *   `render_duration_seconds`, `upload_duration_seconds`
    *   `redis_pending_jobs`, `dlq_size`
*   **Implement Distributed Tracing:** Use OpenTelemetry to trace a `job_id` and `correlation_id` across the entire lifecycle: Orchestrator → Renderer → Distributor.

---

## 🔁 3. Reliability & Recovery

**Goal:** Build a self-healing system that can be proven to recover from failures automatically.

**Do:**
*   **Create a Chaos Testing Script:** Write a simple script to randomly kill `renderer` or `distributor` containers mid-job. Use logs and metrics to verify that the `XCLAIM` mechanism successfully recovers and completes the jobs.
*   **Implement Circuit Breakers:** In the `distributor`, add a circuit breaker for each adapter. If a destination (e.g., a specific webhook) fails repeatedly, temporarily pause deliveries to it while allowing others to continue.
*   **Build a DLQ Replay API:** Expose a secure REST endpoint in the orchestrator (e.g., `POST /api/v1/admin/dlq/replay`) to allow re-queuing of failed jobs from the web dashboard, in addition to the CLI tool.

---

## 🔒 4. Security & Compliance

**Goal:** Harden the system to be suitable for enterprise clients with strict security requirements.

**Do:**
*   **Encrypt Secrets in the Database:** Encrypt the `config` column in the `connections` table using AES-GCM or integrate with a secrets manager like HashiCorp Vault.
*   **Implement Audit Logging:** Create an `audit_logs` table and record all administrative actions (creating users, projects, connections) performed via the API or CLI.
*   **Add API Key Rotation:** Build an API endpoint that allows users to revoke an old API key and generate a new one for a project.

---

## 🧪 5. Testing & QA

**Goal:** Move from assumed reliability to measurable, verifiable reliability.

**Do:**
*   **Write End-to-End Integration Tests:** Create a Go test suite (`/tests`) that uses the API to submit a job, polls for its status, and verifies the final output.
*   **Implement a Simple Load Test:** Use a tool like `k6` or `go-wrk` to simulate 50-100 concurrent job submissions and monitor throughput, latency, and error rates.
*   **Set up Continuous Integration (CI):** Create a GitHub Actions workflow that automatically runs linting, unit tests, and builds Docker images on every push to the main branch.


---

Here is a clean, technical summary of the project architecture and logic state

### **Project Overview: News2Video**
A multi-tenant, event-driven video generation platform written in **Go**. It converts data payloads and JSON templates into short-form videos (MP4) using FFmpeg and ImageMagick.

### **Core Architecture**
The system runs on **Docker Compose** with three microservices, **PostgreSQL**, **Redis**, and **MinIO** (S3).

1.  **Orchestrator (API Gateway & Logic)**
    *   **Role:** Handles Auth (API Keys & JWT), Quotas, DB persistence, and Template Processing.
    *   **Data Flow:** Receives HTTP POST `render_request` $\to$ Merges Payload with Template $\to$ Resolves/Presigns Assets $\to$ Pushes to Redis Stream `render_jobs`.
    *   **Key File:** `orchestrator/template_processor.go` (Handles asset fallback logic and payload merging).

2.  **Renderer (Worker)**
    *   **Role:** The graphics engine. Consumes `render_jobs`.
    *   **Process:**
        1.  Downloads presigned assets to a local temp folder.
        2.  Generates static overlays (text, icons, boxes) using **ImageMagick**.
        3.  Generates background animation (Pan/Zoom) using **FFmpeg**.
        4.  Composites final video using complex FFmpeg filter chains.
        5.  Writes output to a **Shared Volume** (`WORK_DIR`).
    *   **Output:** Pushes job metadata to Redis Stream `distribution_jobs`.
    *   **Key File:** `renderer/renderer.go` (Contains `generateOverlayElement`, `renderBackgroundMotion`, `buildOverlayFilter`).

3.  **Distributor (Worker)**
    *   **Role:** Delivery agent. Consumes `distribution_jobs`.
    *   **Process:** Reads the finished video from the Shared Volume $\to$ Uploads to S3 and/or triggers Webhooks $\to$ Updates DB status to `complete` $\to$ Cleans up local files.

### **The Template Engine Model**

*   **Source of Truth:** The **JSON Template** defines the structure, styling, and **default values**.
*   **Overrides:** The **API Payload** is a flat map.
    *   **Text:** If `payload["key"]` matches a layer `name`, the template's `text_content` is overwritten.
    *   **Assets:** Uses `{{ASSET:filename.ext}}` placeholder. The Orchestrator resolves this path by checking **Project $\to$ Organization $\to$ System** buckets in order.
*   **Carousels:** `text_carousel` layers use a `sourceKey` property in the template (e.g., `"sourceKey": "headlines"`) to map to a string array in the payload.

### **Rendering Logic & Features**

*   **Text Layout (`text` layer):**
    *   **Styled** via a structured `font` object (file, color, size, align).
    *   **Constrained Mode:** If `boundingBox` has `width` AND `height`, text auto-shrinks to fit (using `caption:` without pointsize).
    *   **Wrap Mode:** If `boundingBox` has only `width`, text wraps and grows vertically (using `caption:` with fixed pointsize).
    *   **Background Boxes:** Supports solid or rounded background boxes behind text (`layer.Box`).
*   **Background Motion:**
    *   Supports modes: `linear` (start/end points), `drift` (continuous sine-wave wandering), `pulse` (rhythmic zoom), and `glance` (horizontal pan).
    *   Implemented via dynamic mathematical expressions injected into FFmpeg's `zoompan` filter.
*   **Post-Effects:**
    *   Supports styles like `film` (grain), `warm`, `cool` applied via a secondary FFmpeg pass (`applyPostEffect`).

### **Current State**
*   **Stable:** `renderer.go` handles animation chaining, text layout, and background generation correctly.
*   **Schema:** `types.go` structures (Font, BoundingBox, Motion) are synchronized across services.
*   **Builds:** Dockerfiles use multi-stage builds. `.dockerignore` handles build context size.
