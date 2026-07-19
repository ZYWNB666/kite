package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestListUsersFunctionalVerify is a functional (non-DryRun) test that confirms
// ListUsers returns correct results after the ONLY_FULL_GROUP_BY fix, across
// all sort modes and the role-filter + search paths.
func TestListUsersFunctionalVerify(t *testing.T) {
	originalDB := DB
	testDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:userlist-func-%d?mode=memory&cache=shared", time.Now().UnixNano())))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := testDB.AutoMigrate(&User{}, &Role{}, &RoleAssignment{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	DB = testDB
	t.Cleanup(func() {
		DB = originalDB
		if sqlDB, e := testDB.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})

	// Seed users with varied last_login_at (some nil) and created_at.
	t1 := time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 7, 19, 11, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 7, 19, 9, 0, 0, 0, time.UTC)
	users := []User{
		{Username: "u1", Name: "Alice", Provider: "password", LastLoginAt: &t1},
		{Username: "u2", Name: "Bob", Provider: "feishu", LastLoginAt: &t2},
		{Username: "u3", Name: "Carol", Provider: "password"}, // never logged in
		{Username: "u4", Name: "Dave", Provider: "feishu", LastLoginAt: &t3},
	}
	for i := range users {
		if err := DB.Create(&users[i]).Error; err != nil {
			t.Fatalf("create user: %v", err)
		}
	}

	// Assign admin role to u1 and u3.
	role := Role{Name: "admin"}
	if err := DB.Create(&role).Error; err != nil {
		t.Fatalf("create role: %v", err)
	}
	for _, uname := range []string{"u1", "u3"} {
		if err := DB.Create(&RoleAssignment{RoleID: role.ID, Subject: uname, SubjectType: SubjectTypeUser}).Error; err != nil {
			t.Fatalf("create role assignment: %v", err)
		}
	}

	// Note on IS NULL ordering: SQLite treats `x IS NULL` as 0 (false) and
	// `x IS NOT NULL` as 1 (true), so `ORDER BY last_login_at IS NULL, ...`
	// puts non-NULL rows first in ASC. MySQL behaves the same way. We only
	// assert the first element where the expectation is unambiguous.
	tests := []struct {
		name      string
		sortBy    string
		sortOrd   string
		role      string
		search    string
		wantLen   int
		wantFirst string // username of first result, "" to skip
	}{
		{"lastLoginAt asc", "lastLoginAt", "asc", "", "", 4, "u4"},   // earliest non-null login (t3=09:00)
		{"lastLoginAt desc", "lastLoginAt", "desc", "", "", 4, "u2"}, // latest login (t2=11:00)
		// createdAt assertions only check count; the sub-millisecond insert
		// timestamps make first-element ordering flaky across DBs.
		{"createdAt asc", "createdAt", "asc", "", "", 4, ""},
		{"createdAt desc", "createdAt", "desc", "", "", 4, ""},
		{"id asc", "id", "asc", "", "", 4, "u1"},
		{"role=admin lastLoginAt asc", "lastLoginAt", "asc", "admin", "", 2, "u1"}, // u1 has login, u3 nil
		{"search=Bob", "createdAt", "asc", "", "Bob", 1, "u2"},
		{"search=no-match", "createdAt", "asc", "", "zzz", 0, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, total, err := ListUsers(20, 0, tc.search, tc.sortBy, tc.sortOrd, tc.role)
			if err != nil {
				t.Fatalf("ListUsers error: %v", err)
			}
			if int(total) != tc.wantLen {
				t.Fatalf("total = %d, want %d", total, tc.wantLen)
			}
			if len(got) != tc.wantLen {
				t.Fatalf("len = %d, want %d", len(got), tc.wantLen)
			}
			if tc.wantFirst != "" && len(got) > 0 {
				if got[0].Username != tc.wantFirst {
					t.Fatalf("first user = %q, want %q", got[0].Username, tc.wantFirst)
				}
			}
		})
	}
}
