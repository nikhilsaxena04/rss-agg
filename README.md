# THE AUTONMIS MASTER INTERVIEW GUIDE

This is your ultimate brain-dump. It covers everything about your projects, exactly how they work under the hood, how they translate to Autonmis's tech stack (Node.js/TypeScript/MongoDB/React), and the gaps you need to know to pass this interview. 

---

## PART 1: DECONSTRUCTING YOUR PROJECTS (WHAT YOU BUILT & WHY)

If Sudhanshu asks, "Explain your Notification Broker," you cannot just read the resume. You must understand the physics of your system.

### Project 1: High-Throughput Notification Broker
**1. 1,200+ RPS and <70ms P95 Latency**
*   **What it means:** Your system handles 1,200 requests per second. P95 latency of <70ms means that 95% of all requests are processed in under 70 milliseconds. 
*   **How you achieved it:** You decoupled the architecture into 3 tiers (Router/API, Redis Queue, Worker). The Router doesn't send the email/notification itself; it just validates the request and drops it in Redis, instantly returning a 200 OK to the client. This decoupling is why it's so fast.
*   **Deeper Dive (The Mechanics):** P95 means if you have 100 requests, the 95th slowest one takes 70ms. The remaining 5 might take 200ms due to GC pauses or network blips. This proves you aren't just looking at the average (mean) which hides bad latency spikes. The decoupling means the Router thread isn't blocked by the SMTP server's TCP handshake.
*   **Autonmis Connection:** Autonmis ingests data every 60s. They need decoupling too. An API gateway accepts the payload, dumps it to storage/queue, and returns 200 OK immediately.

**2. Redis BLMOVE & Atomic Processing**
*   **How it works:** `BLMOVE` (Blocking List Move) atomically moves an item from a "Pending" queue to a "Processing" queue. If a worker crashes mid-job, the item isn't lost—it's still in the "Processing" queue. A separate cron job can sweep abandoned items back to "Pending".
*   **DLQ & Backoff:** If sending a notification fails 3 times, you move it to a Dead Letter Queue (DLQ). Exponential backoff means waiting 1s, then 2s, then 4s, etc., plus "jitter" (randomness) so all failing workers don't retry at the exact same millisecond and cause a thundering herd.
*   **Deeper Dive (The Mechanics):** Under the hood, `BLMOVE` blocks the Redis client connection but NOT the Redis single-threaded event loop. Redis uses epoll/kqueue to park the connection until data arrives, costing zero CPU. Your cron job sweeps by running an `LRANGE` to find items in "Processing" older than 5 minutes, and `LMOVE`s them back.
*   **Autonmis Connection:** They build "incident operations systems". A failed Salesforce data sync is exactly like a failed notification. It goes to a DLQ, alerts the ops team, and can be retried automatically.

**3. gRPC vs. REST**
*   **How it works:** gRPC uses HTTP/2 and Protocol Buffers (binary). It's much faster and smaller than REST (HTTP/1.1 and JSON).
*   **Deeper Dive (The Mechanics):** JSON requires CPU-heavy string parsing and reflection. Protobufs generate static structs in Go/TypeScript, allowing direct memory mapping and bitwise operations to read the data. Plus, HTTP/2 multiplexing means gRPC can send 1,000 requests concurrently over a single TCP connection, avoiding the TCP 3-way handshake overhead of HTTP/1.1.
*   **Autonmis Connection:** Autonmis uses microservices. Mentioning that you chose gRPC for inter-service communication shows you care about network bottleneck optimization, which is crucial when moving gigabytes of sync data.

**4. Observability (Prometheus, Grafana, OpenTelemetry)**
*   **How it works:** Prometheus scrapes metrics (CPU, RPS). Grafana visualizes them. OpenTelemetry does distributed tracing (following a single request across multiple microservices). 
*   **The Context Carrier:** Passing trace IDs across an async boundary (like a Redis queue) is hard. You injected the TraceID into the Redis payload metadata so the worker could pick it up and continue the trace. Sudhanshu will love this detail.
*   **Deeper Dive (How you actually coded it):** In an HTTP/gRPC request, OpenTelemetry automatically passes the `TraceID` in the headers (like `traceparent`). But Redis has no HTTP headers; it just stores strings. If your Router pushes a message to Redis, the trace "breaks." To fix this, you added a `map[string]string` field to your JSON payload struct (the Carrier). In the Router, you used OpenTelemetry's `TextMapCarrier.Inject()` to write the trace data into that map before saving to Redis. In the Worker, you read the map and used `Extract()` to put the trace back into the Go `context`. This stitches the graph back together in Jaeger, proving you understand distributed systems debugging.

