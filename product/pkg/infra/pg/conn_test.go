package pg

import "testing"

func TestWithPostgresSSLMode(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{
			in:   "postgres://dupli1:dupli1_dev@localhost:5432/products?sslmode=disable",
			want: "postgres://dupli1:dupli1_dev@localhost:5432/products?sslmode=disable",
		},
		{
			in:   "postgres://dupli1:secret@dupli1-production.abc123.us-east-1.rds.amazonaws.com:5432/products",
			want: "postgres://dupli1:secret@dupli1-production.abc123.us-east-1.rds.amazonaws.com:5432/products?sslmode=require",
		},
		{
			in:   "host=localhost user=dupli1 password=secret dbname=products",
			want: "host=localhost user=dupli1 password=secret dbname=products sslmode=disable",
		},
	}

	for _, tc := range tests {
		if got := withPostgresSSLMode(tc.in); got != tc.want {
			t.Fatalf("withPostgresSSLMode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
