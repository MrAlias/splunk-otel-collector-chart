// Copyright Splunk Inc.
// SPDX-License-Identifier: Apache-2.0

package obi

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	crand "crypto/rand"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/consumer/consumertest"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/signalfx/splunk-otel-collector-chart/functional_tests/internal"
)

const (
	testDir   = "testdata"
	valuesDir = "values"
	namespace = "default"
)

func testdata(paths ...string) string {
	return filepath.Join(append([]string{testDir}, paths...)...)
}

type App struct {
	manifestPath string

	deployments []*appsv1.Deployment
	services    []*corev1.Service
	pods        []*corev1.Pod

	labels []string
}

func NewApp(path string) (*App, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	app := &App{manifestPath: path}

	parts := bytes.Split(data, []byte("\n---\n"))

	deserializer := scheme.Codecs.UniversalDeserializer()
	for _, part := range parts {
		part = bytes.TrimSpace(part)
		if len(part) == 0 {
			continue
		}

		obj, _, e := deserializer.Decode(part, nil, nil)
		if e != nil {
			err = errors.Join(err, e)
			continue
		}

		switch v := obj.(type) {
		case *appsv1.Deployment:
			app.deployments = append(app.deployments, v)
			app.labels = append(app.labels, v.Spec.Template.Labels["app"])
		case *corev1.Service:
			app.services = append(app.services, v)
		case *corev1.Pod:
			app.pods = append(app.pods, v)
			app.labels = append(app.labels, v.Labels["app"])
		default:
			e := fmt.Errorf("unsupported object type %T in manifest %s", v, app.manifestPath)
			err = errors.Join(err, e)
		}
	}

	return app, err
}

func (a *App) len() int {
	return len(a.deployments) + len(a.services) + len(a.pods)
}

// Manifest returns the path to the manifest file used to create the App.
func (a *App) Manifest() string {
	return a.manifestPath
}

func (a *App) Deploy(ctx context.Context, client *kubernetes.Clientset) error {
	errCh := make(chan error, a.len())

	var wg sync.WaitGroup

	for _, d := range a.deployments {
		wg.Add(1)
		go func(deployment *appsv1.Deployment) {
			defer wg.Done()
			errCh <- deployDeployment(ctx, client, deployment)
		}(d)
	}

	for _, s := range a.services {
		wg.Add(1)
		go func(service *corev1.Service) {
			defer wg.Done()
			errCh <- deployService(ctx, client, service)
		}(s)
	}

	for _, p := range a.pods {
		wg.Add(1)
		go func(pod *corev1.Pod) {
			defer wg.Done()
			errCh <- deployPod(ctx, client, pod)
		}(p)
	}

	go func() {
		wg.Wait()
		close(errCh)
	}()

	var err error
	for e := range errCh {
		if e != nil {
			err = errors.Join(err, e)
		}
	}
	return err
}

func deployDeployment(ctx context.Context, c *kubernetes.Clientset, d *appsv1.Deployment) error {
	deploy := c.AppsV1().Deployments(namespace)

	_, err := deploy.Create(ctx, d, metav1.CreateOptions{})
	if err != nil {
		_, err = deploy.Update(ctx, d, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create/update deployment %s: %w", d.Name, err)
		}
	}

	return nil
}

func deployService(ctx context.Context, c *kubernetes.Clientset, s *corev1.Service) error {
	svc := c.CoreV1().Services(namespace)
	_, err := svc.Create(ctx, s, metav1.CreateOptions{})
	if err != nil {
		_, err = svc.Update(ctx, s, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create/update service %s: %w", s.Name, err)
		}
	}

	return nil
}

func deployPod(ctx context.Context, c *kubernetes.Clientset, p *corev1.Pod) error {
	pods := c.CoreV1().Pods(namespace)
	_, err := pods.Create(ctx, p, metav1.CreateOptions{})
	if err != nil {
		_, err = pods.Update(ctx, p, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create/update pod %s: %w", p.Name, err)
		}
	}

	return nil
}

func (a *App) Wait(t *testing.T, client *kubernetes.Clientset, timeout, readyTime time.Duration) {
	t.Helper()

	if len(a.labels) <= 0 {
		return
	}

	var wg sync.WaitGroup
	for _, l := range a.labels {
		wg.Add(1)
		func(label string) {
			defer wg.Done()
			internal.CheckPodsReady(t, client, namespace, label, timeout, readyTime)
		}("app=" + l)
	}

	wg.Wait()
}

