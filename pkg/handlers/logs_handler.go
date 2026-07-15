package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/handlers/wsutil"
	"github.com/zxh326/kite/pkg/kube"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/rbac"
	"golang.org/x/net/websocket"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type LogsHandler struct {
}

const (
	defaultPodLogTailLines int64 = 500
	maxPodLogTailLines     int64 = 100_000
)

func NewLogsHandler() *LogsHandler {
	return &LogsHandler{}
}

type podLogsResponse struct {
	Logs      []string `json:"logs"`
	Container string   `json:"container,omitempty"`
	Pod       string   `json:"pod"`
	Namespace string   `json:"namespace"`
	HasMore   bool     `json:"hasMore"`
	Warnings  []string `json:"warnings,omitempty"`
}

// HandleLogs returns a bounded snapshot of pod logs. The client uses tailLines
// for historical backfill and sinceTime for lightweight incremental polling.
func (h *LogsHandler) HandleLogs(c *gin.Context) {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	user := c.MustGet("user").(model.User)
	namespace := c.Param("namespace")
	podName := c.Param("podName")
	if namespace == "" || namespace == common.AllNamespaces || podName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "a concrete namespace and podName are required"})
		return
	}

	if !rbac.CanAccess(user, string(common.Pods), "log", cs.Name, namespace) {
		c.JSON(http.StatusForbidden, gin.H{"error": rbac.NoAccess(user.Key(), string(common.VerbLog), string(common.Pods), namespace, cs.Name)})
		return
	}
	labelSelector := c.Query("labelSelector")
	if podName == common.AllNamespaces {
		if labelSelector == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "labelSelector is required when podName is _all"})
			return
		}
		if _, err := labels.Parse(labelSelector); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid labelSelector parameter"})
			return
		}
	}

	logOptions, err := parsePodLogOptions(c, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	logs, hasMore, warnings, err := getPodLogSnapshot(c.Request.Context(), cs, namespace, podName, labelSelector, logOptions)
	if err != nil {
		status := http.StatusBadGateway
		switch {
		case apierrors.IsNotFound(err):
			status = http.StatusNotFound
		case apierrors.IsForbidden(err):
			status = http.StatusForbidden
		}
		c.JSON(status, gin.H{"error": "failed to get pod logs: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, podLogsResponse{
		Logs:      logs,
		Container: logOptions.Container,
		Pod:       podName,
		Namespace: namespace,
		HasMore:   hasMore,
		Warnings:  warnings,
	})
}

func getPodLogSnapshot(ctx context.Context, cs *cluster.ClientSet, namespace, podName, labelSelector string, options *corev1.PodLogOptions) ([]string, bool, []string, error) {
	if podName != common.AllNamespaces {
		raw, err := cs.K8sClient.ClientSet.CoreV1().Pods(namespace).GetLogs(podName, options).DoRaw(ctx)
		if err != nil {
			return nil, false, nil, err
		}
		logs := splitPodLogLines(string(raw))
		hasMore := options.TailLines != nil && int64(len(logs)) >= *options.TailLines && *options.TailLines < maxPodLogTailLines
		return logs, hasMore, nil, nil
	}

	if labelSelector == "" {
		return nil, false, nil, errors.New("labelSelector is required when podName is _all")
	}
	if _, err := labels.Parse(labelSelector); err != nil {
		return nil, false, nil, errors.New("invalid labelSelector parameter")
	}

	pods, err := cs.K8sClient.ClientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, false, nil, err
	}

	logs := make([]string, 0)
	warnings := make([]string, 0)
	var firstError error
	successfulPods := 0
	totalLines := 0
	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		raw, err := cs.K8sClient.ClientSet.CoreV1().Pods(namespace).GetLogs(pod.Name, options).DoRaw(ctx)
		if err != nil {
			if firstError == nil {
				firstError = err
			}
			warnings = append(warnings, fmt.Sprintf("%s: %v", pod.Name, err))
			continue
		}
		successfulPods++
		podLines := splitPodLogLines(string(raw))
		totalLines += len(podLines)
		for _, line := range podLines {
			logs = append(logs, prefixPodLogLine(pod.Name, line, options.Timestamps))
		}
		if options.Timestamps && options.TailLines != nil && int64(len(logs)) > *options.TailLines {
			sortPodLogLines(logs)
			logs = logs[len(logs)-int(*options.TailLines):]
		}
	}

	if successfulPods == 0 && firstError != nil {
		return nil, false, warnings, firstError
	}
	if options.Timestamps {
		sortPodLogLines(logs)
	}

	hasMore := options.TailLines != nil && int64(totalLines) >= *options.TailLines && *options.TailLines < maxPodLogTailLines
	if options.TailLines != nil && int64(len(logs)) > *options.TailLines {
		logs = logs[len(logs)-int(*options.TailLines):]
	}
	return logs, hasMore, warnings, nil
}

