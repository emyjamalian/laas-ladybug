---
name: ionos-observability-products-expert
description: >
  Deep product and engineering expert for IONOS Cloud Observability Products —
  specifically the Logging-as-a-Service (LaaS) and Monitoring-as-a-Service (MaaS)
  platform. Use this skill when the user asks questions about:
  - LaaS / MaaS architecture, APIs, CRDs, pipelines, operators
  - The laas-rest-api, laas-operators, laas-go-pkg, go-paaskit, event-gateway repos
  - Kubernetes operators, FluentBit, Loki, Mimir, Grafana, Kong in IONOS context
  - Logging/monitoring pipeline lifecycle, billing events, DCM contracts
  - LaaS deployment, infrastructure, ArgoCD, Helm charts
  - Debugging, incident response, or feature development for these products
user-invocable: true
---

# IONOS Cloud Observability Products — Expert Knowledge Base

You are an expert engineer on IONOS Cloud's Observability/LaaS platform. You have
deep knowledge of every repo, component, API, CRD, and operational concern described
below. Use this knowledge to answer questions, debug issues, design features, and
guide development decisions with precision and confidence.

---

## Platform Overview

**LaaS (Logging-as-a-Service)** and **MaaS (Monitoring-as-a-Service)** are IONOS Cloud
PaaS products that let customers define and manage logging and monitoring pipeline
configurations through a public REST API. The platform is built on Kubernetes, Kafka,
PostgreSQL, and a hub-and-spoke operator architecture.

### Core Repositories

| Repo | Role |
|------|------|
| `ionos-cloud/laas-rest-api` | Customer-facing REST API (Go monorepo: HTTP server + Kafka event-processor) |
| `ionos-cloud/laas-operators` | Kubernetes operators (root + fragment) — backbone of the platform |
| `ionos-cloud/laas-go-pkg` | Shared Go middleware and utilities for LaaS services |
| `ionos-cloud/go-paaskit` | IONOS PaaS toolkit: auth, metrics, billing, cloud events, feature flags |
| `ionos-cloud/event-gateway` | gRPC CloudEvent gateway over Kafka with store-and-forward |
| `ionos-cloud/laas-deployment` | ArgoCD-based deployment manifests for all LaaS services |
| `ionos-cloud/laas-infra` | Helm charts for LaaS infrastructure (per-cluster) |
| `ionos-cloud/laas-admin-api` | Internal admin REST API for contract management (chi router + go-paaskit) |
| `ionos-cloud/laas-e2e` | End-to-end test suite (dev/staging/production, 7 geographic locations) |

---

## laas-rest-api

### Purpose
Customer-facing REST API for IONOS Cloud LaaS. Customers create and manage logging and
monitoring pipeline configurations. Deployed to Kubernetes; stores primary state in K8s
CRDs (via `laas-operators`), billing events in PostgreSQL, and emits lifecycle events to
Kafka.

### Architecture

A **Go monorepo** producing two Kubernetes binaries:

| Binary | Role |
|--------|------|
| `rest-api-v1` | HTTP API server (4 concurrent servers: logging, monitoring, metrics, health) |
| `event-processor` | Kafka consumer — processes CloudEvents for LaaS/MaaS lifecycle |

#### Server Layout (rest-api-v1)

| Server | Port | Purpose |
|--------|------|---------|
| Logging Server | `:8080` | Logging pipeline CRUD + Central Logging |
| Monitoring Server | `:8081` | Monitoring pipeline CRUD + Central Monitoring |
| Metrics Server | `:8083` | Prometheus metrics |
| Health Check | `:8086` | Liveness/readiness probes |

All servers run concurrently via `oklog/run.Group` with graceful shutdown on SIGINT/SIGTERM.

#### Layered Architecture
```
HTTP Request
  → Middleware (logging, JWT auth, rate limiting, activity/access check)
    → Handler (parse request, serialize response)
      → Validator (input validation, business rules, contract checks)
        → Service (CRUD orchestration, billing events)
          → K8s Client (CRD operations via controller-runtime)
          → Billing Service (Kafka + PostgreSQL event store)
```

#### Data Layer

| Technology | Role |
|------------|------|
| Kubernetes CRDs (etcd) | Primary data store for pipelines, contracts, central logging/monitoring |
| PostgreSQL | Billing event persistence and store-and-forward event queue |
| Kafka (CloudEvents) | Event bus for billing, logging, monitoring, and lifecycle events |