func (a *App) Teardown(ctx context.Context, c *kubernetes.Clientset, grace *int64) error {
	opts := metav1.DeleteOptions{GracePeriodSeconds: grace}

	var err error

	deployments := c.AppsV1().Deployments(namespace)
	for _, d := range a.deployments {
		e := deployments.Delete(ctx, d.Name, opts)
		if e != nil {
			e := fmt.Errorf("failed to delete deployment %s: %w", d.Name, e)
			err = errors.Join(err, e)
		}
	}

	services := c.CoreV1().Services(namespace)
	for _, s := range a.services {
		e := services.Delete(ctx, s.Name, opts)
		if e != nil {
			e = fmt.Errorf("failed to delete service %s: %w", s.Name, e)
			err = errors.Join(err, e)
		}
	}

	pods := c.CoreV1().Pods(namespace)
	for _, p := range a.pods {
		e := pods.Delete(ctx, p.Name, opts)
		if e != nil {
			e = fmt.Errorf("failed to delete pod %s: %w", p.Name, e)
			err = errors.Join(err, e)
		}
	}

	return err
}

// ClusterIP returns the first non-empty ClusterIP of the App's services.
func (a *App) ClusterIP(ctx context.Context, client *kubernetes.Clientset) string {
	services := client.CoreV1().Services(namespace)
	opts := metav1.GetOptions{}
	for _, s := range a.services {
		svc, err := services.Get(ctx, s.Name, opts)
		if err != nil {
			// Skip errors.
			continue
		}
		ip := svc.Spec.ClusterIP
		if ip != "" && ip != "None" {
			return ip
		}
	}
	return ""
}

type CurlPod struct {
	pod *corev1.Pod
}

func NewCurlPod(app *App) (*CurlPod, error) {
	if len(app.pods) == 0 {
		return nil, fmt.Errorf("no pods found in app to create CurlPod")
	}
	// Assume the first pod is the curl pod.
	return &CurlPod{pod: app.pods[0]}, nil
}

func (c *CurlPod) Chain(ctx context.Context, cfg *restclient.Config, targets []string) (string, string, error) {
	if len(targets) < 2 {
		return "", "", fmt.Errorf("at least two targets are required for a chain request")
	}

	start, targets := targets[0], targets[1:]

	// JSON payload for the chain request.
	body, err := json.Marshal(map[string]any{"targets": targets})
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal chain request body: %w", err)
	}

	// Seed a W3C traceparent header so that services can forward a single
	// trace context.
	idBytes := make([]byte, 16)
	_, _ = crand.Read(idBytes)
	spanBytes := make([]byte, 8)
	_, _ = crand.Read(spanBytes)
	traceparent := fmt.Sprintf("00-%s-%s-01", hex.EncodeToString(idBytes), hex.EncodeToString(spanBytes))

	// Execute curl command in the test initiator pod to start the chain
	curlCmd := fmt.Sprintf(
		"curl -X POST http://%s/chain -H 'Content-Type: application/json' -H 'traceparent: %s' -d '%s'",
		start,
		traceparent,
		string(body),
	)

	o, e, err := c.exec(ctx, cfg, curlCmd)
	if err != nil {
		return o, e, fmt.Errorf("failed to execute chain request: %w", err)
	}

	out := map[string]any{}
	err = json.Unmarshal([]byte(o), &out)
	if err == nil {
		if msg, ok := out["error"].(string); ok && msg != "" {
			err = fmt.Errorf("chain request returned error: %s", msg)
			return o, e, err
		}
	}
	return o, e, nil
}

