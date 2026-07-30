package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestLargeTextFieldMappings(t *testing.T) {
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8mb4&parseTime=True&loc=Local",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		DryRun:                                   true,
		DisableAutomaticPing:                     true,
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("open dry-run MySQL dialector: %v", err)
	}

	tests := []struct {
		model  interface{}
		fields []string
	}{
		{PendingSession{}, []string{"SystemPrompt", "OpenAIMessages", "AnthropicMessages", "ToolCallArgs"}},
		{ResourceHistory{}, []string{"ResourceYAML", "PreviousYAML"}},
		{ResourceTemplate{}, []string{"YAML"}},
		{User{}, []string{"SidebarPreference"}},
		{GeneralSetting{}, []string{"GlobalSidebarPreference"}},
		{Cluster{}, []string{"Config"}},
	}

	for _, tt := range tests {
		stmt := &gorm.Statement{DB: db}
		if err := stmt.Parse(tt.model); err != nil {
			t.Fatalf("parse %T schema: %v", tt.model, err)
		}
		for _, fieldName := range tt.fields {
			field := stmt.Schema.LookUpField(fieldName)
			if field == nil {
				t.Fatalf("%T field %s not found", tt.model, fieldName)
			}
			if got := db.Dialector.DataTypeOf(field); got != "mediumtext" {
				t.Errorf("%T.%s MySQL type = %q, want mediumtext", tt.model, fieldName, got)
			}
			if got := (postgres.Dialector{}).DataTypeOf(field); got != "text" {
				t.Errorf("%T.%s PostgreSQL type = %q, want text", tt.model, fieldName, got)
			}
			if got := sqlite.Open(":memory:").DataTypeOf(field); got != "text" {
				t.Errorf("%T.%s SQLite type = %q, want text", tt.model, fieldName, got)
			}
		}
	}
}

func TestPendingSessionClusterNameUsesBoundedMySQLType(t *testing.T) {
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8mb4&parseTime=True&loc=Local",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		DryRun:               true,
		DisableAutomaticPing: true,
	})
	if err != nil {
		t.Fatalf("open dry-run MySQL dialector: %v", err)
	}

	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(PendingSession{}); err != nil {
		t.Fatalf("parse PendingSession schema: %v", err)
	}
	field := stmt.Schema.LookUpField("ClusterName")
	if field == nil {
		t.Fatal("PendingSession.ClusterName field not found")
	}
	if got := db.Dialector.DataTypeOf(field); got != "varchar(255)" {
		t.Fatalf("PendingSession.ClusterName MySQL type = %q, want varchar(255)", got)
	}
	if _, indexed := field.TagSettings["INDEX"]; indexed {
		t.Fatal("PendingSession.ClusterName must not create a MySQL index")
	}
}
