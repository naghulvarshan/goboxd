# Getting started

## Prerequisites

- Docker with Compose v2 (`docker compose version` ≥ 2.0)
- Git

No Go toolchain or system dependencies are required on the host. Everything runs inside Docker.

## Installation

```sh
git clone https://github.com/thesouldev/goboxd.git
cd goboxd
make build   # builds the Docker image goboxd:dev
make run     # starts the container, listening on :8080
```

The first build compiles nsjail from source, which takes a few minutes. Subsequent builds are cached.

Verify the service is up:

```sh
curl http://localhost:8080/healthz
# {"status":"ok"}
```

Check that all language runtimes are reachable:

```sh
curl http://localhost:8080/readyz
```

A `200` response with `"status":"success"` means every configured language passed its version check.

---

## Running code

All execution goes through `POST /run`. The request body is JSON.

### Required fields

| Field        | Type     | Description                                      |
|--------------|----------|--------------------------------------------------|
| `language`   | string   | Language ID from `config.yaml` (e.g. `py3`, `c`) |
| `source`     | string   | Source code, max 256 KiB                         |
| `tests`      | array    | At least one test case (or provide `evaluation_script`) |

### Test case fields

| Field             | Type   | Description                  |
|-------------------|--------|------------------------------|
| `stdin`           | string | Input piped to the program   |
| `expected_stdout` | string | Expected standard output     |

### Optional fields

| Field                  | Type   | Description                                               |
|------------------------|--------|-----------------------------------------------------------|
| `source_filename`      | string | Override the default filename for the language            |
| `artifact_filename`    | string | Binary name for compiled languages (e.g. `Solution`)     |
| `build.limits`         | object | Wall time, memory, and process limits for compilation     |
| `run.limits`           | object | Wall time, memory, and process limits per test run        |
| `build.flags`          | array  | Compiler flags substituted into `{{flags}}` in config args |
| `evaluation_script`    | string | Custom grader script (replaces test cases)                |
| `evaluation_script_lang` | string | Interpreter path for the evaluator (e.g. `/usr/bin/python3`) |

### Limits object

```json
{
  "wall_time_s":   5,
  "memory_kb":     524288,
  "max_processes": 64
}
```

Omit a field or set it to `0` to use the language default from `config.yaml`.

---

## Examples

### Python 3 — hello world

```sh
curl -s -X POST http://localhost:8080/run \
  -H "Content-Type: application/json" \
  -d '{
    "language": "py3",
    "source": "print(\"Hello, World!\")",
    "tests": [{"stdin": "", "expected_stdout": "Hello, World!\n"}]
  }'
```

Response:

```json
{
  "status": "success",
  "test": [
    {"status": "accepted", "stdout": "Hello, World!\n", "stderr": "", "duration_ms": 38, "memory_peak_kb": 0}
  ]
}
```

### C — compile and run with stdin

```sh
cat > /tmp/req.json << 'EOF'
{
  "language": "c",
  "source": "#include <stdio.h>\nint main(){int n;scanf(\"%d\",&n);printf(\"%d\\n\",n*n);return 0;}",
  "source_filename": "solution.c",
  "artifact_filename": "solution",
  "build": {"limits": {"wall_time_s": 10, "memory_kb": 524288, "max_processes": 100}},
  "run":   {"limits": {"wall_time_s": 5,  "memory_kb": 524288, "max_processes": 64}},
  "tests": [
    {"stdin": "7",  "expected_stdout": "49\n"},
    {"stdin": "12", "expected_stdout": "144\n"}
  ]
}
EOF
curl -s -X POST http://localhost:8080/run -H "Content-Type: application/json" -d @/tmp/req.json
```

### Bash

```sh
curl -s -X POST http://localhost:8080/run \
  -H "Content-Type: application/json" \
  -d '{
    "language": "bash",
    "source": "#!/bin/bash\necho \"Hello from Bash!\"",
    "source_filename": "solution.sh",
    "run": {"limits": {"wall_time_s": 5, "memory_kb": 524288, "max_processes": 64}},
    "tests": [{"stdin": "", "expected_stdout": "Hello from Bash!\n"}]
  }'
```

