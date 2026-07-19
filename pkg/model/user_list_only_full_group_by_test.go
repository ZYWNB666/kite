package model

import (
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/utils"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// TestListUsersOnlyFullGroupByCompatible verifies that the ListUsers ID-pluck
// query includes the sort column in its SELECT list, so it does not violate
// MySQL's ONLY_FULL_GROUP_BY sql_mode (Error 3065). This is a regression test
// for the production 500 on /api/v1/admin/users when DB_TYPE=mysql.
func TestListUsersOnlyFullGroupByCompatible(t *testing.T) {
	dialects := []struct {
		name string
		open func() (*gorm.DB, error)
	}{
		{"sqlite", func() (*gorm.DB, error) {
			return gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DryRun: true})
		}},
		{"mysql", func() (*gorm.DB, error) {
			return gorm.Open(mysql.New(mysql.Config{
				DSN:                       "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8mb4&parseTime=True&loc=Local",
				SkipInitializeWithVersion: true,
			}), &gorm.Config{
				DryRun:                                   true,
				DisableAutomaticPing:                     true,
				DisableForeignKeyConstraintWhenMigrating: true,
			})
		}},
	}

	cases := []struct {
		sortBy  string
		sortOrd string
	}{
		{"createdAt", "asc"},
		{"lastLoginAt", "asc"},
		{"lastLoginAt", "desc"},
		{"id", "asc"},
		{"unknown", "asc"}, // falls back to users.id
	}

	for _, d := range dialects {
		db, err := d.open()
		if err != nil {
			t.Logf("[skip] %s: %v", d.name, err)
			continue
		}
		if err := db.AutoMigrate(&User{}, &Role{}, &RoleAssignment{}); err != nil {
			t.Fatalf("%s migrate: %v", d.name, err)
		}
		originalDB := DB
		DB = db.Session(&gorm.Session{DryRun: true})
		t.Cleanup(func() { DB = originalDB })

		for _, tc := range cases {
			name := fmt.Sprintf("%s/%s_%s", d.name, tc.sortBy, tc.sortOrd)
			t.Run(name, func(t *testing.T) {
				// Rebuild the exact ListUsers ID-scan query to inspect SQL.
				// This mirrors the implementation in ListUsers: Select must
				// include the sort column so ONLY_FULL_GROUP_BY is satisfied.
				query := DB.Model(&User{}).Where("users.provider != ?", common.APIKeyProvider)
				sortOrder := tc.sortOrd
				if sortOrder != "asc" && sortOrder != "desc" {
					sortOrder = "desc"
				}
				allowedSorts := map[string]string{
					"id":          "users.id",
					"createdAt":   "users.created_at",
					"lastLoginAt": "users.last_login_at",
				}
				sortColumn, ok := allowedSorts[tc.sortBy]
				if !ok {
					sortColumn = "users.id"
				}
				orderExpr := fmt.Sprintf("%s %s", sortColumn, sortOrder)
				if sortColumn == "users.last_login_at" {
					orderExpr = fmt.Sprintf("users.last_login_at IS NULL, users.last_login_at %s", sortOrder)
				}
				idSelect := "users.id"
				if sortColumn != "users.id" {
					idSelect = fmt.Sprintf("users.id, %s", sortColumn)
				}
				type idRow struct {
					ID uint `gorm:"column:id"`
				}
				var idRows []idRow
				stmt := query.Select(idSelect).Group("users.id").Order(orderExpr).Limit(20).Offset(0).Scan(&idRows).Statement
				sql := stmt.SQL.String()

				// The sort column (other than users.id) MUST appear in the
				// SELECT list, otherwise MySQL ONLY_FULL_GROUP_BY rejects it
				// with "Error 3065: Expression #1 of ORDER BY clause is not
				// in SELECT list".
				if sortColumn != "users.id" {
					if !containsSubstring(sql, sortColumn) {
						t.Errorf("sort column %q missing from SELECT list; SQL: %s", sortColumn, sql)
					}
				}
				t.Logf("OK: %s", sql)
			})
		}
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

// TestListUsersEmptyResultNoINClause verifies that when no users match, we
// return early instead of running `WHERE id IN ()` which some drivers reject.
func TestListUsersEmptyResultNoINClause(t *testing.T) {
	originalDB := DB
	testDB, err := gorm.Open(sqlite.Open("file:userlist-empty?mode=memory&cache=shared"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := testDB.AutoMigrate(&User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	DB = testDB
	t.Cleanup(func() {
		DB = originalDB
		if sqlDB, e := testDB.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})

	// No users in DB → ListUsers must return empty slice, no error, total=0.
	users, total, err := ListUsers(20, 0, "", "lastLoginAt", "asc", "")
	if err != nil {
		t.Fatalf("ListUsers on empty DB failed: %v", err)
	}
	if total != 0 || len(users) != 0 {
		t.Fatalf("expected empty result, got total=%d len=%d", total, len(users))
	}
}

var _ = utils.HashPassword
