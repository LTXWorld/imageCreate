package database

import "testing"

func TestIsDisposableTestDatabaseName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want bool
	}{
		{name: "test", want: true},
		{name: "test_app", want: true},
		{name: "app_test", want: true},
		{name: "app_test_db", want: true},
		{name: "APP_TEST_DB", want: true},
		{name: "latest", want: false},
		{name: "contest", want: false},
		{name: "attestation", want: false},
		{name: "production", want: false},
		{name: "mytestdb", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isDisposableTestDatabaseName(tt.name); got != tt.want {
				t.Fatalf("isDisposableTestDatabaseName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
