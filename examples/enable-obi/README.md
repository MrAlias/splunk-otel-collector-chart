# OpenTelemetry eBPF Instrumentation (OBI) Example

This example demonstrates how to enable OpenTelemetry eBPF Instrumentation (OBI) alongside the Splunk OpenTelemetry Collector to automatically instrument applications running in Kubernetes.

## What is OBI?

OpenTelemetry eBPF Instrumentation (OBI) is an eBPF-based instrumentation system that automatically collects:

- **Distributed Traces**: Service-to-service communication without code changes
- **RED Metrics**: Request, Error, Duration metrics for application services
- **Application Metrics**: Custom application metrics via eBPF instrumentation
- **Service Graph**: Automatic service dependency mapping

OBI achieves this **without requiring code changes or application restarts** by instrumenting processes at the kernel level using eBPF (extended Berkeley Packet Filter).

## Prerequisites

Before enabling OBI, ensure your environment meets these requirements:

### Kubernetes Cluster Requirements

- **Linux-only**: OBI requires eBPF, which is a Linux kernel feature. Worker nodes must run Linux.
- **Kernel Version**: Linux kernel 4.x or later with eBPF support enabled.
- **eBPF Capability**: Verify kernel support:
  ```bash
  cat /boot/config-$(uname -r) | grep CONFIG_BPF
  # Should see: CONFIG_BPF=y and CONFIG_HAVE_EBPF_JIT=y
  ```

### Splunk OpenTelemetry Collector

OBI requires either the agent or gateway to be enabled to route telemetry data to Splunk:

- **Agent Mode** (default): OBI sends data to the local agent on each node
- **Gateway Mode**: OBI sends data to a centralized gateway collector

### Privileged Containers

OBI must run as a privileged container with `hostPID` and `hostNetwork` enabled to access kernel instrumentation hooks.

### Distribution-Specific Limitations

OBI is **not supported** on:

- **EKS Fargate**: OBI requires privileged containers and `hostPID`, which are not supported on Fargate
- **GKE Autopilot**: GKE Autopilot has similar restrictions on privileged containers

## Installation

### 1. Set Required Environment Variables

```bash
export SPLUNK_ACCESS_TOKEN="your-splunk-access-token"
export SPLUNK_REALM="us0"  # or your appropriate realm
```

### 2. Install with OBI Enabled

```bash
helm install splunk-otel-collector splunk-otel-collector-chart/splunk-otel-collector \
  --namespace splunk-monitoring --create-namespace \
  -f enable-obi-values.yaml \
  --set splunkObservability.realm=$SPLUNK_REALM \
  --set splunkObservability.accessToken=$SPLUNK_ACCESS_TOKEN \
  --set clusterName=my-cluster
```

### 3. Verify OBI Deployment

Check that OBI daemonset is running on all Linux nodes:

```bash
kubectl -n splunk-monitoring get daemonset splunk-otel-collector-obi
kubectl -n splunk-monitoring get pods -l app=splunk-otel-collector-obi
```

## Configuration

### Basic Configuration (enable-obi-values.yaml)

The example values file includes:

- **clusterName**: Identifies the cluster in Splunk (required for OBI)
- **splunkObservability**: Splunk Observability backend configuration
- **agent.enabled**: Enables the agent for local telemetry collection
- **obi.enabled**: Enables OBI eBPF instrumentation

### Production Configuration

For production deployments, consider these customizations:

#### 1. Pin OBI Version

```yaml
obi:
  image:
    tag: "v0.3.0"  # Pin to a specific stable version
```

#### 2. Configure Resources

OBI's resource usage depends on your cluster's workload complexity. Typical values:

```yaml
obi:
  resources:
    limits:
      cpu: 1000m      # Adjust based on workload
      memory: 1Gi     # Adjust based on instrumentation complexity
    requests:
      cpu: 500m
      memory: 512Mi
```

#### 3. Gateway Mode (Recommended for Large Clusters)

For large clusters, use a centralized gateway instead of local agents:

```yaml
agent:
  enabled: false   # Disable agent

gateway:
  enabled: true    # Enable gateway for OBI data collection
  replicaCount: 3  # Adjust based on load
```

#### 4. Custom OBI Configuration

Configure which services to instrument or exclude:

```yaml
obi:
  config:
    discovery:
      services:
        - k8s_owner_name: .  # Instrument all services
    exclude_instrument:
      # Exclude system components
      - "*splunk-otel-collector*"
      - "*obi*"
      # Add other namespaces/pods to exclude
      - "kube-system/*"
      - "kube-node-lease/*"
    routes:
      ignored_patterns:
        - /health
        - /ready
        - /metrics  # Exclude metrics endpoints from tracing
```

## OpenTelemetry Collector Configuration

The collector automatically includes the OTLP receiver to accept data from OBI. The configuration includes:

### Processors

The collector uses these processors with OBI data:

1. **Memory Limiter**: Prevents out-of-memory conditions
2. **Batch Processor**: Batches telemetry for efficiency
3. **K8s Attributes Processor**: Enriches data with Kubernetes metadata
4. **Resource Detection**: Auto-detects cloud provider and cluster information