### Compiler flags (C++ with optimisation)

```sh
curl -s -X POST http://localhost:8080/run \
  -H "Content-Type: application/json" \
  -d '{
    "language": "cpp",
    "source": "#include <iostream>\nint main(){std::cout<<\"fast\\n\";}",
    "source_filename": "sol.cpp",
    "artifact_filename": "sol",
    "build": {
      "flags": ["-O2", "-std=c++17"],
      "limits": {"wall_time_s": 10, "memory_kb": 524288, "max_processes": 100}
    },
    "run": {"limits": {"wall_time_s": 5, "memory_kb": 524288, "max_processes": 64}},
    "tests": [{"stdin": "", "expected_stdout": "fast\n"}]
  }'
```

### Evaluator script

Use an evaluator when the expected output cannot be compared as a literal string (e.g. floating-point answers, multiple valid orderings).

```sh
cat > /tmp/eval_req.json << 'EOF'
{
  "language": "py3",
  "source": "print(3.14159)",
  "source_filename": "solution.py",
  "run": {"limits": {"wall_time_s": 5, "memory_kb": 524288, "max_processes": 64}},
  "evaluation_script": "import subprocess, json, sys\nresult = subprocess.run([sys.argv[2]], capture_output=True, text=True)\nval = float(result.stdout.strip())\nprint(json.dumps({'accepted': abs(val - 3.14159) < 0.001}))",
  "evaluation_script_lang": "/usr/bin/python3"
}
EOF
curl -s -X POST http://localhost:8080/run -H "Content-Type: application/json" -d @/tmp/eval_req.json
```

The evaluator receives the source filename as its last argument, runs the submission, and prints a JSON result to stdout which is returned in `evaluation_result_json`.

---

## Response reference

```json
{
  "status": "success",
  "build": {
    "status": "ok",
    "stdout": "",
    "stderr": "",
    "duration_ms": 210
  },
  "test": [
    {
      "status": "accepted",
      "stdout": "expected output\n",
      "stderr": "",
      "duration_ms": 45,
      "memory_peak_kb": 1280
    }
  ]
}
```

### Top-level `status`

| Value               | Meaning                                     |
|---------------------|---------------------------------------------|
| `success`           | All steps completed (individual tests may still fail) |
| `compilation_error` | Compiler exited non-zero; see `build.stderr` |

### Per-test `status`

| Value            | Meaning                                      |
|------------------|----------------------------------------------|
| `accepted`       | Output matched `expected_stdout`             |
| `wrong_output`   | Program succeeded but output did not match   |
| `time_exceeded`  | Ran past `wall_time_s`                       |
| `memory_exceeded`| Exceeded `memory_kb` limit                   |
| `errored`        | Non-zero exit for another reason             |

### Error response (4xx)

```json
{
  "error": {
    "code": "unkown_lanugage",
    "message": "langauge is unkown"
  }
}
```

---

## Development

```sh
make test             # unit tests (runs inside tools container)
make run-integration  # integration tests against a live server on :8080
make lint             # golangci-lint

# Run only a specific integration test group or case:
make run-integration TEST_FLAG=Python3
make run-integration TEST_FLAG=Java/HelloWorld
```

### Running unit tests locally (no Docker)

```sh
cd internal
go test ./...
```

### Adding a language

1. Write `scripts/lang_install/<lang>.sh` — installs the toolchain with `apt-get`.
2. Call it from `scripts/install.sh`.
3. Add an entry to `config.yaml`.
4. Add test cases under `tests/testcases/<lang>/`.
5. `make build && make run && make run-integration TEST_FLAG=<Lang>`.

---

## Troubleshooting

**`errored` with nsjail log showing `execve failed: No such file or directory`** — the bind mounts in `default_common_settings.nsjail_args` do not include the runtime directory. Add `-B /path/to/runtime` to `nsjail_args` in `config.yaml`.

**`400` for a language that is in `config.yaml`** — the server loaded the config before your edit. Run `make build && make run` to rebuild and restart.

**`/readyz` shows a language as degraded** — the `version_cmd` for that language fails. Run `docker exec goboxd <version_cmd>` to diagnose.
