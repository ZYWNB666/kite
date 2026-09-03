package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestLocalUserGroupRoleInheritance(t *testing.T) {
	testDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:user-group-%d?mode=memory&cache=shared", time.Now().UnixNano())))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := testDB.AutoMigrate(&User{}, &UserGroup{}, &Role{}, &RoleAssignment{}); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	originalDB := DB
	DB = testDB
	t.Cleanup(func() {
		DB = originalDB
		if sqlDB, dbErr := testDB.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	alice := User{Username: "alice", Provider: "password", Enabled: true}
	bob := User{Username: "bob", Provider: "password", Enabled: true}
	if err := testDB.Create(&alice).Error; err != nil {
		t.Fatalf("create alice: %v", err)
	}
	if err := testDB.Create(&bob).Error; err != nil {
		t.Fatalf("create bob: %v", err)
	}

	group := UserGroup{Name: "platform", Description: "Platform team"}
	if err := CreateUserGroup(&group, []uint{alice.ID}); err != nil {
		t.Fatalf("create group: %v", err)
	}
	role := Role{
		Name:       "platform-viewer",
		Clusters:   []string{"dev"},
		Namespaces: []string{"platform"},
		Resources:  []string{"pods"},
		Verbs:      []string{"get"},
	}
	if err := testDB.Create(&role).Error; err != nil {
		t.Fatalf("create role: %v", err)
	}
	assignment := RoleAssignment{RoleID: role.ID, SubjectType: SubjectTypeLocalGroup, Subject: group.Name}
	if err := testDB.Create(&assignment).Error; err != nil {
		t.Fatalf("create group role assignment: %v", err)
	}

	roles, err := GetUserRolesFromDB("alice")
	if err != nil {
		t.Fatalf("get alice roles: %v", err)
	}
	if len(roles) != 1 || roles[0].Name != role.Name {
		t.Fatalf("alice roles = %#v, want inherited role %q", roles, role.Name)
	}
	roles, err = GetUserRolesFromDB("bob")
	if err != nil {
		t.Fatalf("get bob roles: %v", err)
	}
	if len(roles) != 0 {
		t.Fatalf("bob roles = %#v, want no roles", roles)
	}

	users, total, err := ListUsers(20, 0, "", "id", "asc", role.Name)
	if err != nil {
		t.Fatalf("filter users by inherited role: %v", err)
	}
	if total != 1 || len(users) != 1 || users[0].Username != "alice" {
		t.Fatalf("filtered users = %#v total=%d, want alice only", users, total)
	}

	if err := UpdateUserGroup(&group, "core-platform", group.Description, []uint{alice.ID}); err != nil {
		t.Fatalf("rename group: %v", err)
	}
	var renamedAssignment RoleAssignment
	if err := testDB.First(&renamedAssignment, assignment.ID).Error; err != nil {
		t.Fatalf("load renamed assignment: %v", err)
	}
	if renamedAssignment.Subject != "core-platform" {
		t.Fatalf("assignment subject = %q, want renamed group", renamedAssignment.Subject)
	}

	if err := DeleteUserGroup(&group); err != nil {
		t.Fatalf("delete group: %v", err)
	}
	var assignmentCount int64
	if err := testDB.Model(&RoleAssignment{}).Where("id = ?", assignment.ID).Count(&assignmentCount).Error; err != nil {
		t.Fatalf("count assignments: %v", err)
	}
	if assignmentCount != 0 {
		t.Fatalf("assignment count = %d, want 0", assignmentCount)
	}
}
