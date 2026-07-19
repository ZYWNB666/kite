package feishu

import (
	"io"
	"net/http"
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
