package kube

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
)

const (
	ResourceWatchSnapshot = "snapshot"
	ResourceWatchAdded    = "added"
	ResourceWatchModified = "modified"
	ResourceWatchDeleted  = "deleted"
	ResourceWatchReady    = "ready"
	ResourceWatchError    = "watch-error"
)

const (
	resourceWatchBufferSize     = 512
	resourceWatchTimeout        = 10 * time.Minute
	resourceWatchConnectTimeout = 5 * time.Second
	resourceWatchMaxBackoff     = 30 * time.Second
)

var errResourceWatchConnectTimeout = errors.New("resource watch connection timed out")

// ResourceWatchOptions identifies a shareable Kubernetes list-watch stream.
// Different selectors intentionally use different streams so each subscriber
// receives the same collection semantics as a direct Kubernetes API request.
type ResourceWatchOptions struct {
	GVR           schema.GroupVersionResource
	Namespace     string
	LabelSelector string
	FieldSelector string
}

func (o ResourceWatchOptions) key() string {
	return fmt.Sprintf(
		"%s/%s/%s|%s|%s|%s",
		o.GVR.Group,
		o.GVR.Version,
		o.GVR.Resource,
		o.Namespace,
		o.LabelSelector,
		o.FieldSelector,
	)
}

// ResourceWatchEvent is the internal event sent from a shared Kubernetes
// list-watch to an HTTP/SSE subscriber.
type ResourceWatchEvent struct {
	Type            string
	ResourceVersion string
	Items           []*unstructured.Unstructured
	Object          *unstructured.Unstructured
	Error           string
	Fatal           bool
}

// ResourceWatchSubscription represents one consumer of a shared watch.
type ResourceWatchSubscription struct {
	Events <-chan ResourceWatchEvent

	closeOnce sync.Once
	close     func()
}

func (s *ResourceWatchSubscription) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(s.close)
}

// ResourceWatchHub deduplicates Kubernetes watches within one cluster.
// A K8sClient owns one hub, so the key does not need to include cluster ID.
type ResourceWatchHub struct {
	client dynamic.Interface

	mu      sync.Mutex
	closed  bool
	nextID  uint64
	entries map[string]*resourceWatchEntry
}

type resourceWatchEntry struct {
	hub     *ResourceWatchHub
	key     string
	options ResourceWatchOptions
	cancel  context.CancelFunc

	mu          sync.Mutex
	listed      bool
	watching    bool
	version     string
	items       map[string]*unstructured.Unstructured
	subscribers map[uint64]chan ResourceWatchEvent
}

func NewResourceWatchHub(client dynamic.Interface) *ResourceWatchHub {
	return &ResourceWatchHub{
		client:  client,
		entries: make(map[string]*resourceWatchEntry),
	}
}

// Subscribe joins (or starts) a shared list-watch. Once the initial list has
// completed, every subscriber receives a snapshot before any later events.
func (h *ResourceWatchHub) Subscribe(options ResourceWatchOptions) (*ResourceWatchSubscription, error) {
	if h == nil || h.client == nil {
		return nil, errors.New("resource watch client is unavailable")
	}

	key := options.key()
	events := make(chan ResourceWatchEvent, resourceWatchBufferSize)

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil, errors.New("resource watch hub is closed")
	}

	entry, exists := h.entries[key]
	if !exists {
		ctx, cancel := context.WithCancel(context.Background())
		entry = &resourceWatchEntry{
			hub:         h,
			key:         key,
			options:     options,
			cancel:      cancel,
			items:       make(map[string]*unstructured.Unstructured),
			subscribers: make(map[uint64]chan ResourceWatchEvent),
		}
		h.entries[key] = entry
		go entry.run(ctx)
	}

	h.nextID++
	id := h.nextID
	entry.mu.Lock()
	entry.subscribers[id] = events
	if entry.listed {
		events <- entry.snapshotEventLocked()
	}
	if entry.watching {
		events <- ResourceWatchEvent{
			Type:            ResourceWatchReady,
			ResourceVersion: entry.version,
		}
	}
	entry.mu.Unlock()
	h.mu.Unlock()

	return &ResourceWatchSubscription{
		Events: events,
		close: func() {
			h.unsubscribe(key, entry, id)
		},
	}, nil
}

func (h *ResourceWatchHub) unsubscribe(key string, entry *resourceWatchEntry, id uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	current, exists := h.entries[key]
	if !exists || current != entry {
		return
	}

	entry.mu.Lock()
	if subscriber, exists := entry.subscribers[id]; exists {
		delete(entry.subscribers, id)
		close(subscriber)
	}
	empty := len(entry.subscribers) == 0
	entry.mu.Unlock()

	if empty {
		delete(h.entries, key)
		entry.cancel()
	}
}