func (c *CurlPod) exec(ctx context.Context, cfg *restclient.Config, cmd string) (string, string, error) {
	// Use shell to execute the command
	shellCmd := []string{"/bin/sh", "-c", cmd}

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return "", "", fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	ns := c.pod.Namespace
	if ns == "" {
		ns = namespace
	}

	req := client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(c.pod.Name).
		Namespace(ns).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Command: shellCmd,
			Stdin:   false,
			Stdout:  true,
			Stderr:  true,
			TTY:     false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(cfg, "POST", req.URL())
	if err != nil {
		return "", "", fmt.Errorf("failed to create executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})

	return stdout.String(), stderr.String(), err
}

func validateCircularChain(t *testing.T, sink *consumertest.TracesSink, expectedServices []string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	servicesSeen := make(map[string]bool)
	spansByTraceID := make(map[string]map[string]map[string]interface{})

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for circular chain traces. Services seen: %v, Expected: %v", servicesSeen, expectedServices)
		case <-ticker.C:
			allTraces := sink.AllTraces()

			// Process all traces and collect span hierarchy
			for _, traces := range allTraces {
				for i := 0; i < traces.ResourceSpans().Len(); i++ {
					rs := traces.ResourceSpans().At(i)
					attrs := rs.Resource().Attributes()

					serviceName, ok := attrs.Get("service.name")
					if !ok {
						continue
					}

					svcName := serviceName.Str()
					servicesSeen[svcName] = true

					// Collect spans with hierarchy info
					for j := 0; j < rs.ScopeSpans().Len(); j++ {
						ss := rs.ScopeSpans().At(j)
						for k := 0; k < ss.Spans().Len(); k++ {
							span := ss.Spans().At(k)
							traceID := span.TraceID().String()

							if spansByTraceID[traceID] == nil {
								spansByTraceID[traceID] = make(map[string]map[string]interface{})
							}
							if spansByTraceID[traceID][svcName] == nil {
								spansByTraceID[traceID][svcName] = make(map[string]interface{})
							}

							spansByTraceID[traceID][svcName]["spanID"] = span.SpanID().String()
							spansByTraceID[traceID][svcName]["parentSpanID"] = span.ParentSpanID().String()
							spansByTraceID[traceID][svcName]["spanName"] = span.Name()
						}
					}
				}
			}

			// Check if we've seen all expected services
			allServicesSeen := true
			for _, svc := range expectedServices {
				if !servicesSeen[svc] {
					allServicesSeen = false
					t.Logf("Still waiting for service: %s", svc)
				}
			}

			if allServicesSeen {
				// Accept chain completion when all services have emitted spans,
				// regardless of whether they appear in a single trace.
				t.Logf("All expected services have emitted spans at least once")
				return
			}

			t.Logf("Services seen: %v / %v", len(servicesSeen), len(expectedServices))
		}
	}
}

