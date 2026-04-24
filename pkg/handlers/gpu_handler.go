package handlers

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GPUNodeInfo 存储节点 GPU 信息
type GPUNodeInfo struct {
	NodeName    string   `json:"nodeName"`
	Capacity    int64    `json:"capacity"`
	Allocatable int64    `json:"allocatable"`
	Used        int64    `json:"used"`
	Free        int64    `json:"free"`
	GPUType     string   `json:"gpuType"`
	Taints      []string `json:"taints,omitempty"`
}

// GPUNamespaceStat 按 Namespace 的 GPU 使用统计
type GPUNamespaceStat struct {
	Namespace string `json:"namespace"`
	GPUCount  int64  `json:"gpuCount"`
}

// GPUModelStat 按模型的 GPU 使用统计
type GPUModelStat struct {
	ModelName string `json:"modelName"`
	GPUCount  int64  `json:"gpuCount"`
}

// GPUOverview GPU 资源概览
type GPUOverview struct {
	Summary struct {
		TotalNodes   int     `json:"totalNodes"`
		TotalGPUs    int64   `json:"totalGPUs"`
		UsedGPUs     int64   `json:"usedGPUs"`
		FreeGPUs     int64   `json:"freeGPUs"`
		UsagePercent float64 `json:"usagePercent"`
	} `json:"summary"`
	FullyFreeNodes     []GPUNodeInfo      `json:"fullyFreeNodes"`
	UntaintedFreeNodes []GPUNodeInfo      `json:"untaintedFreeNodes"`
	TaintedFreeNodes   []GPUNodeInfo      `json:"taintedFreeNodes"`
	PartialFreeNodes   []GPUNodeInfo      `json:"partialFreeNodes"`
	NamespaceStats     []GPUNamespaceStat `json:"namespaceStats"`
	ModelStats       []GPUModelStat     `json:"modelStats"`
	NoModelGPUCount  int64              `json:"noModelGPUCount"`
}

// GetGPUOverview 获取 GPU 资源概览
func GetGPUOverview(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	cs := c.MustGet("cluster").(*cluster.ClientSet)

	// 获取 GPU 节点信息
	nodes, err := getGPUNodes(ctx, cs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get GPU nodes: %v", err)})
		return
	}

	if len(nodes) == 0 {
		// 返回空数据而不是错误
		c.JSON(http.StatusOK, GPUOverview{
			Summary: struct {
				TotalNodes   int     `json:"totalNodes"`
				TotalGPUs    int64   `json:"totalGPUs"`
				UsedGPUs     int64   `json:"usedGPUs"`
				FreeGPUs     int64   `json:"freeGPUs"`
				UsagePercent float64 `json:"usagePercent"`
			}{},
			FullyFreeNodes:     []GPUNodeInfo{},
			UntaintedFreeNodes: []GPUNodeInfo{},
			TaintedFreeNodes:   []GPUNodeInfo{},
			PartialFreeNodes:   []GPUNodeInfo{},
			NamespaceStats:     []GPUNamespaceStat{},
			ModelStats:       []GPUModelStat{},
		})
		return
	}

	// 获取 Pod GPU 使用情况
	nodeGPUUsage, err := getGPUUsageFromPods(ctx, cs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get GPU usage from pods: %v", err)})
		return
	}

	// 更新节点使用情况
	for i := range nodes {
		nodes[i].Used = nodeGPUUsage[nodes[i].NodeName]
		nodes[i].Free = nodes[i].Capacity - nodes[i].Used
	}

	// 获取 LWS 统计信息
	namespaceStats, modelStats, noModelCount := getLWSStats(ctx, cs)

	// 生成概览数据
	overview := buildGPUOverview(nodes, namespaceStats, modelStats, noModelCount)

	c.JSON(http.StatusOK, overview)
}

// getGPUNodes 获取所有 GPU 节点
func getGPUNodes(ctx context.Context, cs *cluster.ClientSet) ([]GPUNodeInfo, error) {
	var nodeList corev1.NodeList
	if err := cs.K8sClient.List(ctx, &nodeList); err != nil {
		return nil, err
	}

	var gpuNodes []GPUNodeInfo
	gpuKeys := []string{
		"nvidia.com/gpu",
		"amd.com/gpu",
		"gpu",
		"alpha.kubernetes.io/nvidia-gpu",
	}

	for _, node := range nodeList.Items {
		var gpuCapacity int64
		var gpuAllocatable int64
		var gpuType string

		// 检查 GPU 资源
		for _, key := range gpuKeys {
			if qty, ok := node.Status.Capacity[corev1.ResourceName(key)]; ok {
				gpuCapacity = qty.Value()
				if allocQty, ok := node.Status.Allocatable[corev1.ResourceName(key)]; ok {
					gpuAllocatable = allocQty.Value()
				}
				gpuType = key
				break
			}
		}

		if gpuCapacity > 0 {
			// 收集污点信息
			var taints []string
			for _, taint := range node.Spec.Taints {
				taints = append(taints, fmt.Sprintf("%s:%s", taint.Key, taint.Effect))
			}

			gpuNodes = append(gpuNodes, GPUNodeInfo{
				NodeName:    node.Name,
				Capacity:    gpuCapacity,
				Allocatable: gpuAllocatable,
				GPUType:     gpuType,
				Taints:      taints,
			})
		}
	}

	return gpuNodes, nil
}

