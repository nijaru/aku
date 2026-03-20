# Benchmark Handoff

## Status

Benchmark infrastructure fully operational on Fedora (nick@fedora, Tailscale). Grafana dashboard shows live RPS and p50/p99 latency for all 7 frameworks. Container CPU/memory metrics not available (cAdvisor incompatible with rootless podman — requires rootful Docker).

## Environment

- **Host:** Fedora desktop, i9-13900KF, 32GB RAM, RTX 4090
- **Runtime:** podman-compose 1.5.0, podman 5.8.1 (rootless)
- **Tailscale:** `nick@fedora`, IP `100.93.39.25`
- **Bench dir:** `/home/nick/github/nijaru/aku/.bench/`

## Access

| Service | URL | Notes |
|---|---|---|
| Grafana | http://100.93.39.25:3000 | admin / bench |
| Prometheus | http://100.93.39.25:9090 | 1s scrape interval |
| Dashboard | http://100.93.39.25:3000/d/bench-overview | Auto-provisioned |
| Datasource UID | `PBFA97CFB590B2093` | |

## Services

| Container | Port | Image | Status |
|---|---|---|---|
| aku | 9001→8080 | localhost/bench_aku | up |
| gin | 9002→8080 | localhost/bench_gin | up |
| fiber | 9003→8080 | localhost/bench_fiber | up |
| fastapi | 9004→8080 | localhost/bench_fastapi | up |
| hono | 9005→8080 | localhost/bench_hono | up |
| express | 9006→8080 | localhost/bench_express | up |
| actix | 9007→8080 | localhost/bench_actix | up |
| prometheus | 9090 | Custom (baked config) | up |
| grafana | 3000 | Custom (baked provisioning) | up |

All containers expose `/metrics` in Prometheus format. Prometheus scrapes each at 1s interval. Grafana auto-provisions datasource + dashboard from baked-in config (no volume mounts — rootless podman UID mapping breaks them).

## Benchmark Results — Concurrent (10s per endpoint, all 7 frameworks loaded simultaneously)

Unlimited rate, 4 vegeta workers, 100 connections.

| Endpoint | Aku | Gin | Fiber | FastAPI | Hono | Express | Actix |
|---|---|---|---|---|---|---|---|
| GET / | 108,944 | 14,782 | 14,840 | 14,890 | 23,164 | 14,701 | 168,739 |
| GET /items/{id} | 78,228 | 14,030 | 14,211 | 14,135 | 23,546 | 19,105 | 215,889 |
| GET /users/{uid}/posts/{pid} | 76,400 | 11,646 | 11,650 | 11,684 | 20,784 | 19,098 | 214,927 |
| POST /items | 62,591 | 13,848 | 14,247 | 14,164 | 23,452 | 17,897 | 198,617 |
| **TOTAL** | **326,163** | 54,306 | 54,948 | 54,873 | 90,946 | 70,801 | **798,172** |

**p99 latency (concurrent):**

| Endpoint | Aku | Gin | Fiber | FastAPI | Hono | Express | Actix |
|---|---|---|---|---|---|---|---|
| GET / | 12.6ms | 69.4ms | 74.5ms | 75.7ms | 46.6ms | 71.5ms | 7.3ms |
| GET /items/{id} | 17.8ms | 66.2ms | 68.0ms | 69.2ms | 40.7ms | 53.9ms | 4.0ms |
| GET /users/{uid}/posts/{pid} | 18.2ms | 87.6ms | 95.1ms | 95.9ms | 50.4ms | 58.4ms | 4.0ms |
| POST /items | 21.1ms | 66.6ms | 85.5ms | 80.7ms | 40.2ms | 49.5ms | 4.2ms |

**Relative to Aku:** Gin 6.0x, Fiber 5.9x, FastAPI 5.9x, Hono 3.5x, Express 4.6x, Actix 0.4x (Actix 2.4x faster).

## Benchmark Results — Solo (5s per endpoint, one framework at a time)

| Framework | RPS (GET /) | p99 | CPU% | Memory |
|---|---|---|---|---|
| aku | 168,855 | 4.5ms | 10.6% | 27 MB |
| gin | 80,619 | 15.6ms | 4.5% | 29 MB |
| fiber | 14,397 | 73.8ms | 0.15% | 12 MB |
| fastapi | 14,393 | 70.1ms | 0.28% | 43 MB |
| hono | 14,320 | 69.7ms | 0.33% | 25 MB |
| express | 14,338 | 69.3ms | 0.60% | 33 MB |
| actix | 14,325 | 65.0ms | 0.34% | 27 MB |

Note: Solo results show different dynamics — all frameworks share the i9's cores, so running alone each can burst higher. The concurrent test is more realistic.

## Idle Memory Footprint

