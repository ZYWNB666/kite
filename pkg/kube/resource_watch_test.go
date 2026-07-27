package kube

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestResourceWatchHubSharesSnapshotAndIncrementalEvents(t *testing.T) {
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "services"}
	service := newWatchTestObject("Service", "default", "demo", "uid-1", "10")
	client := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvr: "ServiceList"},
		service,
	)
	fakeWatcher := watch.NewRaceFreeFake()
	defer fakeWatcher.Stop()

	var watchCalls atomic.Int32
	client.PrependWatchReactor("services", func(action ktesting.Action) (bool, watch.Interface, error) {
		watchCalls.Add(1)
		return true, fakeWatcher, nil
	})

	hub := NewResourceWatchHub(client)
	defer hub.Close()

	first, err := hub.Subscribe(ResourceWatchOptions{
		GVR:       gvr,
		Namespace: "default",
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer first.Close()

	snapshot := waitForResourceWatchEvent(t, first.Events, ResourceWatchSnapshot)
	if len(snapshot.Items) != 1 || snapshot.Items[0].GetName() != "demo" {
		t.Fatalf("snapshot = %#v, want service demo", snapshot.Items)
	}
	_ = waitForResourceWatchEvent(t, first.Events, ResourceWatchReady)

	second, err := hub.Subscribe(ResourceWatchOptions{
		GVR:       gvr,
		Namespace: "default",
	})
	if err != nil {
		t.Fatalf("second Subscribe() error = %v", err)
	}
	defer second.Close()

	secondSnapshot := waitForResourceWatchEvent(t, second.Events, ResourceWatchSnapshot)
	if len(secondSnapshot.Items) != 1 || secondSnapshot.Items[0].GetName() != "demo" {
		t.Fatalf("second snapshot = %#v, want service demo", secondSnapshot.Items)
	}
	_ = waitForResourceWatchEvent(t, second.Events, ResourceWatchReady)

	if got := watchCalls.Load(); got != 1 {
		t.Fatalf("shared stream started %d watches, want 1", got)
	}

	modified := service.DeepCopy()
	modified.SetResourceVersion("11")
	_ = unstructured.SetNestedField(modified.Object, "LoadBalancer", "spec", "type")
	fakeWatcher.Modify(modified)

	firstModified := waitForResourceWatchEvent(t, first.Events, ResourceWatchModified)
	secondModified := waitForResourceWatchEvent(t, second.Events, ResourceWatchModified)
	if firstModified.Object.GetResourceVersion() != "11" ||
		secondModified.Object.GetResourceVersion() != "11" {
		t.Fatalf(
			"modified resource versions = %q, %q, want 11",
			firstModified.Object.GetResourceVersion(),
			secondModified.Object.GetResourceVersion(),
		)
	}
}

func TestResourceWatchHubReportsPermanentListFailure(t *testing.T) {
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "secrets"}
	client := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvr: "SecretList"},
	)
	client.PrependReactor("list", "secrets", func(action ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(
			schema.GroupResource{Resource: "secrets"},
			"",
			context.DeadlineExceeded,
		)
	})

	hub := NewResourceWatchHub(client)
	defer hub.Close()

	subscription, err := hub.Subscribe(ResourceWatchOptions{GVR: gvr, Namespace: "default"})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer subscription.Close()

	event := waitForResourceWatchEvent(t, subscription.Events, ResourceWatchError)
	if !event.Fatal {
		t.Fatalf("fatal = false, want true for forbidden list")
	}
}

