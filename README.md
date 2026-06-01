<div align="center">

# goboxd

**A Go HTTP service for executing untrusted code in isolated sandboxes.**

[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.23-00ADD8.svg?logo=go&logoColor=white)](https://go.dev)
[![Docker](https://img.shields.io/badge/Docker-Required-2496ED.svg?logo=docker&logoColor=white)](https://www.docker.com)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](https://github.com/thesouldev/goboxd/pulls)

</div>

---

## Overview

goboxd is an HTTP service written in Go that compiles and runs untrusted code inside isolated sandboxes and returns the result. Supply test cases to assert behaviour against expected output, or provide a custom evaluator script for flexible grading. It is built for safe execution of code across many languages, with strict resource isolation and a YAML-driven language registry.

## Features

- YAML-driven language registry — add a language without touching Go code
- Process isolation via Linux namespaces (nsjail) and cgroup v2 memory enforcement
- Per-request limits for wall-clock time, virtual memory, and process count
- Evaluator script support for custom output grading (e.g. checker scripts)
- Stale workspace cleanup via a background butler goroutine
- Liveness (`/healthz`) and readiness (`/readyz`) probes
- Build info and runtime stats exposed at `/info`

## Quick start

```sh
git clone https://github.com/thesouldev/goboxd.git
cd goboxd
make build        # build Docker image
make run          # start the service on :8080
```

See [docs/getting-started.md](docs/getting-started.md) for the full walkthrough including example API calls.

## API

| Method | Path      | Description                                 |
|--------|-----------|---------------------------------------------|
| `POST` | `/run`    | Compile and run code, return test results   |
| `GET`  | `/healthz`| Liveness probe — returns `{"status":"ok"}`  |
| `GET`  | `/readyz` | Readiness probe — runs version checks per language |
| `GET`  | `/info`   | Build info, language list, and runtime stats |

### Minimal request

```json
{
  "language": "py3",
  "source": "print('hello')",
  "tests": [
    { "stdin": "", "expected_stdout": "hello\n" }
  ]
}
```

```sh
curl -s -X POST http://localhost:8080/run \
  -H "Content-Type: application/json" \
  -d '{"language":"py3","source":"print(\"hello\")","tests":[{"stdin":"","expected_stdout":"hello\n"}]}'
```

### Response

```json
{
  "status": "success",
  "build": { "status": "NOT_RUN" },
  "test": [
    { "status": "accepted", "stdout": "hello\n", "stderr": "", "duration_ms": 42, "memory_peak_kb": 0 }
  ]
}
```

Test status values: `accepted`, `wrong_output`, `time_exceeded`, `memory_exceeded`, `errored`.

## Make targets

| Target            | Description                                  |
|-------------------|----------------------------------------------|
| `make build`      | Build the Docker image                       |
| `make run`        | Start the service on `:8080`                 |
| `make test`       | Run unit tests inside the tools container    |
| `make run-integration` | Run integration tests against a live server |
| `make lint`       | Run `golangci-lint`                          |

## Project structure

```
.
├── cmd/goboxd/        binary entry point
├── internal/          HTTP handlers, run logic, types, cleanup
├── docs/              architecture and getting-started guides
├── tests/             integration tests and YAML test cases
├── scripts/           language install scripts run at image build time
└── config.yaml        language registry
```

## Supported languages

| ID           | Language      |
|--------------|---------------|
| `c`          | C (GCC)       |
| `cpp`        | C++ (G++)     |
| `py`         | Python 2.7    |
| `py3`        | Python 3      |
| `bash`       | Bash          |

Additional languages can be added by editing `config.yaml` and running `scripts/lang_install/<lang>.sh` in the Dockerfile. See [docs/architecture.md](docs/architecture.md#adding-a-language).

## Docs

- [Getting started](docs/getting-started.md) — installation, example requests, troubleshooting
- [Architecture](docs/architecture.md) — design, request lifecycle, sandboxing model

## Contributing

Contributions are welcome. Open an issue to discuss substantial changes before sending a pull request.

## License

This project is distributed under the GNU General Public License v3.0. See [LICENSE](LICENSE) for the full text.
