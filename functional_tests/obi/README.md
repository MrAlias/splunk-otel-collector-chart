# OBI (OpenTelemetry Binary Injection) Functional Tests

This directory contains functional tests for the OBI sub-chart, which provides automatic instrumentation of applications without code changes.

## Tests

### Test_OBI_Minimal_Traces

Basic test that validates OBI can instrument a single Python application and generate traces.

**Services tested:**
- Python (self-calling HTTP server)

### Test_OBI_Distributed_Tracing

Advanced test that validates distributed context propagation and tracing across multiple programming languages over HTTP protocol.

**Services tested:**
- Frontend: Python service that makes HTTP requests to backends
- Backends:
  - Java (Apache Tomcat)
  - .NET (ASP.NET Core)
  - Node.js (Express.js)
  - Go (httpbin)

**Validation:**
- All services emit traces
- Distributed traces span multiple services
- Trace context is propagated correctly

## Running Tests

```bash
# Set KUBECONFIG
export KUBECONFIG=/tmp/kube-config-splunk-otel-collector-chart-functional-testing

# Run all OBI tests
cd functional_tests
go test -v ./obi

# Run specific test
go test -v ./obi -run Test_OBI_Distributed_Tracing

# With make
make functionaltest SUITE=obi
```

## Test Configuration

Tests support these environment variables:
- `KUBECONFIG`: Path to kubeconfig file (required)
- `SKIP_TEARDOWN`: Keep resources after test for debugging
- `TEARDOWN_BEFORE_SETUP`: Clean up before running test
- `SKIP_SETUP`: Skip chart installation (for re-running tests)

## Test Images

The distributed tracing test uses these images:

### Pre-built Images
- `quay.io/splunko11ytest/java_test:latest` - Java backend
- `quay.io/splunko11ytest/dotnet_test:latest` - .NET backend
- `quay.io/splunko11ytest/nodejs_test:latest` - Node.js backend
- `docker.io/mccutchen/go-httpbin:v2.15.0` - Go backend

### Custom Image (needs to be built)
- `quay.io/splunko11ytest/python_distributed_frontend:latest` - Python frontend

See [testdata/distributed-tracing-apps/README.md](testdata/distributed-tracing-apps/README.md) for build instructions.

## Architecture

```
┌─────────────────────────────────────────────────┐
│  Frontend Service (Python)                      │
│  - Makes HTTP calls to all backends             │
│  - Generates distributed traces                 │
└──┬────────────────────────────────────────────┬─┘
   │                                             │
   │  HTTP                                       │  HTTP
   │                                             │
   ├──────────────┬─────────────────────┬────────┤
   │              │                     │        │
   ▼              ▼                     ▼        ▼
┌──────┐      ┌────────┐          ┌─────────┐  ┌────┐
│ Java │      │ .NET   │          │ Node.js │  │ Go │
│      │      │        │          │         │  │    │
└──────┘      └────────┘          └─────────┘  └────┘

All services instrumented via OBI
Traces collected via OTLP exporter
```

## Troubleshooting

### Services not generating traces

1. Check OBI is enabled in the chart:
   ```bash
   kubectl get daemonset -n default
   # Should see splunk-otel-collector-agent
   ```

2. Check pods are running:
   ```bash
   kubectl get pods -n default
   ```

3. Check OBI instrumentation logs:
   ```bash
   kubectl logs -n default <pod-name> -c splunk-otel-auto-instrumentation
   ```

### Distributed traces not found

1. Verify frontend can reach backends:
   ```bash
   kubectl logs -n default <frontend-pod>
   ```

2. Check trace data at sink:
   - Test outputs trace count periodically
   - Look for "Services seen" in test output

3. Verify network connectivity:
   ```bash
   kubectl exec -n default <frontend-pod> -- curl http://java-backend:8080
   ```