func TestResourceWatchHubDoesNotReportReadyBeforeWatchStarts(t *testing.T) {
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "services"}
	service := newWatchTestObject("Service", "default", "demo", "uid-1", "10")
	client := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvr: "ServiceList"},
		service,
	)
	fakeWatcher := watch.NewRaceFreeFake()
	defer fakeWatcher.Stop()

	watchStarted := make(chan struct{})
	releaseWatch := make(chan struct{})
	var startOnce sync.Once
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseWatch) })
	}
	defer release()

	client.PrependWatchReactor("services", func(action ktesting.Action) (bool, watch.Interface, error) {
		startOnce.Do(func() { close(watchStarted) })
		<-releaseWatch
		return true, fakeWatcher, nil
	})

	hub := NewResourceWatchHub(client)
	defer hub.Close()
	subscription, err := hub.Subscribe(ResourceWatchOptions{
		GVR:       gvr,
		Namespace: "default",
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer subscription.Close()

	_ = waitForResourceWatchEvent(t, subscription.Events, ResourceWatchSnapshot)
	select {
	case <-watchStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("watch did not start")
	}

	select {
	case event := <-subscription.Events:
		if event.Type == ResourceWatchReady {
			t.Fatal("received ready before Kubernetes watch was established")
		}
	case <-time.After(100 * time.Millisecond):
	}

	release()
	_ = waitForResourceWatchEvent(t, subscription.Events, ResourceWatchReady)
}

func TestResourceWatchHubTreatsInvalidSelectorAsPermanent(t *testing.T) {
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "services"}
	client := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvr: "ServiceList"},
	)
	client.PrependReactor("list", "services", func(action ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewBadRequest("invalid label selector")
	})

	hub := NewResourceWatchHub(client)
	defer hub.Close()

	subscription, err := hub.Subscribe(ResourceWatchOptions{
		GVR:           gvr,
		Namespace:     "default",
		LabelSelector: "app=demo",
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer subscription.Close()

	event := waitForResourceWatchEvent(t, subscription.Events, ResourceWatchError)
	if !event.Fatal {
		t.Fatalf("fatal = false, want true for invalid selector")
	}
}

func TestResourceWatchHubRelistsAfterExpiredResourceVersion(t *testing.T) {
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	configMap := newWatchTestObject("ConfigMap", "default", "settings", "uid-1", "10")
	_ = unstructured.SetNestedField(configMap.Object, "old", "data", "version")
	client := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvr: "ConfigMapList"},
		configMap,
	)
	firstWatcher := watch.NewRaceFreeFake()
	secondWatcher := watch.NewRaceFreeFake()
	defer firstWatcher.Stop()
	defer secondWatcher.Stop()

	var watchCalls atomic.Int32
	client.PrependWatchReactor("configmaps", func(action ktesting.Action) (bool, watch.Interface, error) {
		if watchCalls.Add(1) == 1 {
			return true, firstWatcher, nil
		}
		return true, secondWatcher, nil
	})

	hub := NewResourceWatchHub(client)
	defer hub.Close()
	subscription, err := hub.Subscribe(ResourceWatchOptions{
		GVR:       gvr,
		Namespace: "default",
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer subscription.Close()

	_ = waitForResourceWatchEvent(t, subscription.Events, ResourceWatchSnapshot)
	_ = waitForResourceWatchEvent(t, subscription.Events, ResourceWatchReady)

	updated := configMap.DeepCopy()
	updated.SetResourceVersion("20")
	_ = unstructured.SetNestedField(updated.Object, "new", "data", "version")
	if _, err := client.Resource(gvr).Namespace("default").Update(
		context.Background(),
		updated,
		metav1.UpdateOptions{},
	); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	firstWatcher.Error(&metav1.Status{
		Status:  metav1.StatusFailure,
		Reason:  metav1.StatusReasonExpired,
		Code:    410,
		Message: "too old resource version",
	})

	snapshot := waitForResourceWatchEvent(t, subscription.Events, ResourceWatchSnapshot)
	if len(snapshot.Items) != 1 {
		t.Fatalf("relist snapshot has %d items, want 1", len(snapshot.Items))
	}
	version, _, err := unstructured.NestedString(snapshot.Items[0].Object, "data", "version")
	if err != nil || version != "new" {
		t.Fatalf("relist snapshot version = %q, err = %v, want new", version, err)
	}
	_ = waitForResourceWatchEvent(t, subscription.Events, ResourceWatchReady)
	if got := watchCalls.Load(); got < 2 {
		t.Fatalf("watch calls = %d, want a restarted watch", got)
	}
}

func waitForResourceWatchEvent(
	t *testing.T,
	events <-chan ResourceWatchEvent,
	eventType string,
) ResourceWatchEvent {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	for {
		select {
		case event, open := <-events:
			if !open {
				t.Fatalf("watch closed before %q event", eventType)
			}
			if event.Type == eventType {
				return event
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for %q event", eventType)
		}
	}
}

func newWatchTestObject(
	kind string,
	namespace string,
	name string,
	uid types.UID,
	resourceVersion string,
) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       kind,
			"metadata": map[string]any{
				"name":            name,
				"namespace":       namespace,
				"uid":             string(uid),
				"resourceVersion": resourceVersion,
			},
		},
	}
	obj.SetCreationTimestamp(metav1.NewTime(time.Unix(100, 0)))
	return obj
}
