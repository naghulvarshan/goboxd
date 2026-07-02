# Architecture

## Overview

goboxd is a stateless HTTP service. Each `POST /run` request creates an isolated workspace on disk, writes the submitted source file, optionally compiles it, runs it once per test case inside an nsjail sandbox, then streams results back to the caller and deletes the workspace.

```
caller
  │
  ▼
HTTP handler (/run)
  │  validate request, look up language in registry
  ▼
Run()  [internal/program.go]
  ├─ generateWorkSpace()   create ~/nsjail_programs/nsip_<id>/
  ├─ addSource()           write source file to workspace
  ├─ (optional) evaluator path ──► runEvaluationScript()
  ├─ createTestWS()        write per-test input files
  ├─ (if compiled lang) compileCode()   run javac/gcc inside nsjail
  └─ runCode()             run binary/interpreter inside nsjail, once per test
        │
        ▼
     []TestOutput  ──► JSON response
```

## Components

### `cmd/goboxd/main.go`

Entry point. Initialises structured logging, parses `--port`, loads `config.yaml`, sets up the cgroup v2 base path at `/sys/fs/cgroup/goboxd`, creates `~/nsjail_programs/`, then calls `server.Serve()`.

### `internal/server.go`

Registers HTTP routes and holds global state:

- `langMap` — map of language ID → `LanguageSettings`, built once at startup from the config
- `activeRequests` — atomic counter for in-flight jobs
- `jobsTotal` / `jobsFailedWithIntSvrErr` — lifetime counters exposed at `/info`

### `internal/program.go`

Core execution logic.

**`generateWorkSpace()`** — creates a uniquely named directory under `~/nsjail_programs/` with `proc/` and `tmp/` subdirectories required by nsjail and the JVM respectively.

**`compileCode()`** — builds an nsjail command that runs the compiler (e.g. `gcc`, `javac`). Resource limits from the request's `build` section override the language defaults. Compilation stdout/stderr is captured; on failure the response status is set to `compilation_error` and the nsjail log is included in `build.stderr`.

**`runCode()`** — for each test case, builds an nsjail command that runs the binary or interpreter. stdin is piped from the test's input file. cgroup v2 memory tracking is created per-run when available. Exit status and stderr are parsed to detect `time_exceeded`, `memory_exceeded`, or `errored`.

**`runEvaluationScript()`** — alternative execution path when `evaluation_script` is present in the request. Runs a single nsjail command of the form `<lang> evaluator.script <source_file>`, captures stdout as raw JSON, and returns it in `evaluation_result_json`. Test cases and compilation are skipped.

### `internal/types.go`

Defines all request and response types, validation constants, and `UnmarshallRequest()` which validates the incoming JSON body (required fields, filename safety, source size, test case count).

### `internal/config.go`

`LoadConfig()` reads and YAML-unmarshals the language registry file. Called once at startup.

### `internal/butler.go`

`junkCleaner()` runs as a background goroutine, ticking every 30 seconds. It deletes any workspace directory under `~/nsjail_programs/` whose modification time is older than one minute, recovering disk space from any request that did not clean up normally.

### `internal/errors.go`

Typed errors (`InvalidInputError`, `InternalServerError`) with HTTP status mappings used by `errorResp()`.

## Sandboxing model