| Framework | Memory | vs Aku |
|---|---|---|
| **Aku** | **13.7 MB** | 1.0x |
| Fiber | 12.7 MB | 0.9x |
| Gin | 18.5 MB | 1.3x |
| Express | 23.2 MB | 1.7x |
| Hono | 25.2 MB | 1.8x |
| Actix | 27.1 MB | 2.0x |
| FastAPI | 42.0 MB | 3.1x |

## Key Gotchas

1. **Rootless podman volume mounts don't work.** UID mapping breaks Prometheus/Grafana/cAdvisor volume mounts. All configs must be baked into Docker images via `COPY` in Dockerfile. Never use `volumes:` in compose for config files — use custom images instead.

2. **Short image names fail without TTY.** Podman prompts for registry selection but can't over SSH. All images need full registry path: `docker.io/grafana/grafana:latest`, `docker.io/prom/prometheus:v3.3.0`, `gcr.io/cadvisor/cadvisor:latest`.

3. **Vegeta stdin over SSH.** `vegeta attack` reads from stdin when piped, causing `encode: can't detect encoding of "stdin"`. Fix: write targets to file, use `-targets=/tmp/file.txt`. Also `-rate=0` (unlimited) requires `-max-workers=N`.

4. **Aku /metrics intercepted by error handler.** `http.Handle("/metrics", promhttp.Handler())` registers on default mux, but `app.ServeHTTP` intercepts all requests and returns JSON 404 for unknown paths. Fixed with `metricsWrapper` that checks `/metrics` before delegating to app. This could become an Aku feature (`app.MetricsHandler()`).

5. **Fiber /metrics on wrong port.** Originally served metrics on port 9090 via goroutine — not reachable from compose network (internal only). Fixed by using `fiber/adaptor.HTTPHandler(promhttp.Handler())` on main app at 8080.

6. **cAdvisor won't run on rootless podman.** Error: `open .containerenv: No such file or directory`. This is a known incompatibility — cAdvisor expects Docker's cgroup layout. Rootful Docker required for container CPU/memory metrics in Prometheus/Grafana.

## How to Run

```bash
# SSH to Fedora
ssh nick@fedora

# Start everything
cd /home/nick/github/nijaru/aku/.bench
podman compose up -d

# Verify all targets healthy
curl -sf http://127.0.0.1:9090/api/v1/targets | jq '.data.activeTargets[] | {job, health}'

# Run benchmark (full concurrent test, ~5 min)
# Option A: inline script via SSH (avoids vegeta stdin issues)
ssh nick@fedora "bash -s" < /path/to/bench_script.sh 2>/dev/null

# Option B: manual vegeta
echo 'GET http://127.0.0.1:9001/' > /tmp/t.txt
vegeta attack -targets=/tmp/t.txt -rate=0 -max-workers=200 -duration=10s -workers=4 -connections=100 | vegeta report

# Container stats during load
podman stats --no-stream

# Tear down
podman compose down
```

## Files

```
.bench/
  docker-compose.yml              # 9 services (7 frameworks + prometheus + grafana)
  bench.sh                        # wrk-based (Mac only, not used for container bench)
  docker/
    aku/                          # Go/Aku + metricsWrapper for /metrics
      Dockerfile                  # copies repo root as /aku, uses replace directive
      go.mod                      # aku-bench with replace => /aku
      main.go
    gin/                          # Go/Gin + promhttp
      Dockerfile                  # copies gin/ dir only
      go.mod / main.go
    fiber/                        # Go/Fiber + adaptor.HTTPHandler for /metrics
      Dockerfile / go.mod / main.go
    fastapi/                      # Python/FastAPI + prometheus-client
      Dockerfile / main.py
    hono/                         # Bun/Hono + prom-client (not bun-prometheus)
      Dockerfile / index.ts
    express/                      # Node/Express + prom-client
      Dockerfile / index.js
    actix/                        # Rust/Actix + actix-web-prom
      Dockerfile / Cargo.toml / src/main.rs
    prometheus/
      Dockerfile                  # bakes prometheus.yml into image
      prometheus.yml              # 1s scrape, 7 framework targets
    grafana/
      Dockerfile                  # bakes provisioning into image
      provisioning/
        datasources/prometheus.yml
        dashboards/bench.json     # RPS + p50/p99 panels
        dashboards/provider.yml
    stats-exporter.sh             # unused — podman stats script (nc-based, flaky)
```

## Next Steps

- [ ] Rewrite `docker/bench.sh` to use vegeta (currently uses wrk, actual benchmarks ran via inline SSH heredoc)
- [ ] Container metrics: switch to rootful Docker on Fedora, then add cAdvisor service
- [ ] Add Go stdlib `net/http` baseline to comparison
- [ ] Consider `app.MetricsHandler()` as built-in Aku feature to eliminate metricsWrapper boilerplate
- [ ] Run longer benchmarks (30s+) for more stable p99 numbers
- [ ] Consider adding Bun/Elysia or other JS frameworks for broader comparison
