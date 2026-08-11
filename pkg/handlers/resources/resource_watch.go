package resources

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/kube"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/rbac"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
)

const (
	resourceWatchKeepAliveInterval  = 15 * time.Second
	resourceWatchConnectionLifetime = 5 * time.Minute
)

type resourceWatchSnapshot struct {
	ResourceVersion string `json:"resourceVersion"`
	Items           []any  `json:"items"`
}

type resourceWatchStatus struct {
	ResourceVersion string `json:"resourceVersion,omitempty"`
	Error           string `json:"error,omitempty"`
	Fatal           bool   `json:"fatal,omitempty"`
}

type resourceWatchTransform func(eventType string, obj *unstructured.Unstructured) (any, error)
type resourceWatchTick func() ([]any, error)

type resourceWatchStreamOptions struct {
	GVR                schema.GroupVersionResource
	Resource           string
	ClusterScoped      bool
	Reduce             bool
	BeforeSnapshot     func()
	Transform          resourceWatchTransform
	OnTick             resourceWatchTick
	ConnectionLifetime time.Duration
}

func serveResourceWatch(c *gin.Context, options resourceWatchStreamOptions) {
	flusher, ok := beginResourceWatchStream(c)
	if !ok {
		return
	}

	cs := c.MustGet("cluster").(*cluster.ClientSet)
	if cs.K8sClient == nil || cs.K8sClient.WatchHub == nil {
		writeResourceWatchFatal(c, "resource watch is unavailable")
		return
	}

	namespace := c.Param("namespace")
	if options.ClusterScoped || namespace == "" || namespace == common.AllNamespaces {
		namespace = ""
	}

	// Open the browser-to-Kite SSE connection before joining the internal
	// subscription. Initial list work may take longer than the browser's
	// connection deadline, so the response headers must already be flushed.
	subscription, err := cs.K8sClient.WatchHub.Subscribe(kube.ResourceWatchOptions{
		GVR:           options.GVR,
		Namespace:     namespace,
		LabelSelector: c.Query("labelSelector"),
		FieldSelector: c.Query("fieldSelector"),
	})
	if err != nil {
		writeResourceWatchFatal(c, err.Error())
		return
	}
	defer subscription.Close()

	ticker := time.NewTicker(resourceWatchKeepAliveInterval)
	defer ticker.Stop()
	connectionLifetime := options.ConnectionLifetime
	if connectionLifetime <= 0 {
		connectionLifetime = resourceWatchConnectionLifetime
	}
	lifetime := time.NewTimer(connectionLifetime)
	defer lifetime.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-lifetime.C:
			// Force EventSource to reconnect through Auth and RBAC middleware so
			// user disablement, expiring access, and role changes take effect.
			return
		case event, open := <-subscription.Events:
			if !open {
				return
			}
			if err := writeResourceWatchEvent(c, options, event); err != nil {
				return
			}
			if event.Type == kube.ResourceWatchError && event.Fatal {
				return
			}
		case <-ticker.C:
			if options.OnTick != nil {
				objects, tickErr := options.OnTick()
				if tickErr != nil {
					klog.Warningf("failed to refresh %s watch data: %v", options.Resource, tickErr)
				} else {
					for _, object := range objects {
						if err := writeSSE(c, kube.ResourceWatchModified, object); err != nil {
							return
						}
					}
				}
			}
			if _, err := fmt.Fprint(c.Writer, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// beginResourceWatchStream commits a successful SSE response immediately. It
// is idempotent so dynamic-resource handlers can open the stream before doing
// CRD discovery and still delegate the actual watch to serveResourceWatch.
func beginResourceWatchStream(c *gin.Context) (http.Flusher, bool) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming unsupported"})
		return nil, false
	}

	if c.Writer.Header().Get("Content-Type") == "text/event-stream" {
		return flusher, true
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache, no-transform")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")

	// EventSource uses this delay only when the browser-to-Kite connection
	// fails. Kubernetes reconnection is handled inside the shared hub.
	_, _ = fmt.Fprint(c.Writer, "retry: 5000\n\n")
	flusher.Flush()
	return flusher, true
}

func writeResourceWatchFatal(c *gin.Context, message string) {
	_ = writeSSE(c, kube.ResourceWatchError, resourceWatchStatus{
		Error: message,
		Fatal: true,
	})
}