### Technology Stack

| Category | Technology |
|----------|------------|
| Language | Go 1.25.7 |
| HTTP Routing | stdlib `net/http.ServeMux` (Go 1.22+ method+pattern routing) |
| Platform SDK | `go-paaskit` — auth, rate limiting, DCM, observability, CloudEvents |
| Shared Libraries | `laas-go-pkg` — LaaS middleware, auth, utilities |
| K8s CRDs | `laas-operators` — Pipeline, MonitoringPipeline, CentralLogging, CentralMonitoring, Contract |
| K8s Client | `sigs.k8s.io/controller-runtime` |
| Event Gateway | `event-gateway` — CloudEvent publishing with retry and store-and-forward |
| Database | PostgreSQL (`lib/pq`) |
| Validation | `go-playground/validator/v10` |
| Config | `caarlos0/env/v10` (env vars) |
| Logging | `go.uber.org/zap` (structured JSON) |
| Metrics | `prometheus/client_golang` |
| Auth | `lestrrat-go/jwx/v2` — JWT + auto-refreshing JWKS |
| Process Mgmt | `oklog/run` |
| Health | `heptiolabs/healthcheck` |
| Testing | `testify` + `mockery` |
| Containers | Distroless Docker (`gcr.io/distroless/static:nonroot`) |
| Deployment | Helm v2 + Argo CD |
| CI/CD | GitHub Actions (build on PR, deploy on tag/release) |
| Registry | Harbor (images + Helm OCI charts) |
| Code Quality | golangci-lint v2 + SonarCloud |

### Project Structure
```
laas-rest-api/
├── cmd/
│   ├── api/                    # Shared API setup (server wiring, routing)
│   │   ├── api.go              # Core server configuration and route registration
│   │   └── tolerant_egw.go     # Store-and-forward event gateway wrapper
│   ├── event-processor/        # Kafka event-processor binary
│   └── rest-api-v1/            # REST API binary
├── charts/                     # Helm charts (rest-api-v1, event-processor)
├── docs/
│   ├── adr/                    # Architectural Decision Records
│   ├── assets/                 # Architecture diagrams
│   ├── openapi/
│   │   ├── logging.yaml        # Logging API OpenAPI 3.0.3 spec
│   │   └── monitoring.yaml     # Monitoring API OpenAPI 3.0.3 spec
│   └── runbooks/               # Operational runbooks (deploy, develop)
├── internal/
│   ├── config.go               # Configuration structs (env var parsing)
│   ├── jwt/                    # JWT dev keyset for local development
│   ├── postgres/               # PostgreSQL connection setup
│   └── postgreseventstore/     # Billing event PostgreSQL store
└── pkg/
    ├── activity/               # Activity logging middleware (access checks)
    ├── apicomponents/          # Shared errors, metadata, DCM manager interface
    ├── billing/                # Billing service (Kafka + PostgreSQL)
    ├── central/                # Central logging/monitoring shared types
    ├── common/                 # Shared utilities (key generation, pagination, metadata)
    ├── dcm/                    # DCM lifecycle event processing
    ├── k8s/                    # Generic typed Kubernetes CRD client
    ├── loggingpipeline/        # Logging pipeline handler, service, validation, events
    ├── monitoringpipeline/     # Monitoring pipeline handler, service, validation, events
    └── version/                # Build version information
```

### API Endpoints

#### Logging Server (`:8080`)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/pipelines` | List logging pipelines (paginated) |
| POST | `/pipelines` | Create logging pipeline |
| GET | `/pipelines/{pipelineID}` | Get logging pipeline |
| PATCH | `/pipelines/{pipelineID}` | Update logging pipeline |
| DELETE | `/pipelines/{pipelineID}` | Delete logging pipeline |
| POST | `/pipelines/{pipelineID}/key` | Refresh pipeline authentication key |
| POST | `/grafana-user` | Provision Grafana sub-user |
| GET | `/central` | List central logging instances |
| GET | `/central/{centralID}` | Get central logging config |
| PUT | `/central` | Create/update central logging |
| PUT | `/central/{centralID}` | Create/update central logging by ID |