// getGPUUsageFromPods 从 Pod 获取 GPU 使用情况
func getGPUUsageFromPods(ctx context.Context, cs *cluster.ClientSet) (map[string]int64, error) {
	var podList corev1.PodList
	if err := cs.K8sClient.List(ctx, &podList); err != nil {
		return nil, err
	}

	nodeGPUUsage := make(map[string]int64)
	gpuKeys := []string{
		"nvidia.com/gpu",
		"amd.com/gpu",
		"gpu",
		"alpha.kubernetes.io/nvidia-gpu",
	}

	for _, pod := range podList.Items {
		// 只统计 Running 和 Pending 的 Pod
		if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending {
			continue
		}

		if pod.Spec.NodeName == "" {
			continue
		}

		var totalGPU int64
		for _, container := range pod.Spec.Containers {
			for _, key := range gpuKeys {
				resourceName := corev1.ResourceName(key)
				// 优先使用 requests
				if qty, ok := container.Resources.Requests[resourceName]; ok {
					totalGPU += qty.Value()
				} else if qty, ok := container.Resources.Limits[resourceName]; ok {
					// 其次使用 limits
					totalGPU += qty.Value()
				}
			}
		}

		if totalGPU > 0 {
			nodeGPUUsage[pod.Spec.NodeName] += totalGPU
		}
	}

	return nodeGPUUsage, nil
}

// getLWSStats 从 LWS (LeaderWorkerSet) 获取统计信息
func getLWSStats(ctx context.Context, cs *cluster.ClientSet) (map[string]int64, map[string]int64, int64) {
	namespaceStats := make(map[string]int64)
	modelStats := make(map[string]int64)
	var noModelCount int64

	var lwsList unstructured.UnstructuredList
	lwsList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "leaderworkerset.x-k8s.io",
		Version: "v1",
		Kind:    "LeaderWorkerSetList",
	})

	if err := cs.K8sClient.List(ctx, &lwsList, &client.ListOptions{}); err != nil {
		// LWS 可能不存在，不返回错误
		return namespaceStats, modelStats, noModelCount
	}

	gpuKeys := []string{"nvidia.com/gpu", "amd.com/gpu", "gpu"}

	for _, item := range lwsList.Items {
		namespace := item.GetNamespace()

		// 获取 replicas
		replicas, _, _ := unstructured.NestedInt64(item.Object, "spec", "replicas")

		// 获取 size
		size, found, _ := unstructured.NestedInt64(item.Object, "spec", "leaderWorkerTemplate", "size")
		if !found {
			size = 1
		}

		// 获取 leader GPU
		leaderGPU := extractGPUFromContainers(item.Object, []string{"spec", "leaderWorkerTemplate", "leaderTemplate", "spec", "containers"}, gpuKeys)

		// 获取 worker GPU
		workerGPU := extractGPUFromContainers(item.Object, []string{"spec", "leaderWorkerTemplate", "workerTemplate", "spec", "containers"}, gpuKeys)

		// 计算总 GPU: replicas × (leaderGPU + workerGPU × (size-1))
		totalGPU := replicas * (leaderGPU + workerGPU*(size-1))

		if totalGPU > 0 {
			// 统计 namespace
			namespaceStats[namespace] += totalGPU

			// 获取模型名称
			modelName := ""
			leaderLabels, found, _ := unstructured.NestedStringMap(item.Object, "spec", "leaderWorkerTemplate", "leaderTemplate", "metadata", "labels")
			if found && leaderLabels != nil {
				if val, ok := leaderLabels["model.magikcompute.ai/name"]; ok {
					modelName = val
				}
			}

			if modelName != "" {
				modelStats[modelName] += totalGPU
			} else {
				noModelCount += totalGPU
			}
		}
	}

	return namespaceStats, modelStats, noModelCount
}

