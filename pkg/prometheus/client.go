package prometheus

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"k8s.io/klog/v2"
)

type Client struct {
	client promAPI
}

type promAPI interface {
	Config(ctx context.Context) (v1.ConfigResult, error)
	Query(ctx context.Context, query string, ts time.Time, opts ...v1.Option) (model.Value, v1.Warnings, error)
	QueryRange(ctx context.Context, query string, r v1.Range, opts ...v1.Option) (model.Value, v1.Warnings, error)
}

type ResourceMetrics struct {
	CPURequest    float64
	CPUTotal      float64
	MemoryRequest float64
	MemoryTotal   float64
}

// UsageDataPoint represents a single time point in usage metrics
type UsageDataPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// ResourceUsageHistory contains historical usage data for a resource
type ResourceUsageHistory struct {
	CPU        []UsageDataPoint `json:"cpu"`
	Memory     []UsageDataPoint `json:"memory"`
	NetworkIn  []UsageDataPoint `json:"networkIn"`
	NetworkOut []UsageDataPoint `json:"networkOut"`
	DiskRead   []UsageDataPoint `json:"diskRead"`
	DiskWrite  []UsageDataPoint `json:"diskWrite"`
}

// PodMetrics contains metrics for a specific pod
type PodMetrics struct {
	CPU        []UsageDataPoint `json:"cpu"`
	Memory     []UsageDataPoint `json:"memory"`
	NetworkIn  []UsageDataPoint `json:"networkIn"`
	NetworkOut []UsageDataPoint `json:"networkOut"`
	DiskRead   []UsageDataPoint `json:"diskRead"`
	DiskWrite  []UsageDataPoint `json:"diskWrite"`
	Fallback   bool             `json:"fallback"`
}

type PodCurrentMetrics struct {
	PodName   string  `json:"podName"`
	Namespace string  `json:"namespace"`
	CPU       float64 `json:"cpu"`    // CPU cores
	Memory    float64 `json:"memory"` // Memory in MB
}