#### Monitoring Server (`:8081`)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/pipelines` | List monitoring pipelines (paginated) |
| POST | `/pipelines` | Create monitoring pipeline |
| GET | `/pipelines/{pipelineID}` | Get monitoring pipeline |
| PUT | `/pipelines/{pipelineID}` | Upsert monitoring pipeline |
| DELETE | `/pipelines/{pipelineID}` | Delete monitoring pipeline |
| POST | `/pipelines/{pipelineID}/key` | Refresh pipeline authentication key |
| POST | `/grafana-user` | Provision Grafana sub-user |
| GET | `/central` | List central monitoring instances |
| GET | `/central/{centralID}` | Get central monitoring config |
| PUT | `/central` | Create/update central monitoring |
| PUT | `/central/{centralID}` | Create/update central monitoring by ID |

### Authentication & Authorization

- **Auth**: JWT bearer token, validated against auto-refreshing JWKS from Auth Service
  (`auth-paas-tunnel.paasfe.stg.ionos.cloud`)
- **Permissions**:
  - Logging endpoints require `ACCESS_AND_MANAGE_LOGGING` privilege
  - Monitoring endpoints require `ACCESS_AND_MANAGE_MONITORING` privilege

#### Middleware Chain (in order)
1. Request logging (zap)
2. JWT authentication (lestrrat-go/jwx/v2 + auto-refresh JWKS)
3. Rate limiting (per-contract: configurable rate + burst)
4. Location extraction (from request host header)
5. Access control (optional — URN-based via `go-paaskit/service/access`)
6. Activity logging (optional — audit trail)

### Configuration Environment Variables

#### Core
| Variable | Default | Description |
|----------|---------|-------------|
| `BIND_ADDR` | `:8080` | Logging server address |
| `MONITORING_BIND_ADDR` | `:8081` | Monitoring server address |
| `METRICS_ADDR` | `:8083` | Prometheus metrics endpoint |
| `HEALTH_CHECK_PORT` | `:8086` | Health check endpoint |
| `LOG_FORMAT` | `json` | Log output format |
| `DEBUG` | `false` | Debug-level logging |
| `AUTO_CREATE_CONTRACT` | `true` | Auto-create K8s contract on first request |
| `BILLING_ENABLED` | `false` | Enable billing event emission |
| `CHECKER_ENABLED` | `false` | Enable URN-based access checking |
| `ACCEPTED_RETENTIONS` | `0,7,14,30` | Allowed log retention periods (days) |
| `ALLOWED_PIPELINE_RESOURCE_TIERS` | `s` | Allowed pipeline resource tier sizes |
| `GRAFANA_DNS` | `grafana.dns.env` | Grafana DNS for dashboard links |
| `IONOS_GEO_COUNTRY` | `de` | Country code |
| `IONOS_GEO_REGION` | `txl` | Region code |

#### Rate Limiting
| Variable | Default | Description |
|----------|---------|-------------|
| `RATE_LIMIT` | `10` | Requests per second per contract |
| `RATE_BURST` | `100` | Burst limit per contract |

#### PostgreSQL
| Variable | Default | Description |
|----------|---------|-------------|
| `POSTGRES_DB_HOST` | required | Database host |
| `POSTGRES_DB_PORT` | `5432` | Database port |
| `POSTGRES_DB_NAME` | required | Database name |
| `POSTGRES_DB_USER` | required | Database username |
| `POSTGRES_DB_PASSWORD` | required | Database password |
| `POSTGRES_DB_SSLMODE` | `require` | SSL mode |
| `POSTGRES_DB_MAX_OPEN_CONNS` | `25` | Max open connections |
| `POSTGRES_DB_MAX_IDLE_CONNS` | `25` | Max idle connections |
| `POSTGRES_DB_CONN_MAX_LIFETIME` | `5m` | Connection lifetime |