func sortPodLogLines(logs []string) {
	sort.SliceStable(logs, func(i, j int) bool {
		left, leftOK := podLogTimestamp(logs[i])
		right, rightOK := podLogTimestamp(logs[j])
		if leftOK != rightOK {
			return leftOK
		}
		return leftOK && left.Before(right)
	})
}

func prefixPodLogLine(podName, line string, timestamps bool) string {
	if timestamps {
		if separator := strings.IndexByte(line, ' '); separator > 0 {
			return line[:separator] + " [" + podName + "]: " + line[separator+1:]
		}
	}
	return "[" + podName + "]: " + line
}

func podLogTimestamp(line string) (time.Time, bool) {
	value, _, found := strings.Cut(line, " ")
	if !found {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return parsed, err == nil
}

func parsePodLogOptions(c *gin.Context, follow bool) (*corev1.PodLogOptions, error) {
	tail, err := strconv.ParseInt(c.DefaultQuery("tailLines", strconv.FormatInt(defaultPodLogTailLines, 10)), 10, 64)
	if err != nil || tail == 0 || tail < -1 || tail > maxPodLogTailLines {
		return nil, errors.New("invalid tailLines parameter")
	}
	if tail == -1 && c.Query("sinceTime") == "" && c.Query("sinceSeconds") == "" {
		return nil, errors.New("invalid tailLines parameter")
	}

	timestamps, err := strconv.ParseBool(c.DefaultQuery("timestamps", "true"))
	if err != nil {
		return nil, errors.New("invalid timestamps parameter")
	}
	previous, err := strconv.ParseBool(c.DefaultQuery("previous", "false"))
	if err != nil {
		return nil, errors.New("invalid previous parameter")
	}

	options := &corev1.PodLogOptions{
		Container:  c.Query("container"),
		Follow:     follow,
		Timestamps: timestamps,
		Previous:   previous,
	}
	if tail != -1 {
		options.TailLines = &tail
	}

	if sinceSeconds := c.Query("sinceSeconds"); sinceSeconds != "" {
		since, err := strconv.ParseInt(sinceSeconds, 10, 64)
		if err != nil || since < 0 {
			return nil, errors.New("invalid sinceSeconds parameter")
		}
		options.SinceSeconds = &since
	}

	if sinceTime := c.Query("sinceTime"); sinceTime != "" {
		if options.SinceSeconds != nil {
			return nil, errors.New("sinceSeconds and sinceTime cannot be used together")
		}
		parsed, err := time.Parse(time.RFC3339Nano, sinceTime)
		if err != nil {
			return nil, errors.New("invalid sinceTime parameter")
		}
		value := metav1.NewTime(parsed)
		options.SinceTime = &value
	}

	return options, nil
}

func splitPodLogLines(raw string) []string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.TrimRight(raw, "\n")
	if raw == "" {
		return []string{}
	}
	return strings.Split(raw, "\n")
}