func NewClientWithRoundTripper(prometheusURL string, rt http.RoundTripper) (*Client, error) {
	if prometheusURL == "" {
		return nil, fmt.Errorf("prometheus URL cannot be empty")
	}
	client, err := api.NewClient(api.Config{
		Address:      prometheusURL,
		RoundTripper: rt,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating prometheus client: %w", err)
	}

	v1api := v1.NewAPI(client)
	return &Client{
		client: v1api,
	}, nil
}

// GetResourceUsageHistory fetches historical usage data for CPU and Memory
func (c *Client) GetResourceUsageHistory(ctx context.Context, instance string, duration string, nodeLabel string) (*ResourceUsageHistory, error) {
	var step time.Duration
	var timeRange time.Duration

	switch duration {
	case "30m":
		timeRange = 30 * time.Minute
		step = 1 * time.Minute
	case "1h":
		timeRange = 1 * time.Hour
		step = 2 * time.Minute
	case "24h":
		timeRange = 24 * time.Hour
		step = 30 * time.Minute
	default:
		return nil, fmt.Errorf("unsupported duration: %s", duration)
	}

	now := time.Now()
	start := now.Add(-timeRange)

	conditions := []string{
		`container!="POD"`, // Exclude the "POD" container
		`container!=""`,    // Exclude empty containers
	}
	cpuConditions := []string{
		`resource="cpu"`,
	}
	memoryConditions := []string{
		`resource="memory"`,
	}
	if instance != "" {
		conditions = append(conditions, fmt.Sprintf(`%s="%s"`, nodeLabel, instance))
		cpuConditions = append(cpuConditions, fmt.Sprintf(`node="%s"`, instance))
		memoryConditions = append(memoryConditions, fmt.Sprintf(`node="%s"`, instance))
	}

	// Query CPU usage percentage - using container CPU usage
	cpuQuery := fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{%s}[1m])) / sum(kube_node_status_allocatable{%s}) * 100`, strings.Join(conditions, ","), strings.Join(cpuConditions, ","))
	cpuData, err := c.queryRange(ctx, cpuQuery, start, now, step)
	if err != nil {
		return nil, fmt.Errorf("error querying CPU usage: %w", err)
	}

	// Query Memory usage percentage - using container memory working set
	memoryQuery := fmt.Sprintf(`sum(container_memory_working_set_bytes{%s}) / sum(kube_node_status_allocatable{%s}) * 100`, strings.Join(conditions, ","), strings.Join(memoryConditions, ","))
	memoryData, err := c.queryRange(ctx, memoryQuery, start, now, step)
	if err != nil {
		return nil, fmt.Errorf("error querying Memory usage: %w", err)
	}

	conditions = []string{}
	if instance != "" {
		conditions = append(conditions, fmt.Sprintf(`%s="%s"`, nodeLabel, instance))
	}

	// Query Network incoming bytes rate (bytes per second)
	networkInQuery := fmt.Sprintf(`sum(rate(container_network_receive_bytes_total{%s}[1m]))`, strings.Join(conditions, ","))
	networkInData, err := c.queryRange(ctx, networkInQuery, start, now, step)
	if err != nil {
		return nil, fmt.Errorf("error querying Network incoming bytes: %w", err)
	}

	// Query Network outgoing bytes rate (bytes per second)
	networkOutQuery := fmt.Sprintf(`sum(rate(container_network_transmit_bytes_total{%s}[1m]))`, strings.Join(conditions, ","))
	networkOutData, err := c.queryRange(ctx, networkOutQuery, start, now, step)
	if err != nil {
		return nil, fmt.Errorf("error querying Network outgoing bytes: %w", err)
	}

	if len(cpuData) == 0 && len(memoryData) == 0 && len(networkInData) == 0 && len(networkOutData) == 0 {
		return nil, fmt.Errorf("metrics-server or kube-state-metrics may not be available or configured correctly")
	}

	return &ResourceUsageHistory{
		CPU:        cpuData,
		Memory:     memoryData,
		NetworkIn:  networkInData,
		NetworkOut: networkOutData,
	}, nil
}

func (c *Client) queryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]UsageDataPoint, error) {
	r := v1.Range{
		Start: start,
		End:   end,
		Step:  step,
	}

	result, warnings, err := c.client.QueryRange(ctx, query, r)
	if err != nil {
		klog.Error("queryRange", "error", err)
		return nil, err
	}
	if len(warnings) > 0 {
		fmt.Printf("Warnings: %v\n", warnings)
	}

	var dataPoints []UsageDataPoint

	switch result.Type() {
	case model.ValMatrix:
		matrix := result.(model.Matrix)
		if len(matrix) > 0 {
			for _, sample := range matrix[0].Values {
				dataPoints = append(dataPoints, UsageDataPoint{
					Timestamp: sample.Timestamp.Time(),
					Value:     float64(sample.Value),
				})
			}
		}
	default:
		return nil, fmt.Errorf("unexpected result type: %s", result.Type())
	}

	return dataPoints, nil
}

// HealthCheck verifies if Prometheus is accessible
func (c *Client) HealthCheck(ctx context.Context) error {
	_, err := c.client.Config(ctx)
	return err
}

// Query executes an instant query against Prometheus
func (c *Client) Query(ctx context.Context, query string, ts time.Time, opts ...v1.Option) (model.Value, v1.Warnings, error) {
	return c.client.Query(ctx, query, ts, opts...)
}

// QueryRange executes a range query against Prometheus
func (c *Client) QueryRange(ctx context.Context, query string, r v1.Range, opts ...v1.Option) (model.Value, v1.Warnings, error) {
	return c.client.QueryRange(ctx, query, r, opts...)
}

func (c *Client) GetCPUUsage(ctx context.Context, namespace, podNamePrefix, container string, timeRange, step time.Duration) ([]UsageDataPoint, error) {
	now := time.Now()
	return c.getCPUUsage(ctx, namespace, podNamePrefix, container, now.Add(-timeRange), now, step)
}

func (c *Client) getCPUUsage(ctx context.Context, namespace, podNamePrefix, container string, start, end time.Time, step time.Duration) ([]UsageDataPoint, error) {
	// Build query conditionally based on whether pod name prefix and container are provided
	conditions := []string{
		`container!="POD"`, // Exclude the "POD" container
		`container!=""`,    // Exclude empty containers
	}
	if podNamePrefix != "" {
		conditions = append(conditions, fmt.Sprintf(`pod=~"%s.*"`, podNamePrefix))
	}
	if container != "" {
		conditions = append(conditions, fmt.Sprintf(`container="%s"`, container))
	}
	if namespace != "" {
		conditions = append(conditions, fmt.Sprintf(`namespace="%s"`, namespace))
	}
	query := fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{%s}[1m]))`, strings.Join(conditions, ","))
	return c.queryRange(ctx, query, start, end, step)
}