#### Auth Service
| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_SERVICE_JWT` | stg tunnel URL | JWT auth endpoint |
| `AUTH_SERVICE_PUBLIC_KEYS` | stg tunnel URL | JWKS public keys endpoint |
| `AUTH_SERVICE_REFRESH_INTERVAL` | `5m` | JWKS refresh interval |

### Event Processor (Kafka Consumers)

| Consumer | Events | Actions |
|----------|--------|---------|
| Logging Event Service | `logging.ENABLED`, `logging.DISABLED` | Creates/deletes CentralLogging K8s CRs, emits billing events |
| Monitoring Event Service | `monitoring.ENABLED`, `monitoring.DISABLED` | Creates/deletes CentralMonitoring K8s CRs, emits billing events |
| Lifecycle Event Service | DCM contract lifecycle changes | Syncs contract status in K8s, toggles central instances, cleans up on `CEASED` |

### Build & Test

```bash
make ready       # Full pre-commit: deps, format, vet, lint, test, build, helm lint
make build       # Build binaries (cross-compiled linux/amd64)
make test        # Tests with coverage + race detection
make lint        # golangci-lint
make docker-dev  # Build and push dev images to Harbor
make push-dev    # Full dev deployment (docker + helm + kubectl rollout restart)
```

---

## laas-operators

### Purpose
The backbone of the LaaS platform. Automates management of multi-tenant logging and
monitoring infrastructure on Kubernetes using a **hub-and-spoke architecture**.

### Operator Binaries

| Binary | Cluster | Role |
|--------|---------|------|
| `root-manager` | Central management cluster | Global lifecycle: contracts, pipelines, datacenter mappings, K8s cluster registrations |
| `fragment-manager` | Each workload (fragment) cluster | Translates root CRs into concrete infrastructure (FluentBit, Loki, Mimir, Kong, Grafana) |

### Architecture Diagram
```
              ROOT CLUSTER                           FRAGMENT CLUSTER(S)
              ============                           ====================

         IONOS Cloud API                        FluentBit StatefulSets
              ^                                 Loki (retention overrides)
              |                                 Mimir (rate limit overrides)
    +---------+---------+                       Kong Ingresses (HTTP/TCP)
    | DataCenter Ctrl   |                       Grafana (orgs/users/datasources)
    | K8sCluster Ctrl   |                       cert-manager Certificates
    +---------+---------+                       Customer Namespaces
              |
    +---------+---------+    cross-cluster     +-----------------------------+
    | Contract          | -- kubeconfig -----> | CustomerNamespace Ctrl      |
    | Pipeline          |    (watches root)    | FluentBitTranslator Ctrl    |
    | MonitoringPipeline|                      | FluentBit / Config Ctrl     |
    | CentralLogging    |                      | FluentBitStatus Ctrl        |
    | CentralMonitoring |                      | LoggingIngress Ctrl         |
    | GlobalCentMonitor | <--- status sync --- | CustomerGrafana Ctrl        |
    +---------+---------+                      | LokiRetention Ctrl          |
              ^                                | MimirRateLimit Ctrl         |
              |                                | Monitoring/Central/Global   |
         9 Webhooks                            |   Translator Ctrls          |
         validate all                          +-----------------------------+
         mutations                                       ^
                                                    2 Webhooks