### Project 2: Meta Clash
**1. Go Hexagonal Design & FSM**
*   **How it works:** Hexagonal (Ports & Adapters) isolates core business logic from databases or HTTP. FSM (Finite State Machine) ensures a game can only transition through valid states (Lobby -> Character Select -> Combat -> Result). 
*   **Deeper Dive (The Mechanics):** In code, an FSM is typically implemented using an Interface (e.g., `State`) with a `Transition(Event)` method. When an event fires, the current state executes its logic and returns the next struct implementing the `State` interface. This completely eliminates messy `if/else` spaghetti code in complex pipelines.
*   **Autonmis Connection:** Their ETL pipelines (Extract, Transform, Load) are essentially state machines. Data is Extracted, then Transformed, then Loaded. You can model their sync pipelines using FSMs to ensure strict state transitions.

**2. WebSocket Hub (Goroutines, Channels, sync.RWMutex)**
*   **How it works:** A central "Hub" struct manages all active WS connections. You used a `sync.RWMutex` to prevent race conditions when multiple users join/leave simultaneously. The 54s ping/pong heartbeat detects "ghost" connections (users who closed their laptop without cleanly disconnecting) and cleans up server memory.
*   **Deeper Dive (The Mechanics):** A `sync.RWMutex` (Read-Write Mutex) is crucial here. Broadcasting a message to 100 users only requires a Read lock (`RLock`), meaning all 100 can be read concurrently. You only acquire a Write lock (`Lock`), blocking everyone, when a user explicitly connects or disconnects from the underlying map. The 54s ping writes a control frame to the TCP socket; if it fails, the OS TCP stack returns an error, and the goroutine cleans up.
*   **Autonmis Connection:** Autonmis has real-time enterprise dashboards. They need WebSockets to stream live data (like a failed sync alert) to the UI. You know how to manage real-time connections securely at scale.

**3. AI Generation Pipeline (Jikan + Gemini)**
*   **How it works:** You fetch data from a REST API (Jikan), cache it in-memory with a TTL (Time-To-Live) to avoid rate limits, and use Gemini to generate cards. If Gemini times out, you fallback to deterministic FNV-1a hashing to auto-generate stats.
*   **Autonmis Connection:** Autonmis uses an AI/RAG layer. Fallbacks are critical for enterprise software. If their LLM provider goes down, the system must degrade gracefully, not crash.

---

## PART 2: THE GREAT TRANSLATION (GO TO NODE.JS/TYPESCRIPT)

Sudhanshu needs TypeScript/Node.js mastery. Here is how your Go knowledge translates, and what you MUST know to survive his questions.