func (c *Client) GetMemoryUsage(ctx context.Context, namespace, podNamePrefix, container string, timeRange, step time.Duration) ([]UsageDataPoint, error) {
	now := time.Now()
	return c.getMemoryUsage(ctx, namespace, podNamePrefix, container, now.Add(-timeRange), now, step)
}

func (c *Client) getMemoryUsage(ctx context.Context, namespace, podNamePrefix, container string, start, end time.Time, step time.Duration) ([]UsageDataPoint, error) {
	// Build query conditionally based on whether pod name prefix and container are provided
	conditions := []string{
		`container!="POD"`, // Exclude the "POD" container
		`container!=""`,    // Exclude empty containers
	}
	if podNamePrefix != "" {
		conditions = append(conditions, fmt.Sprintf(`pod=~"%s.*"`, podNamePrefix))
	}
	if container != "" {
		conditions = append(conditions, fmt.Sprintf(`container="%s"`, container))
	}
	if namespace != "" {
		conditions = append(conditions, fmt.Sprintf(`namespace="%s"`, namespace))
	}
	query := fmt.Sprintf(`sum(container_memory_working_set_bytes{%s}) / 1024 / 1024`, strings.Join(conditions, ","))
	return c.queryRange(ctx, query, start, end, step)
}

func (c *Client) GetNetworkInUsage(ctx context.Context, namespace, podNamePrefix, container string, timeRange, step time.Duration) ([]UsageDataPoint, error) {
	now := time.Now()
	return c.getNetworkInUsage(ctx, namespace, podNamePrefix, container, now.Add(-timeRange), now, step)
}

func (c *Client) getNetworkInUsage(ctx context.Context, namespace, podNamePrefix, container string, start, end time.Time, step time.Duration) ([]UsageDataPoint, error) {
	conditions := []string{}
	if podNamePrefix != "" {
		conditions = append(conditions, fmt.Sprintf(`pod=~"%s.*"`, podNamePrefix))
	}
	if container != "" {
		conditions = append(conditions, fmt.Sprintf(`container="%s"`, container))
	}
	if namespace != "" {
		conditions = append(conditions, fmt.Sprintf(`namespace="%s"`, namespace))
	}
	query := fmt.Sprintf(`sum(rate(container_network_receive_bytes_total{%s}[1m]))`, strings.Join(conditions, ","))
	return c.queryRange(ctx, query, start, end, step)
}

func (c *Client) GetNetworkOutUsage(ctx context.Context, namespace, podNamePrefix, container string, timeRange, step time.Duration) ([]UsageDataPoint, error) {
	now := time.Now()
	return c.getNetworkOutUsage(ctx, namespace, podNamePrefix, container, now.Add(-timeRange), now, step)
}

func (c *Client) getNetworkOutUsage(ctx context.Context, namespace, podNamePrefix, container string, start, end time.Time, step time.Duration) ([]UsageDataPoint, error) {
	conditions := []string{}
	if podNamePrefix != "" {
		conditions = append(conditions, fmt.Sprintf(`pod=~"%s.*"`, podNamePrefix))
	}
	if container != "" {
		conditions = append(conditions, fmt.Sprintf(`container="%s"`, container))
	}
	if namespace != "" {
		conditions = append(conditions, fmt.Sprintf(`namespace="%s"`, namespace))
	}
	query := fmt.Sprintf(`sum(rate(container_network_transmit_bytes_total{%s}[1m]))`, strings.Join(conditions, ","))
	return c.queryRange(ctx, query, start, end, step)
}