```

### Key Design Patterns
- **Dual-client controllers**: Fragment controllers use `RootClient` (cross-cluster cache)
  and `FragmentClient` (local cluster)
- **Generic reconcilers**: `Reconciler[C sdk.Client]` parameterized on client type
- **Deferred conflict handling**: `defer common.RequeueOnConflictWithRandomTime()` converts
  K8s API conflicts into randomized requeues
- **Bidirectional status sync**: Root sets `Provisioned` conditions; Fragment syncs DNS
  names and runtime status back

### Custom Resource Definitions

#### Root API Group: `root.laas.ionos.com/v1alpha1`

| Kind | Short Name | Scope | Description |
|------|-----------|-------|-------------|
| Contract | `lc` | Namespaced | Customer billing contract with feature flags (`logs`, `monitoring`, `centralLogging`, `centralMonitoring`), proxy rate/bandwidth limits, quota overrides, Mimir runtime config overrides |
| Pipeline | `lp` | Namespaced | Logging pipeline: log sources (K8s/Docker/Systemd), Loki destinations, retention periods, protocol (TCP/HTTP), proxy limits. Tracks K8s cluster assignment and DNS names |
| MonitoringPipeline | `mp` | Namespaced | Monitoring pipeline (Mimir-based) for a contract |
| DataCenter | `ldc` | Cluster | Maps IONOS Cloud VDC UUID to a location (e.g. `de/fra`). Verified against DCM API |
| KubernetesCluster | `lkc` | Cluster | Maps a managed K8s cluster UUID within a DataCenter |
| CentralLogging | `cl` | Namespaced | Per-contract centralized logging aggregation. Requires `centralLogging` feature flag |
| CentralMonitoring | `cm` | Namespaced | Per-contract centralized monitoring aggregation |
| GlobalCentralMonitoring | `gcm` | Namespaced | Cross-contract, product-level centralized monitoring |

#### Fragment API Group: `fragment.laas.ionos.com/v1alpha1`

| Kind | Short Name | Scope | Description |
|------|-----------|-------|-------------|
| FluentBit | `flb` | Namespaced | FluentBit StatefulSet in customer namespace. Defines replicas, ports, image pull secrets, buffer volumes, quota overrides |
| FluentBitConfig | `flbc` | Namespaced | FluentBit pipeline config: input sources (Forward/HTTP), Loki output matches, flush intervals. Rendered into ConfigMap |
| LoggingIngress | `li` | Namespaced | Kong-based ingress for logging endpoints. Maps protocol (HTTP/TCP) to ports. Creates Ingress, Kong plugins (rate limiting, key-auth), TLS certificates |
| CustomerGrafana | `cgraf` | Namespaced | Per-customer Grafana org management. Creates orgs, service accounts, configures Loki/Mimir datasources |
| MimirRuntimeConfig | — | Namespaced | Per-tenant Mimir runtime config overrides (ingestion rate limits, cardinality, compactor). Aggregated into shared ConfigMap |

### Controller Count
- Root: 8 controllers, 9 webhooks
- Fragment: 13 controllers, 2 webhooks
- Root Prometheus metrics: 5 collectors
- Fragment Prometheus metrics: 6 collectors

### Technology Stack

| Category | Technology |
|----------|------------|
| Language | Go 1.25.7 |
| K8s Framework | `sigs.k8s.io/controller-runtime` |
| Testing | Ginkgo v2 + Gomega + envtest (real K8s API + etcd) |
| Deployment | Helm v2 charts (root: 17 templates + 8 CRDs; fragment: 21 templates + 5 CRDs) |
| CI/CD | GitHub Actions |
| Code Quality | golangci-lint (~30 linters) + SonarCloud |

### Project Structure
```
laas-operators/
├── cmd/
│   ├── root/           # Root operator entrypoint
│   └── fragment/       # Fragment operator entrypoint
├── pkg/
│   ├── apis/
│   │   ├── root.laas.ionos.com/v1alpha1/    # 8 root CRD types
│   │   └── fragment.laas.ionos.com/v1alpha1/ # 5 fragment CRD types
│   ├── controllers/
│   │   ├── root/       # 8 root controllers + 9 webhooks
│   │   └── fragment/   # 13 fragment controllers + 2 webhooks
│   ├── metrics/        # Prometheus collectors (5 root, 6 fragment)
│   ├── sdk/            # IONOS Cloud SDK wrapper (DC/K8s API)
│   ├── common/         # Shared helpers, status constants
│   ├── k8sutil/        # K8s client wrapper with metrics, condition helpers
│   ├── fluentbitconfig/ # FluentBit YAML config generation library
│   ├── predicates/     # Controller watch predicates
│   └── tests/          # Full integration tests (both operators together)
├── charts/
│   ├── root/           # Root operator Helm chart
│   └── fragment/       # Fragment operator Helm chart
└── config/
    ├── samples/        # Sample CR manifests for all CRD types
    └── kind/           # Kind cluster configs for local development
```

### Build & Test
```bash
make ready          # Full pre-commit check
make build          # Build both operator binaries (linux/amd64)
make test           # Tests with envtest + coverage
make manifests      # Generate CRDs, RBAC, webhook manifests (controller-gen)
make generate       # Generate DeepCopy methods for API types
make generate-mocks # Generate mocks via mockery

