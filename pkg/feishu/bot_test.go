package feishu

import (
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestReplyCardWithMessageID(t *testing.T) {
	originalClient := botHTTPClient
	botHTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || !strings.HasSuffix(req.URL.Path, "/im/v1/messages/root-message/reply") {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"code":0,"msg":"ok","data":{"message_id":"summary-reply"}}`)),
		}, nil
	})}
	t.Cleanup(func() { botHTTPClient = originalClient })

	client := &BotClient{
		AppID:     "app",
		AppSecret: "secret",
		token:     "cached-token",
		tokenExp:  time.Now().Add(time.Hour),
	}
	messageID, err := client.ReplyCardWithMessageID("root-message", map[string]interface{}{"elements": []interface{}{}})
	if err != nil {
		t.Fatalf("ReplyCardWithMessageID() error = %v", err)
	}
	if messageID != "summary-reply" {
		t.Fatalf("ReplyCardWithMessageID() = %q, want summary-reply", messageID)
	}
}

func TestTextMessageIDs(t *testing.T) {
	originalClient := botHTTPClient
	botHTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		messageID := "group-text"
		if strings.HasSuffix(req.URL.Path, "/im/v1/messages/root-message/reply") {
			messageID = "reply-text"
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"code":0,"msg":"ok","data":{"message_id":"` + messageID + `"}}`,
			)),
		}, nil
	})}
	t.Cleanup(func() { botHTTPClient = originalClient })

	client := &BotClient{token: "cached-token", tokenExp: time.Now().Add(time.Hour)}
	replyID, err := client.ReplyTextWithMessageID("root-message", "AI summary failed")
	if err != nil || replyID != "reply-text" {
		t.Fatalf("ReplyTextWithMessageID() = %q, %v", replyID, err)
	}
	groupID, err := client.SendText("group-chat", "message_id missing")
	if err != nil || groupID != "group-text" {
		t.Fatalf("SendText() = %q, %v", groupID, err)
	}
}

func TestBuildResultCardKeepsRequestCardFields(t *testing.T) {
	requestTypes := []RequestCardData{
		{
			RequestType: "full_update",
			ReportLink:  "https://example.com/full-report/42",
		},
		{
			RequestType: "canary_update",
			ReportLink:  "https://example.com/canary-report/42",
		},
		{
			RequestType:     "route_adjust",
			TargetResources: []string{"route-a", "route-b"},
		},
	}
	statuses := []string{"approved", "rejected", "withdrawn", "expired"}

	for _, requestType := range requestTypes {
		requestType := requestType
		t.Run(requestType.RequestType, func(t *testing.T) {
			requestType.RequestID = 42
			requestType.RequesterName = "申请人"
			requestType.Cluster = "yunqiao"
			requestType.Namespace = "envoy-gateway-system"
			requestType.DurationHours = 1
			requestType.RiskLevel = "low"
			requestType.Reason = "测试权限申请模板"
			requestType.ApproverOpenID = "ou_approver"
			requestType.ApproverName = "审批人"

			requestFields := cardFields(t, BuildRequestCardFromData(requestType))
			for _, status := range statuses {
				t.Run(status, func(t *testing.T) {
					resultCard := BuildResultCardFromData(requestType, status, "审批备注")
					resultFields := cardFields(t, resultCard)
					if !reflect.DeepEqual(resultFields, requestFields) {
						t.Fatalf("result card fields changed:\nrequest: %#v\nresult:  %#v", requestFields, resultFields)
					}

					for _, element := range resultCard["elements"].([]interface{}) {
						elementMap, ok := element.(map[string]interface{})
						if ok && elementMap["tag"] == "action" {
							t.Fatal("result card must not keep action buttons")
						}
					}
				})
			}
		})
	}
}

func cardFields(t *testing.T, card map[string]interface{}) []interface{} {
	t.Helper()
	elements, ok := card["elements"].([]interface{})
	if !ok || len(elements) == 0 {
		t.Fatalf("card elements = %#v, want a non-empty slice", card["elements"])
	}
	fieldElement, ok := elements[0].(map[string]interface{})
	if !ok {
		t.Fatalf("first card element = %#v, want map", elements[0])
	}
	fields, ok := fieldElement["fields"].([]interface{})
	if !ok {
		t.Fatalf("first card element fields = %#v, want slice", fieldElement["fields"])
	}
	return fields
}