func (c *Client) GetDiskReadUsage(ctx context.Context, namespace, podNamePrefix, container string, timeRange, step time.Duration) ([]UsageDataPoint, error) {
	now := time.Now()
	return c.getDiskReadUsage(ctx, namespace, podNamePrefix, container, now.Add(-timeRange), now, step)
}

func (c *Client) getDiskReadUsage(ctx context.Context, namespace, podNamePrefix, container string, start, end time.Time, step time.Duration) ([]UsageDataPoint, error) {
	conditions := []string{
		`container!="POD"`, // Exclude the "POD" container
		`container!=""`,    // Exclude empty containers
	}
	if podNamePrefix != "" {
		conditions = append(conditions, fmt.Sprintf(`pod=~"%s.*"`, podNamePrefix))
	}
	if container != "" {
		conditions = append(conditions, fmt.Sprintf(`container="%s"`, container))
	}
	if namespace != "" {
		conditions = append(conditions, fmt.Sprintf(`namespace="%s"`, namespace))
	}
	query := fmt.Sprintf(`sum(rate(container_fs_reads_bytes_total{%s}[1m]))`, strings.Join(conditions, ","))
	return c.queryRange(ctx, query, start, end, step)
}

func (c *Client) GetDiskWriteUsage(ctx context.Context, namespace, podNamePrefix, container string, timeRange, step time.Duration) ([]UsageDataPoint, error) {
	now := time.Now()
	return c.getDiskWriteUsage(ctx, namespace, podNamePrefix, container, now.Add(-timeRange), now, step)
}

func (c *Client) getDiskWriteUsage(ctx context.Context, namespace, podNamePrefix, container string, start, end time.Time, step time.Duration) ([]UsageDataPoint, error) {
	conditions := []string{
		`container!="POD"`, // Exclude the "POD" container
		`container!=""`,    // Exclude empty containers
	}
	if podNamePrefix != "" {
		conditions = append(conditions, fmt.Sprintf(`pod=~"%s.*"`, podNamePrefix))
	}
	if container != "" {
		conditions = append(conditions, fmt.Sprintf(`container="%s"`, container))
	}
	if namespace != "" {
		conditions = append(conditions, fmt.Sprintf(`namespace="%s"`, namespace))
	}
	query := fmt.Sprintf(`sum(rate(container_fs_writes_bytes_total{%s}[1m]))`, strings.Join(conditions, ","))
	return c.queryRange(ctx, query, start, end, step)
}

func FillMissingDataPoints(timeRange time.Duration, step time.Duration, existing []UsageDataPoint) []UsageDataPoint {
	return fillMissingDataPoints(time.Now().Add(-timeRange), step, existing)
}

func fillMissingDataPoints(startTime time.Time, step time.Duration, existing []UsageDataPoint) []UsageDataPoint {
	if len(existing) == 0 {
		return existing
	}

	firstTime := existing[0].Timestamp

	if firstTime.Sub(startTime) <= step {
		return existing
	}

	result := []UsageDataPoint{}
	for t := startTime.Add(step); t.Before(firstTime); t = t.Add(step) {
		result = append(result, UsageDataPoint{
			Timestamp: t,
			Value:     0.0,
		})
	}

	return append(result, existing...)
}