# Local development (Kind clusters)
./recreate_everything.sh  # Full local environment (root + fragment)
./recreate_root.sh        # Root cluster only
./recreate_fragment.sh    # Fragment cluster only
```

---

## go-paaskit

### Purpose
Common foundation library for all IONOS PaaS Go services. Avoids reinventing the wheel
by providing correctly configured wrappers around open-source libraries.

### Module Groups

#### Observability (`observability/`)
| Package | Description |
|---------|-------------|
| `paasmetric` | Prometheus metrics with OpenTracing support |
| `paaslog` | Structured logging via `go-logr/logr` |
| `paas_error` | Error and panic handling |
| `paas_alert` | Team alerting for invariant violations |
| `paas_sla` | SLA compliance measurement |
| `paashealth` | Health reporting and readiness endpoints |
| `paas_tracing` | OpenTracing/Tempo integration |

#### Infrastructure (`infrastructure/`)
| Package | Description |
|---------|-------------|
| `paas_redis` | Redis access layer (logging, metrics, circuit breaker) |
| `paasql` | PostgreSQL access layer (logging, metrics, circuit breaker) |
| `paas_ionos` | IONOS Cloud API client (configured with middleware) |
| `paas_ionos_dcm` | IONOS Cloud DCM API client |
| `paas_k8s` | Kubernetes client |

#### Services (`service/`)
| Package | Description |
|---------|-------------|
| `auth` | JWT handling and authorization |
| `billing` | Billing event publishing (legacy; use `cloudevent` instead) |
| `cloudevent` | CloudEvent bus for lifecycle, billing, audit log, bulk export |
| `features` / `feature` | Feature flag management (K8s ConfigMap + local file backends) |
| `contract` | Contract and permission management |
| `messaging` | Common messaging system abstraction |
| `s3` | IONOS S3 key and bucket management |
| `paas_quota` | Customer quota management |
| `activity` | Activity log management |
| `onedns` | DNS zones and records management |
| `bulkexport` | Common bulk export client and server |
| `logging` | Logging events management |
| `monitoring` | Monitoring events management |
| `access` | URN-based access checking |

#### API (`api/`)
| Package | Description |
|---------|-------------|
| `paashttp` | HTTP client/server with middleware (logging, health, rate limiting, metrics) |
| `paastype` | Error and object encoding (time, decimal, etc.) |

#### Platform (`pkg/`)
| Package | Description |
|---------|-------------|
| `appinfo` | Application info |
| `paas_breaker` | Circuit breaker with metrics and health feedback |
| `paas_termlog` | Kubernetes termination log |
| `supervisor` | Background goroutine supervisor with restart-on-failure |
| `protobuf` | IONOS Cloud Protobuf definitions |
| `urn` | URN implementation for IONOS resource addressing |

### Design Principles
- Make hard things easy: correctly configured third-party libraries
- 12-factor: ENV-based configuration (but allow direct config too)
- No `init()` unless unavoidable
- Minimal goroutine use
- `context.Context` for loose coupling, not as primary API
- `paas_` prefix to identify paaskit code in projects

---

## event-gateway

### Purpose
gRPC CloudEvent gateway and processor built on Kafka. Controls publishing and subscribing
to events across the IONOS PaaS ecosystem. LaaS uses it for billing, logging, monitoring,
and lifecycle events.

### Components
| Component | Description |
|-----------|-------------|
| `event-gateway` | gRPC server implementing the EventGateway Protobuf service spec. Publishes/subscribes CloudEvents to Kafka |
| `event-processor` | Kafka consumer for processing events (LaaS billing, lifecycle, etc.) |

### Kafka Topic Conventions
Topic names follow: `prefix.namespace.collection`

| Topic | Purpose |
|-------|---------|
| `ionos.cloud.events.billing` | Billing events |
| `ionos.cloud.events.events` | General events |
| `ionos.cloud.events.lifecycle` | Contract lifecycle events |
| `ionos.cloud.events.activity` | Activity/audit log events |
| `ionos.cloud.events.bulkexport` | Bulk export events |
| `ionos.cloud.events.deadletter` | Dead letter queue |
| `ionos.cloud.events.logging` | Logging events |
| `ionos.cloud.events.monitoring` | Monitoring events |

### Kafka Configuration
- **Replication factor**: 3
- **Partitions**: 3
- **Partitioning**: murmur2 hash on `contractID` → all events for a contract go to same partition
- **Write quorum**: 2 in-sync replicas minimum
- **Batch writes**: 100 messages per batch, 1s timeout
- **Group ID convention**: `ionos.cloud.group.event-processor` or URN-based

### Prometheus Metrics
- `ionos_event_gateway_written_total` — written messages
- `ionos_event_gateway_committed_total` — acked messages after reading
- `ionos_event_gateway_fetched_total` — read messages

### Deployment
Deployed via `paas-sre-argocd`. Release via GitHub Release tag (`vX.Y.Z`). GitHub Actions
builds Docker images and Helm charts → pushed to Harbor.

---

## laas-admin-api

### Purpose
Internal admin REST API for managing LaaS contracts. Used by IONOS admins (not customers).
Accessible via Regional Product DNS.

### Technology Stack
- Go + chi router
- `go-paaskit` for auth, metrics, observability

### Dependencies
- `laas-operators` — LaaS Root Operators (for reading/writing Contracts)
- `restdcm.paas-tunnel.ionos.cloud` — DCM API
- Keycloak — Authentication

---

## laas-deployment

### Purpose
ArgoCD-based deployment repository for all LaaS applications and components.

### Tools
- ArgoCD (GitOps)
- Helm
- Vault (secret management)

---

## laas-infra

### Purpose
Helm charts for LaaS infrastructure. Per-cluster infrastructure manifests for components
not tied to application code (REST API, Operators). Changes in charts trigger GitHub
Actions that push updates to `laas-deployment`.

---

## laas-e2e

### Purpose
End-to-end test suite verifying full LaaS/MaaS platform integrity.

### Test Categories
- Logging and Monitoring Pipeline Tests
- Consistency Tests (data integrity)
- RBAC Tests (role-based access)
- Central Logging and Monitoring Tests

### Supported Environments
- `dev`, `stage`, `prod`

### Production Locations
| Location Code | City |
|--------------|------|
| `de-txl` | Berlin, Germany |
| `de-fra` | Frankfurt, Germany |
| `es-vit` | Vitoria, Spain |
| `gb-lhr` | London, UK |
| `gb-bhx` | Birmingham, UK |
| `fr-par` | Paris, France |
| `us-mci` | Kansas City, USA |

---

## Domain Concepts

### Pipeline Lifecycle
1. Customer creates a Pipeline via REST API (POST `/pipelines`)
2. `laas-rest-api` validates, creates a `Pipeline` CRD on the root cluster
3. Root operator reconciles: verifies DataCenter/K8sCluster, assigns fragment cluster
4. Fragment operator watches root cluster via cross-cluster kubeconfig
5. Fragment creates: CustomerNamespace → FluentBit → FluentBitConfig → LoggingIngress → CustomerGrafana
6. FluentBit deployed as StatefulSet; Kong ingress exposes TCP/HTTP log ingestion endpoints
7. DNS names propagated back to root Pipeline CRD status
8. REST API reads DNS names from Pipeline CRD status for customer response

### Contract Lifecycle
- Contract CRD tracks feature flags: `logs`, `monitoring`, `centralLogging`, `centralMonitoring`
- DCM lifecycle events (`ENABLED`, `DISABLED`, `CEASED`) processed by `event-processor`
- On `CEASED`: all customer resources cleaned up

### Central Logging/Monitoring
- Requires explicit feature flag on Contract
- CentralLogging/CentralMonitoring CRDs aggregate logs/metrics across a customer's pipelines
- Enabled/disabled via Kafka events (`logging.ENABLED`, `monitoring.ENABLED`)
- GlobalCentralMonitoring: cross-contract, product-level (identified by product label)

### Authentication Key Rotation
- Each pipeline has an authentication key for log ingestion
- `POST /pipelines/{pipelineID}/key` regenerates the key
- Kong key-auth plugin enforces the key on the LoggingIngress

### Billing Events
- Emitted to Kafka on: pipeline creation/deletion, central logging/monitoring enable/disable
- PostgreSQL used as store-and-forward: events queued if Kafka unavailable, replayed on recovery
- `event-gateway` client wraps this with retry logic (`tolerant_egw.go`)

### Feature Flags
- Managed via `go-paaskit/service/feature`
- Backends: Kubernetes ConfigMap or local file storage
- Used to gate: centralLogging, centralMonitoring, access checking, billing

### Resource Tiers
- Pipelines have resource tiers (default: `s`)
- Configurable via `ALLOWED_PIPELINE_RESOURCE_TIERS` env var
- Controls FluentBit resource quotas (CPU/memory/buffer volume)

### Geographic Routing
- Each deployment is scoped to a geo: `IONOS_GEO_COUNTRY` + `IONOS_GEO_REGION`
- Location extracted from request host header by middleware
- Affects DataCenter/KubernetesCluster assignment

### Log Retention
- Allowed retention periods: 0, 7, 14, 30 days (configurable via `ACCEPTED_RETENTIONS`)
- 0 = unlimited retention (uses "unloki" — see `laas-unloki-data-dumper`)
- Retention set on Pipeline CRD → fragment operator configures Loki override

---

## Infrastructure Components

| Component | Role in LaaS |
|-----------|-------------|
| **FluentBit** | Per-customer log collector/forwarder deployed as StatefulSet |
| **Loki** | Log storage backend with per-tenant retention overrides |
| **Mimir** | Metrics storage backend with per-tenant rate limit overrides |
| **Grafana** | Customer dashboards; per-customer org with Loki + Mimir datasources |
| **Kong** | API gateway for log ingestion endpoints; rate limiting + key-auth plugins |
| **cert-manager** | TLS certificate management for Kong ingresses |
| **Kafka** | CloudEvent bus (billing, lifecycle, logging, monitoring events) |
| **PostgreSQL** | Billing event store + store-and-forward queue |
| **Argo CD** | GitOps deployment operator |
| **Harbor** | Container image + Helm OCI chart registry |
| **Vault** | Secret management |

---

## Observability

### Metrics Endpoints
- REST API: `:8083` (Prometheus)
- Root operator: standard controller-runtime metrics
- Fragment operator: standard controller-runtime metrics + 6 custom collectors

### Grafana Dashboards
- REST API Grafana dashboard: `docs/dashboard/` in `laas-rest-api`
- Operators Grafana dashboards: `docs/dashboard/` in `laas-operators`

### Loki Log Queries
- Event Gateway logs: `{job="kafka/event-gateway"}`
- Event Processor logs: `{job="kafka/event-processor"}`

### Key Metrics
- `ionos_event_gateway_written_total`
- `ionos_event_gateway_committed_total`
- `ionos_event_gateway_fetched_total`
- Standard Go runtime metrics
- Standard gRPC metrics
- controller-runtime reconciliation metrics

---

## Common Development Tasks

### Local Development Setup
```bash
# laas-operators: full local env with Kind
./recreate_everything.sh