### 1. The Event Loop (The Most Important Concept)
*   **Go:** You just type `go doSomething()` and the Go runtime scheduler maps millions of goroutines onto a few OS threads. CPU-heavy tasks don't block the server.
*   **Node.js:** Node is single-threaded (libuv handles async I/O in the background). 
    *   *The Danger:* If you run a `for` loop that takes 5 seconds, or call `JSON.parse()` on a 50MB string, the Node.js main thread completely freezes for 5 seconds. No webhooks are received. WebSockets disconnect. 
    *   *The Fix:* Use Streams (`fs.createReadStream`, `stream-json`). If you must do heavy CPU work (like complex data parsing/aggregation for Autonmis's data warehouse), you MUST use Node.js `worker_threads` (via libraries like `Piscina`). This spins up a separate V8 JavaScript engine to do the math, leaving the main thread free.
    *   *Deeper Dive (The Mechanics):* Node.js uses the V8 engine for JavaScript execution and libuv for the event loop. The event loop checks the Call Stack. If the stack has `JSON.parse()`, it executes it. It cannot process the Task Queue (where network I/O callbacks live) until the Call Stack is empty. `worker_threads` literally spawn a new V8 Isolate (a totally separate heap memory and call stack), communicating back to the main thread via ArrayBuffers.

### 2. Relational (Postgres) vs. Document (MongoDB)
*   **Your Experience:** You used Postgres (SQL) in Meta Clash with connection pooling.
*   **Autonmis:** They use MongoDB and SQL. 
*   **What you need to know:** 
    *   In Postgres, you normalize data (User table, Post table, joined by Foreign Key). 
    *   In MongoDB, you often **denormalize (embed)** data. If Autonmis pulls a "Salesforce Account" and its "Contacts", in Mongo, you might store the Contacts inside the Account document as an array. This makes reads incredibly fast (no JOINs required), but updates are heavier.
    *   *The Bottleneck:* Connection pooling is still critical in Node.js. If 50 workers try to insert 10,000 rows each simultaneously, they will exhaust the DB connection pool. You must use `db.collection.insertMany()` (Bulk Writes) in MongoDB to batch writes.
    *   *Deeper Dive (The Mechanics):* Bulk writes in MongoDB (`bulkWrite`) package multiple operations (inserts, updates, deletes) into a single BSON array payload and send it over the wire in one network trip. The MongoDB wiredTiger storage engine processes these in batches, dramatically reducing lock contention on the collections compared to thousands of individual write locks.

### 3. The "Gigabyte Sync" Problem (Redis Memory Pressure)
*   **The Trap:** If you put a 1GB Salesforce JSON dump into Redis, Redis will run out of RAM and crash. 
*   **The Solution (Claim Check Pattern):**
    1. The API receives the 1GB payload.
    2. The API immediately streams it to AWS S3.
    3. The API puts a tiny message in Redis: `{"job_id": 123, "s3_key": "salesforce/sync_123.json"}`.
    4. The Node.js worker pulls the tiny message from Redis, streams the file from S3, parses it, and saves to MongoDB.
    *   *Deeper Dive (The Mechanics):* Streaming to S3 uses `Multipart Upload`. Instead of holding the 1GB file in Node.js RAM, the incoming HTTP request stream is piped directly to the outgoing S3 upload stream in 5MB chunks. V8 garbage collects the chunks immediately. The worker downloads it the exact same way, piping the S3 download stream into the `stream-json` parser, maintaining a constant RAM usage of ~20MB regardless of file size.
    *This proves you know how to architect for enterprise scale, not just side projects.*

---

## PART 3: FRONTEND, DASHBOARDS, & UX (AUTONMIS UI)

Autonmis needs complex UI dashboarding and "motion-driven interfaces" (Framer).

### 1. Rendering Massive Datasets (Virtualization)
*   If Autonmis syncs 10,000 rows of data, you cannot render 10,000 `<tr>` elements in React. The DOM will freeze. 
*   **The Solution:** Use **Virtualization** (e.g., `react-window` or `react-virtuoso`). This only renders the 20 rows currently visible on the screen. As the user scrolls, it recycles the DOM nodes. This is mandatory for enterprise dashboards.

### 2. State Management for Complex Dashboards
*   Dashboards have filters, time-ranges, and drill-downs. 
*   **The Solution:** Don't use `useState` for everything. Use URL Query Parameters for filter state (so users can share dashboard links). Use a global state manager (Zustand or Redux) or a data-fetching library (React Query / SWR) to cache API responses and manage loading/error states automatically.

### 3. Motion & UX (Framer)
*   Autonmis mentioned "Framer for motion-driven interfaces."
*   **The Concept:** Micro-animations (like a subtle spring animation when opening a dropdown, or a skeleton loader transitioning smoothly into a chart) reduce perceived latency. 
*   **How you pitch it:** "In Meta Clash, I managed deterministic UI states. With Framer Motion in React, I would map backend FSM states (Syncing -> Transforming -> Complete) to layout animations, giving users visual feedback that the system is working, which builds trust in B2B tools."

---

## PART 4: AI & RAG (AUTONMIS CORE INTEL)

They use an "AI/RAG layer to auto-build dashboards, reports, and KPIs from plain language."

### 1. How RAG (Retrieval-Augmented Generation) Works
If they ask how you'd build their AI layer, explain this:
1.  **Ingestion:** When data syncs from Snowflake, you convert the metadata (schema, column names) into vector embeddings (using OpenAI embeddings API).
2.  **Storage:** Store these vectors in a Vector Database (like Pinecone, Milvus, or Postgres pgvector).
3.  **Retrieval:** When an executive types, "Show me churn by cohort," you convert that question into a vector, do a similarity search in the DB, and retrieve the relevant Snowflake schema details.
4.  **Generation:** You pass the question AND the retrieved schema to the LLM. The LLM generates the SQL query or the JSON configuration for the dashboard.
    *   *Deeper Dive (The Mechanics):* An embedding is an array of floating-point numbers (e.g., `[0.014, -0.421, ...]`) representing semantic meaning in a 1,536-dimensional space. "Revenue" and "Sales" are mapped close together. The Vector DB performs a Cosine Similarity search (calculating the dot product of the vectors) to find schemas semantically related to the user's query, even if the exact keywords don't match.

### 2. Preventing Hallucinations (Crucial for B2B)
*   An LLM hallucinating a game character in Meta Clash is funny. An LLM hallucinating a financial metric in Autonmis is a lawsuit.
*   **How you handle it:** 
    1. Use Temperature = 0 for the LLM to make it deterministic.
    2. Force the LLM to output strict JSON (using OpenAI's JSON mode or structured outputs).
    3. Use `Zod` (a TypeScript validation library) to parse the LLM's output. If the LLM generates a metric that doesn't exist in the database, `Zod` throws an error, and the system automatically prompts the LLM to correct itself (self-healing).

---

## PART 5: SUDHANSHU'S ATTACK VECTORS (HOW TO DEFEND)

Here are the exact questions Sudhanshu will ask to break you, and your definitive answers.

### Attack 1: "Go is fast, Node is slow. Why shouldn't we just rewrite Autonmis in Go?"
*   **Your Defense:** "Go is amazing for CPU-bound tasks and raw network throughput (like my Notification Broker). But Autonmis integrates with 50+ third-party APIs. The Node.js/TypeScript ecosystem has infinitely better SDKs for Salesforce, Snowflake, and AWS. Furthermore, using TypeScript across the entire stack (React frontend + Node backend) allows us to share Zod schemas and types, massively accelerating feature delivery in a lean startup. We can solve Node's CPU limitations with worker pools, but we can't easily replicate npm's ecosystem in Go."

### Attack 2: "Our dashboard freezes when a live WebSocket pushes 1,000 updates a second. Fix it."
*   **Your Defense:** "The React DOM can't handle 1,000 renders a second. I would implement **Throttling/Debouncing** on the frontend. The WebSocket receives all 1,000 events, but instead of calling `setState` 1,000 times, we push the updates into an array (a buffer). Every 200ms, a `requestAnimationFrame` or `setInterval` loop flushes the buffer and updates the state exactly once. This gives real-time feel without nuking the browser."

### Attack 3: "A worker process crashes midway through transforming a massive Snowflake dataset. How do you recover?"
*   **Your Defense:** "Idempotency. In my Notification system, I used atomic queues. Here, every batch job must be idempotent (running it twice has the same result as running it once). I would use a transactional outbox pattern or MongoDB multi-document transactions. If the worker dies on chunk 5 of 10, the DLQ orchestrator restarts the job, and the DB upsert logic ensures we simply overwrite or ignore chunks 1-4, resuming safely without duplicating data."

### Attack 4: "We have an RBAC (Role Based Access Control) system. How do you prevent a regular user from querying executive dashboards via our AI?"
*   **Your Defense:** "In Meta Clash, I used stateless JWTs for auth. I would embed the user's `role` and `tenant_id` inside the JWT payload. When the AI generates a SQL query based on the user's prompt, we never run it raw. We intercept it in the backend and automatically inject a `WHERE tenant_id = ? AND clearance_level <= ?` clause using a SQL AST (Abstract Syntax Tree) parser, or we restrict the RAG vector search to only include metadata the user has permissions for. Zero-trust AI."

---

## PART 6: THE DEEP CUTS (Explaining Your Infrastructure)

When Sudhanshu asks about your deployment tools, do NOT just say "I used Docker to containerize it." You need to explain the mechanics of *why* and *how*. Here is how you explain the infrastructure tools from your resume like a Senior Engineer:

### 1. Docker (Multi-Stage Distroless Builds)
*   **What you don't say:** "I put my app in a container so it runs anywhere."
*   **What you say (The Mechanics):** "I used multi-stage builds. In Stage 1, I use a heavy `golang:alpine` image to download dependencies and compile the Go binary. In Stage 2, I copy *only* that compiled binary into a Google `distroless` image. Distroless has no OS package manager, no shell (`/bin/sh`), and no core utilities. This reduces the final image size to <20MB and completely eliminates the attack surface for remote code execution (RCE) vulnerabilities. If a hacker breaches the container, there is literally no shell for them to execute commands on."

### 2. Kubernetes (Deployments vs. StatefulSets vs. HPA)
*   **What you don't say:** "I used Kubernetes to manage my containers."
*   **What you say (The Mechanics):** "I architected the cluster by separating state. The API Routers and Workers are stateless, so they run as Kubernetes **Deployments**. If a pod dies, K8s spins up an identical one instantly. However, Redis and Postgres hold state. They must run as **StatefulSets**, which guarantees stable network IDs (like `redis-0`) and persistent volume claims that survive pod restarts. I also attached a **Horizontal Pod Autoscaler (HPA)** to the Worker Deployment, which reads the Prometheus CPU metrics and dynamically adds more worker pods if CPU usage spikes above 70%."

### 3. JWT (Stateless Authentication)
*   **What you don't say:** "I used JWTs to log users in."
*   **What you say (The Mechanics):** "I used stateless JWTs to reduce database load. A JWT has 3 parts: Header, Payload (where I store `user_id`), and Signature. Because the backend signs the payload with a secret cryptographic key (HMAC SHA-256), any of my microservices can verify the token's authenticity just by doing math. We never have to do a database lookup to check if a session is valid. This is crucial for high-throughput APIs where DB reads are the bottleneck."

### 4. Prometheus (Pull Architecture)
*   **What you don't say:** "I used Prometheus to track metrics."
*   **What you say (The Mechanics):** "I chose Prometheus because it uses a **Pull model**, not a Push model. My Go services don't push metrics to a central server (which can cause a DDOS attack on the monitoring server during traffic spikes). Instead, my services just expose a `/metrics` HTTP endpoint in memory. The Prometheus server scrapes (pulls) that endpoint every 15 seconds. If a worker is overwhelmed, Prometheus might fail to scrape it, but the worker's CPU isn't wasted trying to push metrics."

### 5. PostgreSQL & Database Connection Pooling
*   **What you don't say:** "I saved data to a Postgres database."
*   **What you say (The Mechanics):** "Opening a new TCP connection to a database requires a 3-way handshake and authentication, which is extremely slow. In Meta Clash, I used a **Connection Pool**. This creates a pool of, say, 20 long-lived TCP connections to Postgres when the server starts. When a user requests their match history, the worker borrows a connection from the pool, runs the query, and returns the connection. This prevents connection exhaustion and keeps latency in the single digits."

---

## PART 7: ADVANCED FRONTEND & DEPLOYMENT GOTCHAS (The Meta Clash Hard Lessons)

Sudhanshu will want to know if you understand frontend deployments and network protocols, not just backend code. Use these real battle scars from Meta Clash:

### 1. WebSockets vs. HTTP 301 Redirects
*   **The Gotcha:** You had a bug in Meta Clash where the Next.js frontend occasionally requested `//api/ws` instead of `/api/ws`.
*   **The Senior Explanation:** "The Go router automatically responded with an HTTP 301 Redirect to strip the double slash. Browsers can easily follow 301 redirects for standard HTTP requests, but the **WebSocket protocol (RFC 6455) explicitly does not support following redirects during the HTTP Upgrade handshake.** The connection just silently died. I fixed it by writing a strict URL sanitizer in the Next.js client before initiating the `new WebSocket()` connection."

### 2. Next.js on Vercel vs. Docker (The Standalone Conflict)
*   **The Gotcha:** You used `output: 'standalone'` in Next.js to make a tiny Docker image, but it broke your Vercel deployment.
*   **The Senior Explanation:** "When deploying Next.js to Docker, `output: 'standalone'` is amazing because it traces file dependencies and creates a minimal Node.js server. But when deploying to Vercel's Edge network, Vercel relies on the default build output to auto-generate its proprietary Serverless Functions. Leaving `standalone` turned on conflicted with Vercel's build engine and caused fatal boot errors (`FUNCTION_INVOCATION_FAILED`). You have to architect your CI pipeline to toggle this config based on the deployment target."

### 3. In-Memory LRU Caching & Rate Limits
*   **The Gotcha:** Your game queried a 3rd party API (Jikan) which had strict rate limits.
*   **The Senior Explanation:** "To prevent getting IP-banned by the API, I built an in-memory **LRU (Least Recently Used) Cache** with a 100-entry cap and a 1-hour TTL (Time-to-Live). Because servers handle requests concurrently, a simple map isn't thread-safe. In Go, I used a `sync.RWMutex` to lock the cache during writes, preventing race conditions. If Autonmis queries Salesforce heavily, we must implement a similar TTL caching layer to protect our 3rd-party API quotas."

---

## PART 8: SUDHANSHU'S FINAL BOSS ATTACKS (Agentic Platform Scenarios)

I researched Autonmis's actual business model. They are an **"Agentic Platform."** This means they don't just build dashboards; they build autonomous AI agents that monitor data pipelines, detect anomalies, and take action (like fixing a broken sync or sending an email) across ERPs and CRMs. Sudhanshu will test if you can build *safe* autonomous systems.

### Attack 5: "Our AI Agent autonomously fixes stuck Salesforce syncs. How do you ensure a hallucinating AI doesn't accidentally delete production data?"
*   **The Trap:** Do you understand AI safety, sandboxing, and RBAC in autonomous systems? If you say "I'll tell the prompt not to delete data," you instantly fail.
*   **Your Defense:** "Prompt engineering isn't a security boundary. You need a **Human-in-the-loop (HITL)** interceptor combined with database transactions. In Meta Clash, I used an FSM (Finite State Machine). For Autonmis, the Agent should never execute queries directly on prod. It generates the SQL/API payload and moves the system to a `pending_approval` state. We then run a 'dry-run' (e.g., `BEGIN; ... ROLLBACK;` in Postgres) to calculate the blast radius. If the blast radius exceeds a safety threshold (e.g., affects >10 rows), it halts and sends a WebSocket alert to a human dashboard for approval. The agent's database role only has permissions to *propose* actions, not commit them."

### Attack 6: "Our users type plain English like 'Fix the inventory delay.' How do you architect a backend that translates that into 5 different API calls across Snowflake and our ERP?"
*   **The Trap:** Checking if you understand modern LLM orchestration (Function/Tool Calling) versus basic RAG.
*   **Your Defense:** "Basic RAG isn't enough; you need an orchestration layer using **LLM Tool Calling** (similar to how I orchestrated Claude MCP agents in my SDLC). You provide the LLM with a strict JSON schema of your available backend tools (e.g., `querySnowflake`, `fetchERP`). When the user gives the command, the LLM doesn't generate text; it responds with a JSON array of function calls. My Node.js workers parse that JSON, execute the API calls concurrently using `Promise.all()`, and feed the raw data back to the LLM to generate the final human-readable summary. The LLM acts as a router, and the deterministic backend code executes the actual logic."

### Attack 7: "What happens when our Agent makes an API call to a third-party ERP, and the ERP just hangs forever without responding?"
*   **The Trap:** Checking your knowledge of network timeouts, context cancellation, and thread exhaustion.
*   **Your Defense:** "If you don't bound your requests, your Node.js workers or Go routines will suffer from resource exhaustion and take down the entire platform. In Go, I would pass a `context.WithTimeout` to the HTTP client. In Node.js, I would pass an `AbortController` signal to the `fetch` request. If the ERP hangs for more than 5 seconds, the abort signal fires, instantly killing the TCP connection. The worker catches the timeout error, logs it via structured logging, and gracefully escalates the job to the Dead Letter Queue (DLQ) without locking up the event loop."

---

## SUMMARY CHECKLIST FOR THE INTERVIEW
1.  **Never say "I don't know."** Say, "I haven't implemented that specifically, but based on my experience with X, I would architect it by doing Y."
2.  **Always map it to Business Ops.** Autonmis isn't building a tech demo; they are de-risking operations. Talk about "saving executive time," "preventing data loss," and "building trust through observability."
3.  **Speak TypeScript.** Use words like `Promises`, `Event Loop`, `Worker Threads`, `Zod schemas`, `Stream pipelines`, and `V8 Isolate`.
4.  **Stand your ground.** Sudhanshu is testing your confidence. When he challenges your architecture, acknowledge his point, but logically defend your tradeoffs. 

You have the systems knowledge. You built a distributed gRPC broker and a real-time WebSocket state machine. You are ready.
