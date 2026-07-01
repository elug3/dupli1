package bootstrap

import "testing"

func TestWithPostgresSSLMode(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{
			in:   "postgresql://postgres:password@172.17.0.2:5432",
			want: "postgresql://postgres:password@172.17.0.2:5432?sslmode=require",
		},
		{
			in:   "postgres://dupli1:dupli1_dev@localhost:5432/dupli1_db?sslmode=disable",
			want: "postgres://dupli1:dupli1_dev@localhost:5432/dupli1_db?sslmode=disable",
		},
		{
			in:   "host=localhost user=postgres password=secret",
			want: "host=localhost user=postgres password=secret sslmode=disable",
		},
		{
			in:   "postgres://dupli1:secret@dupli1-production.abc123.us-east-1.rds.amazonaws.com:5432/dupli1_db",
			want: "postgres://dupli1:secret@dupli1-production.abc123.us-east-1.rds.amazonaws.com:5432/dupli1_db?sslmode=require",
		},
		{
			in:   "postgres://dupli1:secret@postgres.dupli1.local:5432/dupli1_db",
			want: "postgres://dupli1:secret@postgres.dupli1.local:5432/dupli1_db?sslmode=disable",
		},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := withPostgresSSLMode(tc.in); got != tc.want {
				t.Fatalf("withPostgresSSLMode(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