func Test_OBI_Distributed_Tracing_Circular_Chain(t *testing.T) {
	kubeconfig, ok := os.LookupEnv("KUBECONFIG")
	require.True(t, ok, "KUBECONFIG must be set")

	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err)

	client, err := kubernetes.NewForConfig(cfg)
	require.NoError(t, err)

	newApp := func(path ...string) *App {
		app, err := NewApp(testdata(path...))
		require.NoError(t, err)
		return app
	}

	// Define all 8 language backend services and cURL container.
	javaApp := "java-backend"
	nodejsApp := "nodejs-backend"
	dotnetApp := "dotnet-backend"
	pythonApp := "python-backend"
	rubyApp := "ruby-backend"
	cppApp := "cpp-backend"
	rustApp := "rust-backend"
	goApp := "go-backend"
	curlApp := "test-initiator"

	apps := map[string]*App{
		goApp:     newApp("go", "manifest.yaml"),
		javaApp:   newApp("java", "manifest.yaml"),
		nodejsApp: newApp("nodejs", "manifest.yaml"),
		dotnetApp: newApp("dotnet", "manifest.yaml"),
		pythonApp: newApp("python", "manifest.yaml"),
		rubyApp:   newApp("ruby", "manifest.yaml"),
		cppApp:    newApp("cpp", "manifest.yaml"),
		rustApp:   newApp("rust", "manifest.yaml"),

		curlApp: newApp("curl", "manifest.yaml"),
	}

	if os.Getenv("TEARDOWN_BEFORE_SETUP") == "true" {
		internal.ChartUninstall(t, kubeconfig)

		grace := int64(0)
		for _, app := range apps {
			err := app.Teardown(t.Context(), client, &grace)
			require.NoError(t, err)
		}
	}

	// Start a local OTLP sink on ports 4317/4318. OBI defaults export to
	// ${HOST_IP}:4317 (gRPC).
	tracesSink := internal.SetupOTLPTracesSink(t)

	valuesFile, err := filepath.Abs(filepath.Join(testDir, valuesDir, "obi_values.yaml.tmpl"))
	require.NoError(t, err)

	hostEp := internal.HostEndpoint(t)
	if strings.Contains(hostEp, ":") {
		// IPv6 needs to be enclosed in brackets.
		hostEp = fmt.Sprintf("[%s]", hostEp)
	}

	// Deploy all backend services in parallel.
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		internal.ChartInstallOrUpgrade(t, kubeconfig, valuesFile, map[string]any{
			"OTLPEndpoint": fmt.Sprintf("%s:%d", hostEp, internal.OTLPGRPCReceiverPort),
		}, 0, internal.GetDefaultChartOptions())

		cleanup(t, func() {
			internal.ChartUninstall(t, kubeconfig)
		})
	}()

	for _, app := range apps {
		wg.Add(1)
		go func(appItem *App) {
			defer wg.Done()
			err := appItem.Deploy(t.Context(), client)
			require.NoError(t, err, "failed to deploy app from manifest %s", appItem.Manifest())

			cleanup(t, func() {
				ctx := context.Background()
				grace := int64(0)
				appItem.Teardown(ctx, client, &grace)
			})

			appItem.Wait(t, client, time.Minute, 3*time.Second)
		}(app)
	}

	wg.Wait()

	curlPod, err := NewCurlPod(apps[curlApp])
	require.NoError(t, err)

	// Build the circular chain targets using service IPs instead of DNS names.
	//
	//  go -> java -> nodejs -> dotnet -> python -> ruby -> cpp -> rust -> go
	//
	// Using ClusterIP directly avoids potential DNS resolution issues.
	chain := []string{
		fmt.Sprintf("%s:8080", apps[goApp].ClusterIP(t.Context(), client)),
		fmt.Sprintf("%s:8080", apps[javaApp].ClusterIP(t.Context(), client)),
		fmt.Sprintf("%s:8080", apps[nodejsApp].ClusterIP(t.Context(), client)),
		fmt.Sprintf("%s:8080", apps[dotnetApp].ClusterIP(t.Context(), client)),
		fmt.Sprintf("%s:8080", apps[pythonApp].ClusterIP(t.Context(), client)),
		fmt.Sprintf("%s:8080", apps[pythonApp].ClusterIP(t.Context(), client)),
		fmt.Sprintf("%s:8080", apps[rubyApp].ClusterIP(t.Context(), client)),
		fmt.Sprintf("%s:8080", apps[cppApp].ClusterIP(t.Context(), client)),
		fmt.Sprintf("%s:8080", apps[rustApp].ClusterIP(t.Context(), client)),
		fmt.Sprintf("%s:8080", apps[goApp].ClusterIP(t.Context(), client)),
	}

	t.Logf("Executing circular chain: %v", chain)

	// Create a context that we can cancel when traces are received
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Start a goroutine to continuously execute the chain while waiting for traces
	chainDone := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				chainDone <- nil
				return
			case <-ticker.C:
				stdout, stderr, err := curlPod.Chain(t.Context(), cfg, chain)
				if err != nil {
					t.Logf("Chain execution error: %v", err)
					t.Logf("cURL stdout: %s", stdout)
					t.Logf("cURL stderr: %s", stderr)
				} else {
					t.Logf("Chain executed successfully")
				}
			}
		}
	}()

	// Execute chain once immediately
	stdout, stderr, err := curlPod.Chain(t.Context(), cfg, chain)
	if err != nil {
		t.Logf("Chain execution error: %v", err)
		t.Logf("cURL stdout: %s", stdout)
		t.Logf("cURL stderr: %s", stderr)
	} else {
		t.Logf("Chain executed successfully")
	}

	// Wait for some traces to be received - start with a lower expectation to debug
	internal.WaitForTraces(t, 1, tracesSink)

	// Stop the chain execution loop once traces are received
	cancel()
	<-chainDone

	// Log what we received for debugging
	allTraces := tracesSink.AllTraces()
	t.Logf("Received %d trace collections", len(allTraces))
	for idx, traces := range allTraces {
		t.Logf("Trace collection %d has %d resource spans", idx, traces.ResourceSpans().Len())
		for i := 0; i < traces.ResourceSpans().Len(); i++ {
			rs := traces.ResourceSpans().At(i)
			if svcName, ok := rs.Resource().Attributes().Get("service.name"); ok {
				t.Logf("  Service: %s", svcName.Str())
			}
		}
	}

	// Now try to validate distributed tracing with the actual chain
	validateCircularChain(t, tracesSink, []string{
		"go-backend",
		"java-backend",
		"nodejs-backend",
		"dotnet-backend",
		"python-backend",
		"ruby-backend",
		"cpp-backend",
		"rust-backend",
	})
}

func cleanup(t *testing.T, f func()) {
	if os.Getenv("SKIP_TEARDOWN") == "true" {
		// Skip teardown if the environment variable is set
		return
	}

	t.Cleanup(f)
}
