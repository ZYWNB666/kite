package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/common"
)

func runInitCheck() *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/init_check", nil)
	InitCheck(c)
	return recorder
}

func TestInitCheckReturnsErrorWhenUserCountFails(t *testing.T) {
	oldAnonymousUserEnabled := common.AnonymousUserEnabled
	common.AnonymousUserEnabled = false
	defer func() { common.AnonymousUserEnabled = oldAnonymousUserEnabled }()

	oldCountUsers := countUsersForInitCheck
	countUsersForInitCheck = func() (int64, error) {
		return 0, errors.New("database unavailable")
	}
	defer func() { countUsersForInitCheck = oldCountUsers }()

	recorder := runInitCheck()

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if cookie := recorder.Header().Get("Set-Cookie"); cookie != "" {
		t.Fatalf("Set-Cookie = %q, want empty", cookie)
	}
}

func TestInitCheckReturnsErrorWhenClusterCountFails(t *testing.T) {
	oldCountUsers := countUsersForInitCheck
	oldCountClusters := countClustersForInitCheck
	countUsersForInitCheck = func() (int64, error) { return 1, nil }
	countClustersForInitCheck = func() (int64, error) {
		return 0, errors.New("database unavailable")
	}
	defer func() {
		countUsersForInitCheck = oldCountUsers
		countClustersForInitCheck = oldCountClusters
	}()

	recorder := runInitCheck()

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

func TestInitCheckReturnsSingleResponseWhenUserIsMissing(t *testing.T) {
	oldAnonymousUserEnabled := common.AnonymousUserEnabled
	common.AnonymousUserEnabled = false
	defer func() { common.AnonymousUserEnabled = oldAnonymousUserEnabled }()

	oldCountUsers := countUsersForInitCheck
	oldCountClusters := countClustersForInitCheck
	countUsersForInitCheck = func() (int64, error) { return 0, nil }
	countClustersForInitCheck = func() (int64, error) {
		t.Fatal("CountClusters should not be called when no user exists")
		return 0, nil
	}
	defer func() {
		countUsersForInitCheck = oldCountUsers
		countClustersForInitCheck = oldCountClusters
	}()

	recorder := runInitCheck()

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var response struct {
		Initialized bool `json:"initialized"`
		Step        int  `json:"step"`
	}
	decoder := json.NewDecoder(strings.NewReader(recorder.Body.String()))
	if err := decoder.Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		t.Fatal("response contains more than one JSON value")
	}
	if response.Initialized || response.Step != 0 {
		t.Fatalf("response = %+v, want initialized=false step=0", response)
	}
	if cookie := recorder.Header().Get("Set-Cookie"); !strings.Contains(cookie, "auth_token=") {
		t.Fatalf("Set-Cookie = %q, want expired auth_token", cookie)
	}
}