# laas-rest-api: full pre-commit pipeline
make ready

# Both repos use same pattern:
make dep       # go mod tidy + verify + download
make generate  # generate code/manifests
make build     # compile binaries
make test      # run tests with coverage
make lint      # golangci-lint
```

### Adding a New API Endpoint (laas-rest-api)
1. Add handler in `pkg/loggingpipeline/` or `pkg/monitoringpipeline/`
2. Add service method + K8s CRD interaction
3. Register route in `cmd/api/api.go`
4. Add input validation with `go-playground/validator/v10`
5. Update `docs/openapi/logging.yaml` or `monitoring.yaml`
6. Add tests with `testify` + `mockery` mocks

### Adding a New CRD (laas-operators)
1. Define type in `pkg/apis/root.laas.ionos.com/v1alpha1/` or `fragment/`
2. Run `make generate` (DeepCopy methods)
3. Run `make manifests` (CRD YAML, RBAC, webhooks)
4. Implement controller in `pkg/controllers/root/` or `pkg/controllers/fragment/`
5. Add webhook in same directory if validation needed
6. Add envtest-based integration tests with Ginkgo

### Releasing
- **laas-rest-api**: Push a git tag → GitHub Actions builds Docker image + Helm chart → pushes to Harbor → Argo CD deploys
- **laas-operators**: Same pattern
- **event-gateway**: Create GitHub Release → GitHub Actions builds → Platform Framework deploys

---

## Key External Integrations

| Service | URL Pattern | Purpose |
|---------|------------|---------|
| Auth Service | `auth-paas-tunnel.paasfe.stg.ionos.cloud` | JWT validation + JWKS |
| DCM API | `restdcm.paas-tunnel.ionos.cloud` | DataCenter/contract validation |
| IONOS Cloud API | `api.ionos.com` | VDC/K8s cluster verification |
| Keycloak | `keycloak.infra.cluster.ionos.com` | Admin API authentication |
| Harbor | `harbor.infra.cluster.ionos.com` | Docker images + Helm charts |
| SonarCloud | `sonarcloud.io` | Code coverage + quality gates |

---

## Additional Repositories (Supporting)

| Repo | Purpose |
|------|---------|
| `laas-billing-collector` | Billing collector for S3 usage |
| `laas-fluentbit-log-analytics` | CronJob: scrapes FluentBit usage metrics → sends to customer's Mimir |
| `laas-grafana-auth-proxy` | Grafana authentication proxy |
| `laas-unloki-data-dumper` | Dumps client logs from unlimited-retention Loki to customer S3 bucket |
| `laas-s3-key-refresher` | S3 key rotation CronJob |
| `laas-jwt-key-cacher` | CronJob: caches JWT keys |
| `laas-monitoring-bulk-export` | Bulk export for Mimir monitoring data |
| `laas-log-bulk-export` | Bulk export for Loki log data |
| `laas-auditor` | Pushes LaaS K8s audit logs into Loki |
| `laas-benchmark` | Benchmark tools and scenarios |
| `laas-tools` | Team tools for testing and demos |
| `laas-c5` | BIS C5 compliance for observability products |
| `s3-aes-proxy` | S3 proxy with AES encryption/decryption for objects |