// Close stops every shared watch and releases all subscribers.
func (h *ResourceWatchHub) Close() {
	if h == nil {
		return
	}

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	entries := make([]*resourceWatchEntry, 0, len(h.entries))
	for _, entry := range h.entries {
		entries = append(entries, entry)
	}
	clear(h.entries)

	for _, entry := range entries {
		entry.cancel()
		entry.mu.Lock()
		for id, subscriber := range entry.subscribers {
			delete(entry.subscribers, id)
			close(subscriber)
		}
		entry.mu.Unlock()
	}
	h.mu.Unlock()
}

func (h *ResourceWatchHub) entryFinished(entry *resourceWatchEntry) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.entries[entry.key] == entry {
		delete(h.entries, entry.key)
	}

	entry.mu.Lock()
	for id, subscriber := range entry.subscribers {
		delete(entry.subscribers, id)
		close(subscriber)
	}
	entry.mu.Unlock()
}

func (e *resourceWatchEntry) run(ctx context.Context) {
	defer e.hub.entryFinished(e)

	resource := e.hub.client.Resource(e.options.GVR)
	var resourceClient dynamic.ResourceInterface
	if e.options.Namespace == "" {
		resourceClient = resource
	} else {
		resourceClient = resource.Namespace(e.options.Namespace)
	}

	listOptions := metav1.ListOptions{
		LabelSelector: e.options.LabelSelector,
		FieldSelector: e.options.FieldSelector,
	}

	needsList := true
	backoff := time.Second

	for {
		if err := ctx.Err(); err != nil {
			return
		}

		if needsList {
			list, err := resourceClient.List(ctx, listOptions)
			if err != nil {
				if e.handleFailure(ctx, fmt.Errorf("list %s: %w", e.options.GVR.String(), err), backoff) {
					return
				}
				backoff = nextResourceWatchBackoff(backoff)
				continue
			}

			e.replaceSnapshot(list)
			needsList = false
			backoff = time.Second
		}

		timeoutSeconds := int64(resourceWatchTimeout / time.Second)
		watchOptions := metav1.ListOptions{
			LabelSelector:       e.options.LabelSelector,
			FieldSelector:       e.options.FieldSelector,
			ResourceVersion:     e.currentVersion(),
			AllowWatchBookmarks: true,
			TimeoutSeconds:      &timeoutSeconds,
		}
		watcher, cancelWatch, err := startResourceWatch(
			ctx,
			resourceClient,
			watchOptions,
			resourceWatchConnectTimeout,
		)
		if err != nil {
			e.setWatching(false)
			if apierrors.IsResourceExpired(err) || apierrors.IsGone(err) {
				needsList = true
				continue
			}
			if errors.Is(err, errResourceWatchConnectTimeout) {
				e.broadcast(ResourceWatchEvent{
					Type:  ResourceWatchError,
					Error: err.Error(),
				})
				continue
			}
			if e.handleFailure(ctx, fmt.Errorf("watch %s: %w", e.options.GVR.String(), err), backoff) {
				return
			}
			backoff = nextResourceWatchBackoff(backoff)
			continue
		}

		e.setWatching(true)
		backoff = time.Second

		relist, stopped := e.consumeWatch(ctx, watcher)
		watcher.Stop()
		cancelWatch()
		e.setWatching(false)
		if stopped {
			return
		}
		if relist {
			needsList = true
			continue
		}

		// A normal timeout or transport close resumes from the last observed
		// resourceVersion. A short delay prevents tight loops with broken
		// aggregated API servers that immediately close watch responses.
		if !sleepResourceWatch(ctx, wait.Jitter(time.Second, 0.2)) {
			return
		}
	}
}

type resourceWatchStartResult struct {
	watcher watch.Interface
	err     error
}

func startResourceWatch(
	ctx context.Context,
	resourceClient dynamic.ResourceInterface,
	options metav1.ListOptions,
	timeout time.Duration,
) (watch.Interface, context.CancelFunc, error) {
	attemptCtx, cancel := context.WithCancel(ctx)
	result := make(chan resourceWatchStartResult)
	go func() {
		watcher, err := resourceClient.Watch(attemptCtx, options)
		select {
		case result <- resourceWatchStartResult{watcher: watcher, err: err}:
		case <-attemptCtx.Done():
			if watcher != nil {
				watcher.Stop()
			}
		}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		cancel()
		return nil, nil, ctx.Err()
	case <-timer.C:
		cancel()
		return nil, nil, fmt.Errorf(
			"%w after %s",
			errResourceWatchConnectTimeout,
			timeout,
		)
	case watchResult := <-result:
		if watchResult.err != nil {
			cancel()
			return nil, nil, watchResult.err
		}
		if watchResult.watcher == nil {
			cancel()
			return nil, nil, errors.New("resource watch returned no watcher")
		}
		return watchResult.watcher, cancel, nil
	}
}