### Pipelines

OBI data flows through these pipelines:

- **Traces Pipeline**: 
  - Receivers: `otlp` (from OBI)
  - Processing: Memory limiting, batching, K8s attribute enrichment
  - Exporters: Splunk Observability (via otlphttp or direct exporters)

- **Metrics Pipeline**:
  - Receivers: `otlp` (from OBI), plus hostmetrics and kubeletstats
  - Processing: Memory limiting, batching, attribute enhancement
  - Exporters: SignalFx exporter to Splunk Observability

- **Logs Pipeline** (if enabled):
  - OBI pods are automatically excluded from log collection to prevent circular telemetry

### Example Collector Configuration Section

The collector configuration (in ConfigMaps) includes:

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317  # OBI sends traces/metrics here
      http:
        endpoint: 0.0.0.0:4318  # OBI sends traces/metrics here

processors:
  # Memory limiter prevents OOM
  memory_limiter:
    check_interval: 1s
    limit_mib: 500
  
  # Batch processor for efficiency
  batch:
    send_batch_size: 100
    timeout: 10s
  
  # K8s attributes enrichment
  k8sattributes:
    auth_type: serviceAccount
    passthrough: false
    extract:
      metadata:
        - container.id
        - k8s.volume.type
      labels:
        - tag_name: k8s.pod.labels.app
          key: app

service:
  pipelines:
    traces:
      receivers: [otlp, jaeger, zipkin]  # OTLP for OBI
      processors: [memory_limiter, k8sattributes, batch]
      exporters: [otlphttp]
    
    metrics:
      receivers: [hostmetrics, kubeletstats, otlp]  # OTLP for OBI
      processors: [memory_limiter, batch, k8sattributes]
      exporters: [signalfx]
```

## Viewing OBI Data in Splunk

Once OBI is running and sending data:

### 1. Traces

View automatically collected traces in Splunk Observability:

- Navigate to **Traces** tab
- Filter by `k8s.cluster.name: my-cluster`
- View service-to-service communication without instrumentation

### 2. RED Metrics

Access automatic RED metrics:

- Navigate to **Metrics** tab
- Search for metrics with prefix `otel_ebpf_` or service-specific metrics
- Build dashboards for application health monitoring

### 3. Service Map

Visualize service dependencies:

- Navigate to **Metrics** tab
- Create a service map visualization
- OBI automatically discovers service relationships

## Troubleshooting

### OBI Pods Not Starting

Check pod status:

```bash
kubectl -n splunk-monitoring describe pod -l app=splunk-otel-collector-obi
kubectl -n splunk-monitoring logs -l app=splunk-otel-collector-obi --tail=50
```

Common issues:

- **"eBPF not supported"**: Kernel doesn't have eBPF support. Check prerequisites.
- **"privileged denied"**: Pod security policy restricts privileged containers. Adjust policies.
- **"host filesystem access denied"**: Node permissions restrict access to cgroups. Verify node permissions.

### No Data Appearing in Splunk

Verify the data flow:

1. Check OBI is sending data to the agent/gateway:
   ```bash
   kubectl -n splunk-monitoring logs -l app=splunk-otel-collector-agent | grep otlp
   ```

2. Verify agent/gateway is forwarding to Splunk:
   ```bash
   kubectl -n splunk-monitoring logs -l app=splunk-otel-collector | grep "sent span"
   ```

3. Confirm `clusterName` is set and matches expected value in Splunk

### High CPU or Memory Usage

OBI resource usage depends on the number of processes and services being instrumented:

1. Check current resource consumption:
   ```bash
   kubectl -n splunk-monitoring top pod -l app=splunk-otel-collector-obi
   ```

2. Adjust resource limits if needed:
   ```bash
   kubectl -n splunk-monitoring set resources daemonset splunk-otel-collector-obi \
     --limits=cpu=1000m,memory=1Gi \
     --requests=cpu=500m,memory=512Mi
   ```

## Advanced Topics

### Multi-Cluster Instrumentation

To instrument applications across multiple clusters, use unique `clusterName` for each:

```yaml
clusterName: prod-us-east-1
```

OBI data includes this label, allowing filtering and correlation across clusters.

### Network Policy and OBI

If using network policies, ensure OBI pods can communicate with the agent/gateway:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-obi-to-collector
spec:
  podSelector:
    matchLabels:
      app: splunk-otel-collector-agent
  policyTypes:
    - Ingress
  ingress:
    - from:
        - podSelector:
            matchLabels:
              app: splunk-otel-collector-obi
      ports:
        - protocol: TCP
          port: 4317  # OTLP gRPC
        - protocol: TCP
          port: 4318  # OTLP HTTP
```

### Istio Integration

When Istio is deployed, OBI can instrument both Istio proxies and application services for complete observability.

## Support and Documentation

- [OpenTelemetry Documentation](https://opentelemetry.io/)
- [Splunk Observability Documentation](https://docs.splunk.com/observability/)
- [OBI Project](https://github.com/open-telemetry/ebpf-instrumentation-poc)
- [Splunk Helm Chart Documentation](https://github.com/signalfx/splunk-otel-collector-chart)