// extractGPUFromContainers 从容器配置中提取 GPU 数量
func extractGPUFromContainers(obj map[string]interface{}, path []string, gpuKeys []string) int64 {
	containers, _, _ := unstructured.NestedSlice(obj, path...)
	var totalGPU int64

	for _, container := range containers {
		containerMap, ok := container.(map[string]interface{})
		if !ok {
			continue
		}

		// 检查 requests
		if requests, found, _ := unstructured.NestedMap(containerMap, "resources", "requests"); found {
			for _, key := range gpuKeys {
				if gpuValue, ok := requests[key]; ok {
					if gpuInt, ok := gpuValue.(int64); ok {
						totalGPU += gpuInt
					} else if gpuStr, ok := gpuValue.(string); ok {
						var gpu int64
						fmt.Sscanf(gpuStr, "%d", &gpu)
						totalGPU += gpu
					}
					break
				}
			}
		}

		// 如果没有 requests，检查 limits
		if totalGPU == 0 {
			if limits, found, _ := unstructured.NestedMap(containerMap, "resources", "limits"); found {
				for _, key := range gpuKeys {
					if gpuValue, ok := limits[key]; ok {
						if gpuInt, ok := gpuValue.(int64); ok {
							totalGPU += gpuInt
						} else if gpuStr, ok := gpuValue.(string); ok {
							var gpu int64
							fmt.Sscanf(gpuStr, "%d", &gpu)
							totalGPU += gpu
						}
						break
					}
				}
			}
		}
	}

	return totalGPU
}

// buildGPUOverview 构建 GPU 概览数据
func buildGPUOverview(nodes []GPUNodeInfo, namespaceStats, modelStats map[string]int64, noModelCount int64) GPUOverview {
	var overview GPUOverview

	// 排序节点
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].NodeName < nodes[j].NodeName
	})

	// 统计信息
	var totalGPUs, usedGPUs, freeGPUs int64
	var fullyFreeNodes, untaintedFreeNodes, taintedFreeNodes, partialFreeNodes []GPUNodeInfo

	for _, node := range nodes {
		totalGPUs += node.Capacity
		usedGPUs += node.Used
		freeGPUs += node.Free

		if node.Free > 0 {
			if node.Used == 0 {
				fullyFreeNodes = append(fullyFreeNodes, node)
				if len(node.Taints) > 0 {
					taintedFreeNodes = append(taintedFreeNodes, node)
				} else {
					untaintedFreeNodes = append(untaintedFreeNodes, node)
				}
			} else {
				partialFreeNodes = append(partialFreeNodes, node)
			}
		}
	}

	overview.Summary.TotalNodes = len(nodes)
	overview.Summary.TotalGPUs = totalGPUs
	overview.Summary.UsedGPUs = usedGPUs
	overview.Summary.FreeGPUs = freeGPUs
	if totalGPUs > 0 {
		overview.Summary.UsagePercent = float64(usedGPUs) / float64(totalGPUs) * 100
	}

	overview.FullyFreeNodes = fullyFreeNodes
	if overview.FullyFreeNodes == nil {
		overview.FullyFreeNodes = []GPUNodeInfo{}
	}

	overview.UntaintedFreeNodes = untaintedFreeNodes
	if overview.UntaintedFreeNodes == nil {
		overview.UntaintedFreeNodes = []GPUNodeInfo{}
	}

	overview.TaintedFreeNodes = taintedFreeNodes
	if overview.TaintedFreeNodes == nil {
		overview.TaintedFreeNodes = []GPUNodeInfo{}
	}

	overview.PartialFreeNodes = partialFreeNodes
	if overview.PartialFreeNodes == nil {
		overview.PartialFreeNodes = []GPUNodeInfo{}
	}

	// 转换并排序 namespace 统计
	var nsStats []GPUNamespaceStat
	for ns, count := range namespaceStats {
		nsStats = append(nsStats, GPUNamespaceStat{
			Namespace: ns,
			GPUCount:  count,
		})
	}
	sort.Slice(nsStats, func(i, j int) bool {
		return nsStats[i].GPUCount > nsStats[j].GPUCount
	})
	overview.NamespaceStats = nsStats
	if overview.NamespaceStats == nil {
		overview.NamespaceStats = []GPUNamespaceStat{}
	}

	// 转换并排序模型统计
	var mStats []GPUModelStat
	for model, count := range modelStats {
		mStats = append(mStats, GPUModelStat{
			ModelName: model,
			GPUCount:  count,
		})
	}
	sort.Slice(mStats, func(i, j int) bool {
		return mStats[i].GPUCount > mStats[j].GPUCount
	})
	overview.ModelStats = mStats
	if overview.ModelStats == nil {
		overview.ModelStats = []GPUModelStat{}
	}

	overview.NoModelGPUCount = noModelCount

	return overview
}