func (e *resourceWatchEntry) consumeWatch(ctx context.Context, watcher watch.Interface) (relist, stopped bool) {
	for {
		select {
		case <-ctx.Done():
			return false, true
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return false, false
			}

			if event.Type == watch.Error {
				err := apierrors.FromObject(event.Object)
				if apierrors.IsResourceExpired(err) || apierrors.IsGone(err) {
					return true, false
				}
				fatal := isPermanentResourceWatchError(err)
				e.broadcast(ResourceWatchEvent{
					Type:  ResourceWatchError,
					Error: err.Error(),
					Fatal: fatal,
				})
				return false, fatal
			}

			obj, ok := event.Object.(*unstructured.Unstructured)
			if !ok || obj == nil {
				klog.Warningf(
					"resource watch %s returned unexpected object %T",
					e.options.GVR.String(),
					event.Object,
				)
				continue
			}

			switch event.Type {
			case watch.Bookmark:
				e.setVersion(obj.GetResourceVersion())
			case watch.Added:
				e.applyObject(ResourceWatchAdded, obj)
			case watch.Modified:
				e.applyObject(ResourceWatchModified, obj)
			case watch.Deleted:
				e.applyObject(ResourceWatchDeleted, obj)
			}
		}
	}
}

func (e *resourceWatchEntry) replaceSnapshot(list *unstructured.UnstructuredList) {
	items := make(map[string]*unstructured.Unstructured, len(list.Items))
	for i := range list.Items {
		obj := list.Items[i].DeepCopy()
		items[resourceWatchObjectKey(obj)] = obj
	}

	e.mu.Lock()
	e.items = items
	e.version = list.GetResourceVersion()
	e.listed = true
	e.broadcastLocked(e.snapshotEventLocked())
	e.mu.Unlock()
}

func (e *resourceWatchEntry) applyObject(eventType string, obj *unstructured.Unstructured) {
	copied := obj.DeepCopy()
	key := resourceWatchObjectKey(copied)

	e.mu.Lock()
	switch eventType {
	case ResourceWatchAdded, ResourceWatchModified:
		e.items[key] = copied
	case ResourceWatchDeleted:
		delete(e.items, key)
	}
	if copied.GetResourceVersion() != "" {
		e.version = copied.GetResourceVersion()
	}
	e.broadcastLocked(ResourceWatchEvent{
		Type:            eventType,
		ResourceVersion: e.version,
		Object:          copied.DeepCopy(),
	})
	e.mu.Unlock()
}

func (e *resourceWatchEntry) setVersion(version string) {
	if version == "" {
		return
	}
	e.mu.Lock()
	e.version = version
	e.mu.Unlock()
}

func (e *resourceWatchEntry) setWatching(watching bool) {
	e.mu.Lock()
	changed := e.watching != watching
	e.watching = watching
	if changed && watching {
		e.broadcastLocked(ResourceWatchEvent{
			Type:            ResourceWatchReady,
			ResourceVersion: e.version,
		})
	}
	e.mu.Unlock()
}

func (e *resourceWatchEntry) currentVersion() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.version
}

func (e *resourceWatchEntry) snapshotEventLocked() ResourceWatchEvent {
	items := make([]*unstructured.Unstructured, 0, len(e.items))
	for _, obj := range e.items {
		items = append(items, obj.DeepCopy())
	}
	return ResourceWatchEvent{
		Type:            ResourceWatchSnapshot,
		ResourceVersion: e.version,
		Items:           items,
	}
}

func (e *resourceWatchEntry) broadcast(event ResourceWatchEvent) {
	e.mu.Lock()
	e.broadcastLocked(event)
	e.mu.Unlock()
}

func (e *resourceWatchEntry) broadcastLocked(event ResourceWatchEvent) {
	removedSubscriber := false
	for id, subscriber := range e.subscribers {
		select {
		case subscriber <- event:
		default:
			// A stalled HTTP client must not block every other subscriber.
			delete(e.subscribers, id)
			close(subscriber)
			removedSubscriber = true
		}
	}
	if removedSubscriber && len(e.subscribers) == 0 {
		e.cancel()
	}
}

func (e *resourceWatchEntry) handleFailure(ctx context.Context, err error, backoff time.Duration) bool {
	fatal := isPermanentResourceWatchError(err)
	e.broadcast(ResourceWatchEvent{
		Type:  ResourceWatchError,
		Error: err.Error(),
		Fatal: fatal,
	})
	if fatal {
		return true
	}
	return !sleepResourceWatch(ctx, wait.Jitter(backoff, 0.2))
}

func resourceWatchObjectKey(obj *unstructured.Unstructured) string {
	if uid := obj.GetUID(); uid != "" {
		return string(uid)
	}
	return obj.GetNamespace() + "/" + obj.GetName()
}

func isPermanentResourceWatchError(err error) bool {
	return apierrors.IsForbidden(err) ||
		apierrors.IsUnauthorized(err) ||
		apierrors.IsNotFound(err) ||
		apierrors.IsMethodNotSupported(err) ||
		apierrors.IsBadRequest(err) ||
		apierrors.IsInvalid(err)
}

func nextResourceWatchBackoff(current time.Duration) time.Duration {
	next := current * 2
	if next > resourceWatchMaxBackoff {
		return resourceWatchMaxBackoff
	}
	return next
}

func sleepResourceWatch(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
