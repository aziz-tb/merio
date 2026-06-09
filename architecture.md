flowchart LR
    Browser["Browser — Vue SPAs<br/>agent desk + embeddable widget"]

    API["API binary (cmd/)<br/>HTTP API + embedded SPA<br/>domain managers"]

    Temporal{{"Temporal service<br/>task queues"}}
    Worker["Temporal worker (cmd/temporal_worker)<br/>workflows + activities<br/>AI runs · customer-id · embeddings"]

    PG[("PostgreSQL<br/>application state")]
    Redis[("Redis<br/>realtime pub/sub · run-event streams<br/>sessions · exec tokens · caches")]

    subgraph Boundary["🔒 TRUST BOUNDARY"]
        Exec["exec-runner — Python FastAPI<br/>runs operator/tool code in KVM microVMs"]
    end

    %% --- request path ---
    Browser -- "REST (cookie+CSRF · widget Bearer)" --> API
    API -- "serves SPA + /widget.js · WS push · AI-run SSE" --> Browser

    %% --- state + event bus ---
    API -- "read / write" --> PG
    API -- "sessions · rate-limit · exec tokens · caches" --> Redis
    Redis -- "subscribe (realtime) / XREAD (run events)" --> API

    %% --- durable dispatch ---
    API -- "StartWorkflow → 202 (fire-and-forget)" --> Temporal
    Temporal -- "poll task queue" --> Worker

    %% --- worker outputs ---
    Worker -- "write authoritative state" --> PG
    Worker -- "publish realtime + run events" --> Redis

    %% --- tool execution across the trust boundary ---
    Worker -- "POST /v1/execute (code + secrets + callback token)" --> Exec
    API -. "POST /v1/execute (sync ERP / customer search)" .-> Exec
    Exec -- "sandbox callback — sx_ token → HTTP API" --> API