Every execution runs inside [nsjail](https://github.com/google/nsjail) in `STANDALONE_ONCE` mode with Linux namespace isolation:

| Namespace  | Isolated |
|------------|----------|
| Network    | Yes — no outbound access |
| PID        | Yes |
| Mount      | Yes — chroot to workspace; host directories bind-mounted read-only |
| User       | Yes |
| UTS / IPC  | Yes |

### Resource limits

Three mechanisms work together:

1. **`--rlimit_as`** (nsjail) — limits virtual address space. Derived from `memory_kb` in the request or language config, converted from KB to MiB (`memory_kb / 1024`).
2. **`--time_limit`** (nsjail) — wall-clock seconds before SIGKILL.
3. **`--rlimit_nproc`** (nsjail) — maximum threads/processes for the user inside the jail.
4. **cgroup v2 `memory.max`** — RSS cap in bytes, applied per-run when cgroupv2 is available. This catches languages (e.g. Python) that bypass virtual-memory limits.

### Exit status interpretation

After each `runCode` invocation the nsjail stderr is scanned:

| Pattern in stderr | Reported status |
|-------------------|-----------------|
| `run time >= time limit` | `time_exceeded` |
| `MemoryError` or `terminated with signal: 9` (when memory limit set) | `memory_exceeded` |
| Non-zero exit, no match | `errored` |
| Zero exit, wrong output | `wrong_output` |
| Zero exit, correct output | `accepted` |

## Language registry

Languages are declared in `config.yaml`. Each entry maps to a `LanguageSettings` struct:

```yaml
- id: py3                       # identifier sent in API requests
  name: "Python 3"
  source: "test.py"             # default source filename
  run:
    cmd: "/usr/bin/python3"
    args: ["{{source}}"]        # {{source}} and {{artifact}} are substituted at runtime
    limits:
      wall_time_s: 9
      max_processes: 100
      memory_kb: 102400
  version_cmd: "/usr/bin/python3 --version"
```

Compiled languages add a `build` section with `cmd`, `args` (supporting `{{flags}}`, `{{source}}`, `{{artifact}}`), and `limits`. The `artifact` key (set to `"TAKE_FROM_REQUEST"`) signals that the binary name comes from `artifact_filename` in the request rather than the config.

### Adding a language

1. Add a `lang_install/<lang>.sh` script that installs the toolchain inside the runtime container.
2. Reference it from `scripts/install.sh`.
3. Add an entry to `config.yaml` with `id`, `source`, `run` (and optionally `build`).
4. Rebuild: `make build`.

No Go code changes are needed.

## Data flow diagram

```
POST /run
  body: { language, source, tests[], build?, run?, evaluation_script? }
         │
         ▼
   UnmarshallRequest()
   ─ validate JSON
   ─ check source size ≤ 256 KiB
   ─ check tests ≤ 50
   ─ sanitise source_filename
         │
         ▼
   langMap lookup by language ID
         │
         ▼
   generateWorkSpace()          ~/nsjail_programs/nsip_<5-char-id>/
         │                        ├── proc/
         │                        ├── tmp/
         │                        └── <source_filename>
         │
    ┌────┴──────────────────────────────────────┐
    │ evaluation_script present?                 │
    │  YES ──► runEvaluationScript()             │
    │          └── return { evaluation_result_json }
    │                                            │
    │  NO                                        │
    │   ├─ createTestWS()   test_0/input ...     │
    │   ├─ compileCode()?   (compiled langs)     │
    │   └─ runCode()        per test case        │
    │        ├─ nsjail exec                      │
    │        ├─ cgroup memory tracking           │
    │        └─ status classification            │
    └────────────────────────────────────────────┘
         │
         ▼
   defer os.RemoveAll(workspace)
         │
         ▼
   JSON response
```

## Deployment

The service is distributed as a single Docker image built from a multi-stage Dockerfile:

| Stage          | Base                    | Purpose                              |
|----------------|-------------------------|--------------------------------------|
| `nsjail-builder` | `debian:bookworm-slim` | Compile nsjail from source           |
| `builder`        | `golang:1.23-bookworm` | Compile goboxd binary                |
| `runtime`        | `debian:bookworm-slim` | Minimal runtime with language runtimes installed |

The container must be started with `privileged: true` and `cgroup: host` to allow nsjail to create namespaces and the cgroup v2 hierarchy to be writable.

Build args `GIT_COMMIT`, `NSJAIL_VERSION`, and `GO_VERSION` are stamped into the image and exposed at `/info`.