// GetPodMetrics fetches metrics for a specific pod
func (c *Client) GetPodMetrics(ctx context.Context, namespace, podName, container string, duration string) (*PodMetrics, error) {
	var step time.Duration
	var timeRange time.Duration

	switch duration {
	case "30m":
		timeRange = 30 * time.Minute
		step = 15 * time.Second
	case "1h":
		timeRange = 1 * time.Hour
		step = 1 * time.Minute
	case "24h":
		timeRange = 24 * time.Hour
		step = 5 * time.Minute
	default:
		return nil, fmt.Errorf("unsupported duration: %s", duration)
	}

	end := time.Now()
	start := end.Add(-timeRange)

	cpuData, err := c.getCPUUsage(ctx, namespace, podName, container, start, end, step)
	if err != nil {
		return nil, fmt.Errorf("error querying pod CPU usage: %w", err)
	}
	// Memory usage query for specific pod
	memoryData, err := c.getMemoryUsage(ctx, namespace, podName, container, start, end, step)
	if err != nil {
		return nil, fmt.Errorf("error querying pod Memory usage: %w", err)
	}

	networkInData, err := c.getNetworkInUsage(ctx, namespace, podName, container, start, end, step)
	if err != nil {
		return nil, fmt.Errorf("error querying pod Network incoming usage: %w", err)
	}

	networkOutData, err := c.getNetworkOutUsage(ctx, namespace, podName, container, start, end, step)
	if err != nil {
		return nil, fmt.Errorf("error querying pod Network outgoing usage: %w", err)
	}

	diskReadData, err := c.getDiskReadUsage(ctx, namespace, podName, container, start, end, step)
	if err != nil {
		return nil, fmt.Errorf("error querying pod Disk read usage: %w", err)
	}

	diskWriteData, err := c.getDiskWriteUsage(ctx, namespace, podName, container, start, end, step)
	if err != nil {
		return nil, fmt.Errorf("error querying pod Disk write usage: %w", err)
	}

	return &PodMetrics{
		CPU:        fillMissingDataPoints(start, step, cpuData),
		Memory:     fillMissingDataPoints(start, step, memoryData),
		NetworkIn:  fillMissingDataPoints(start, step, networkInData),
		NetworkOut: fillMissingDataPoints(start, step, networkOutData),
		DiskRead:   fillMissingDataPoints(start, step, diskReadData),
		DiskWrite:  fillMissingDataPoints(start, step, diskWriteData),
		Fallback:   false,
	}, nil
}

// NodeDiskMetric contains current disk usage for a single node.
type NodeDiskMetric struct {
	DiskUsed  int64
	DiskTotal int64
}

// GetNodeDiskMetrics fetches current filesystem usage for all nodes in a single Prometheus
// instant query. nodeIPToName maps InternalIP → nodeName for clusters where node-exporter
// uses the "instance" label instead of the "node" label.
//
// It queries the root filesystem (/), excluding virtual fs types.
// Label resolution order: "node" label → strip port from "instance" → IP lookup.
func (c *Client) GetNodeDiskMetrics(ctx context.Context, nodeIPToName map[string]string) (map[string]*NodeDiskMetric, error) {
	now := time.Now()

	// node-exporter root filesystem metrics; exclude virtual/overlay filesystems.
	const fsFilter = `mountpoint="/",fstype!~"tmpfs|overlay|squashfs|ramfs|devtmpfs"`

	usedQuery := fmt.Sprintf(
		`sum by (node,instance) (node_filesystem_size_bytes{%s} - node_filesystem_avail_bytes{%s})`,
		fsFilter, fsFilter,
	)
	totalQuery := fmt.Sprintf(
		`sum by (node,instance) (node_filesystem_size_bytes{%s})`,
		fsFilter,
	)

	usedVal, _, err := c.client.Query(ctx, usedQuery, now)
	if err != nil {
		return nil, fmt.Errorf("query node disk used: %w", err)
	}
	totalVal, _, err := c.client.Query(ctx, totalQuery, now)
	if err != nil {
		return nil, fmt.Errorf("query node disk total: %w", err)
	}

	result := map[string]*NodeDiskMetric{}

	parseVector := func(val model.Value, setter func(*NodeDiskMetric, int64)) {
		v, ok := val.(model.Vector)
		if !ok {
			return
		}
		for _, s := range v {
			name := resolveNodeLabel(s.Metric, nodeIPToName)
			if name == "" {
				continue
			}
			if _, exists := result[name]; !exists {
				result[name] = &NodeDiskMetric{}
			}
			setter(result[name], int64(s.Value))
		}
	}

	parseVector(usedVal, func(m *NodeDiskMetric, v int64) { m.DiskUsed = v })
	parseVector(totalVal, func(m *NodeDiskMetric, v int64) { m.DiskTotal = v })

	if len(result) == 0 {
		return nil, fmt.Errorf("no node disk metrics returned from Prometheus")
	}
	return result, nil
}