func writeResourceWatchEvent(
	c *gin.Context,
	options resourceWatchStreamOptions,
	event kube.ResourceWatchEvent,
) error {
	switch event.Type {
	case kube.ResourceWatchSnapshot:
		if options.BeforeSnapshot != nil {
			options.BeforeSnapshot()
		}
		items := filterAndSortWatchSnapshot(c, options.Resource, event.Items)
		transformed := make([]any, 0, len(items))
		for _, item := range items {
			object, err := transformWatchObject(options, kube.ResourceWatchSnapshot, item)
			if err != nil {
				klog.Warningf("failed to transform %s watch snapshot item: %v", options.Resource, err)
				continue
			}
			transformed = append(transformed, object)
		}
		return writeSSE(c, kube.ResourceWatchSnapshot, resourceWatchSnapshot{
			ResourceVersion: event.ResourceVersion,
			Items:           transformed,
		})
	case kube.ResourceWatchAdded, kube.ResourceWatchModified, kube.ResourceWatchDeleted:
		if event.Object == nil || !canStreamWatchObject(c, options.Resource, event.Object) {
			return nil
		}
		object, err := transformWatchObject(options, event.Type, event.Object)
		if err != nil {
			return err
		}
		return writeSSE(c, event.Type, object)
	case kube.ResourceWatchReady:
		return writeSSE(c, kube.ResourceWatchReady, resourceWatchStatus{
			ResourceVersion: event.ResourceVersion,
		})
	case kube.ResourceWatchError:
		return writeSSE(c, kube.ResourceWatchError, resourceWatchStatus{
			Error: event.Error,
			Fatal: event.Fatal,
		})
	default:
		return nil
	}
}

func transformWatchObject(
	options resourceWatchStreamOptions,
	eventType string,
	obj *unstructured.Unstructured,
) (any, error) {
	prepared := prepareWatchObject(obj)
	if options.Reduce {
		reduceWatchObject(options.Resource, prepared)
	}
	if options.Transform == nil {
		return prepared, nil
	}
	return options.Transform(eventType, prepared)
}

func prepareWatchObject(obj *unstructured.Unstructured) *unstructured.Unstructured {
	prepared := obj.DeepCopy()
	prepared.SetManagedFields(nil)
	annotations := prepared.GetAnnotations()
	if annotations != nil {
		delete(annotations, common.KubectlAnnotation)
		prepared.SetAnnotations(annotations)
	}
	return prepared
}

// Avoid continuously sending fields that table views do not consume. CRD
// schemas can be very large; ConfigMap and Secret tables only need data keys.
func reduceWatchObject(resource string, obj *unstructured.Unstructured) {
	switch resource {
	case string(common.CRDs):
		reduceUnstructuredCustomResourceDefinition(obj)
	case string(common.ConfigMaps):
		emptyNestedMapValues(obj.Object, "data")
		emptyNestedMapValues(obj.Object, "binaryData")
	case string(common.Secrets):
		emptyNestedMapValues(obj.Object, "data")
		emptyNestedMapValues(obj.Object, "stringData")
	}
}

func emptyNestedMapValues(object map[string]any, field string) {
	values, found, err := unstructured.NestedMap(object, field)
	if err != nil || !found {
		return
	}
	for key := range values {
		values[key] = ""
	}
	_ = unstructured.SetNestedMap(object, values, field)
}

func filterAndSortWatchSnapshot(
	c *gin.Context,
	resource string,
	items []*unstructured.Unstructured,
) []*unstructured.Unstructured {
	filtered := make([]*unstructured.Unstructured, 0, len(items))
	for _, item := range items {
		if item != nil && canStreamWatchObject(c, resource, item) {
			filtered = append(filtered, item)
		}
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		first := filtered[i].GetCreationTimestamp()
		second := filtered[j].GetCreationTimestamp()
		if first.Equal(&second) {
			return filtered[i].GetName() < filtered[j].GetName()
		}
		return first.After(second.Time)
	})
	return filtered
}

func canStreamWatchObject(c *gin.Context, resource string, obj *unstructured.Unstructured) bool {
	if !matchesRequestedNamespace(c, obj.GetNamespace()) {
		return false
	}
	user := c.MustGet("user").(model.User)
	cs := c.MustGet("cluster").(*cluster.ClientSet)

	if resource == string(common.Namespaces) {
		return rbac.CanAccessNamespace(user, cs.Name, obj.GetName())
	}
	if c.Param("namespace") == common.AllNamespaces && obj.GetNamespace() != "" {
		if !rbac.CanAccessNamespace(user, cs.Name, obj.GetNamespace()) {
			return false
		}
	}
	return canReadResourceObject(user, resource, cs.Name, obj.GetNamespace(), obj.GetName())
}

func writeSSE(c *gin.Context, event string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.Writer, "event: %s\n", event); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", b); err != nil {
		return err
	}
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}