// HandleLogsWebSocket handles WebSocket connections for log streaming
func (h *LogsHandler) HandleLogsWebSocket(c *gin.Context) {
	websocket.Handler(func(ws *websocket.Conn) {
		ctx, cancel := context.WithCancel(c.Request.Context())
		defer cancel()
		cs := c.MustGet("cluster").(*cluster.ClientSet)
		user := c.MustGet("user").(model.User)
		namespace := c.Param("namespace")
		podName := c.Param("podName")
		if namespace == "" || podName == "" {
			wsutil.SendErrorMessage(ws, "namespace and podName are required")
			return
		}

		if !rbac.CanAccess(user, string(common.Pods), "log", cs.Name, namespace) {
			wsutil.SendErrorMessage(ws, rbac.NoAccess(user.Key(), string(common.VerbLog), string(common.Pods), namespace, cs.Name))
			return
		}

		container := c.Query("container")
		tailLines := c.DefaultQuery("tailLines", "100")
		timestamps := c.DefaultQuery("timestamps", "true")
		previous := c.DefaultQuery("previous", "false")
		sinceSeconds := c.Query("sinceSeconds")

		tail, err := strconv.ParseInt(tailLines, 10, 64)
		if err != nil {
			wsutil.SendErrorMessage(ws, "invalid tailLines parameter")
			return
		}
		timestampsBool := timestamps == "true"
		previousBool := previous == "true"
		tailPtr := &tail
		if *tailPtr == -1 {
			tailPtr = nil
		}

		// Build log options
		logOptions := &corev1.PodLogOptions{
			Container:  container,
			Follow:     true,
			Timestamps: timestampsBool,
			TailLines:  tailPtr,
			Previous:   previousBool,
		}

		if sinceSeconds != "" {
			since, err := strconv.ParseInt(sinceSeconds, 10, 64)
			if err != nil {
				wsutil.SendErrorMessage(ws, "invalid sinceSeconds parameter")
				return
			}
			logOptions.SinceSeconds = &since
		}

		labelSelector := c.Query("labelSelector")
		bl := kube.NewBatchLogHandler(ws, cs.K8sClient, logOptions)

		if podName == common.AllNamespaces && labelSelector != "" {
			selector, err := metav1.ParseToLabelSelector(labelSelector)
			if err != nil {
				wsutil.SendErrorMessage(ws, "invalid labelSelector parameter: "+err.Error())
				return
			}
			labelSelectorOption, err := metav1.LabelSelectorAsSelector(selector)
			if err != nil {
				wsutil.SendErrorMessage(ws, "failed to convert labelSelector: "+err.Error())
				return
			}

			podList := &corev1.PodList{}
			var listOpts []client.ListOption
			listOpts = append(listOpts, client.InNamespace(namespace))
			listOpts = append(listOpts, client.MatchingLabelsSelector{Selector: labelSelectorOption})
			if err := cs.K8sClient.List(ctx, podList, listOpts...); err != nil {
				wsutil.SendErrorMessage(ws, "failed to list pods: "+err.Error())
				return
			}
			for _, pod := range podList.Items {
				if pod.Status.Phase == corev1.PodRunning {
					bl.AddPod(pod)
				}
			}

			go h.watchPods(ctx, cs, namespace, labelSelectorOption, bl)
		} else {
			bl.AddPod(corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: namespace,
				},
			})
		}

		bl.StreamLogs(ctx)
	}).ServeHTTP(c.Writer, c.Request)
}

func (h *LogsHandler) watchPods(ctx context.Context, cs *cluster.ClientSet, namespace string, labelSelector labels.Selector, bl *kube.BatchLogHandler) {
	listOptions := metav1.ListOptions{
		LabelSelector: labelSelector.String(),
	}

	watchInterface, err := cs.K8sClient.ClientSet.CoreV1().Pods(namespace).Watch(ctx, listOptions)
	if err != nil {
		return
	}
	defer watchInterface.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watchInterface.ResultChan():
			if !ok {
				return
			}

			pod, ok := event.Object.(*corev1.Pod)
			if !ok {
				continue
			}

			klog.Infof("Pod %s in namespace %s is %s, event Type: %s", pod.Name, pod.Namespace, pod.Status.Phase, event.Type)

			switch event.Type {
			case watch.Added, watch.Modified:
				if pod.Status.Phase == corev1.PodRunning {
					bl.AddPod(*pod)
				} else {
					bl.RemovePod(*pod)
				}
			case watch.Deleted:
				bl.RemovePod(*pod)
			}
		}
	}
}