// resolveNodeLabel maps a Prometheus metric's labels to a K8s node name.
// It prefers the "node" label, then resolves "instance" (ip:port) via nodeIPToName.
func resolveNodeLabel(metric model.Metric, nodeIPToName map[string]string) string {
	if node, ok := metric["node"]; ok && string(node) != "" {
		return string(node)
	}
	if instance, ok := metric["instance"]; ok {
		ip := string(instance)
		if colon := strings.LastIndex(ip, ":"); colon != -1 {
			ip = ip[:colon]
		}
		if name, ok := nodeIPToName[ip]; ok {
			return name
		}
	}
	return ""
}

// NodeInstantMetric contains current CPU and memory usage for a single node.
type NodeInstantMetric struct {
	// CPUUsageMillicores is CPU usage in millicores (cores × 1000).
	// Derived from rate(node_cpu_seconds_total{mode!="idle"}[2m]) — same source as Lens.
	CPUUsageMillicores int64
	// MemoryUsageBytes is resident memory usage in bytes.
	// Derived from node_memory_MemTotal_bytes - node_memory_MemAvailable_bytes.
	MemoryUsageBytes int64
}

// GetNodeInstantMetrics fetches current CPU and memory usage for all nodes via two
// Prometheus instant queries (node-exporter metrics, same as Lens/OpenLens).
// It runs both queries concurrently and returns a map keyed by K8s node name.
func (c *Client) GetNodeInstantMetrics(ctx context.Context, nodeIPToName map[string]string) (map[string]*NodeInstantMetric, error) {
	now := time.Now()

	type queryResult struct {
		val model.Value
		err error
	}

	cpuCh := make(chan queryResult, 1)
	memCh := make(chan queryResult, 1)

	// CPU: rate of non-idle cpu seconds → cores → ×1000 = millicores
	go func() {
		val, _, err := c.client.Query(ctx,
			`sum by (node,instance) (rate(node_cpu_seconds_total{mode!="idle"}[2m]))`,
			now,
		)
		cpuCh <- queryResult{val, err}
	}()

	// Memory: total - available = used (same formula as Lens)
	go func() {
		val, _, err := c.client.Query(ctx,
			`sum by (node,instance) (node_memory_MemTotal_bytes - node_memory_MemAvailable_bytes)`,
			now,
		)
		memCh <- queryResult{val, err}
	}()

	cpuRes := <-cpuCh
	memRes := <-memCh

	if cpuRes.err != nil {
		return nil, fmt.Errorf("query node cpu: %w", cpuRes.err)
	}
	if memRes.err != nil {
		return nil, fmt.Errorf("query node memory: %w", memRes.err)
	}

	result := map[string]*NodeInstantMetric{}

	parseVector := func(val model.Value, setter func(*NodeInstantMetric, float64)) {
		v, ok := val.(model.Vector)
		if !ok {
			return
		}
		for _, s := range v {
			name := resolveNodeLabel(s.Metric, nodeIPToName)
			if name == "" {
				continue
			}
			if _, exists := result[name]; !exists {
				result[name] = &NodeInstantMetric{}
			}
			setter(result[name], float64(s.Value))
		}
	}

	parseVector(cpuRes.val, func(m *NodeInstantMetric, v float64) { m.CPUUsageMillicores = int64(v * 1000) })
	parseVector(memRes.val, func(m *NodeInstantMetric, v float64) { m.MemoryUsageBytes = int64(v) })

	if len(result) == 0 {
		return nil, fmt.Errorf("no node instant metrics returned from Prometheus")
	}
	return result, nil
}
