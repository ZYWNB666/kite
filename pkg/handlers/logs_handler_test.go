package handlers

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func newLogsQueryContext(target string) *gin.Context {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, target, nil)
	return ctx
}

func TestParsePodLogOptions(t *testing.T) {
	ctx := newLogsQueryContext("/logs/ns/pod?container=api&tailLines=750&timestamps=false&previous=true&sinceTime=2026-07-15T10%3A20%3A30.123Z")
	options, err := parsePodLogOptions(ctx, false)
	if err != nil {
		t.Fatalf("parsePodLogOptions() error = %v", err)
	}

	if options.Container != "api" || options.Follow || options.Timestamps || !options.Previous {
		t.Fatalf("unexpected options: %#v", options)
	}
	if options.TailLines == nil || *options.TailLines != 750 {
		t.Fatalf("TailLines = %v, want 750", options.TailLines)
	}
	wantTime, _ := time.Parse(time.RFC3339Nano, "2026-07-15T10:20:30.123Z")
	if options.SinceTime == nil || !options.SinceTime.Time.Equal(wantTime) {
		t.Fatalf("SinceTime = %v, want %v", options.SinceTime, wantTime)
	}
}

func TestParsePodLogOptionsRejectsInvalidValues(t *testing.T) {
	tests := []string{
		"?tailLines=-1",
		"?tailLines=0",
		"?tailLines=100001",
		"?timestamps=maybe",
		"?previous=maybe",
		"?sinceSeconds=-1",
		"?sinceSeconds=10&sinceTime=2026-07-15T10%3A20%3A30Z",
		"?sinceTime=not-a-time",
	}
	for _, query := range tests {
		t.Run(query, func(t *testing.T) {
			if _, err := parsePodLogOptions(newLogsQueryContext("/logs/ns/pod"+query), false); err == nil {
				t.Fatalf("parsePodLogOptions(%q) returned nil error", query)
			}
		})
	}
}

func TestParsePodLogOptionsAllowsUnboundedIncrement(t *testing.T) {
	ctx := newLogsQueryContext("/logs/ns/pod?tailLines=-1&sinceTime=2026-07-15T10%3A20%3A30.123Z")
	options, err := parsePodLogOptions(ctx, false)
	if err != nil {
		t.Fatalf("parsePodLogOptions() error = %v", err)
	}
	if options.TailLines != nil {
		t.Fatalf("TailLines = %v, want nil for an incremental request", options.TailLines)
	}
}

func TestParsePodLogOptionsAllowsUnboundedSinceSeconds(t *testing.T) {
	ctx := newLogsQueryContext("/logs/ns/pod?tailLines=-1&sinceSeconds=10")
	options, err := parsePodLogOptions(ctx, false)
	if err != nil {
		t.Fatalf("parsePodLogOptions() error = %v", err)
	}
	if options.TailLines != nil || options.SinceSeconds == nil || *options.SinceSeconds != 10 {
		t.Fatalf("unexpected options: %#v", options)
	}
}

func TestSplitPodLogLines(t *testing.T) {
	want := []string{"first", "second", "", "fourth"}
	got := splitPodLogLines("first\r\nsecond\n\nfourth\n")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitPodLogLines() = %#v, want %#v", got, want)
	}
	if got := splitPodLogLines(""); len(got) != 0 {
		t.Fatalf("splitPodLogLines(empty) = %#v, want empty", got)
	}
}

func TestPrefixPodLogLinePreservesTimestamp(t *testing.T) {
	got := prefixPodLogLine("api-123", "2026-07-15T10:20:30.123Z started", true)
	want := "2026-07-15T10:20:30.123Z [api-123]: started"
	if got != want {
		t.Fatalf("prefixPodLogLine() = %q, want %q", got, want)
	}
	if timestamp, ok := podLogTimestamp(got); !ok || timestamp.IsZero() {
		t.Fatalf("podLogTimestamp(%q) = %v, %v, want a timestamp", got, timestamp, ok)
	}
}
