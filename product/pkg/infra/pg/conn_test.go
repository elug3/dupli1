package pg

import "testing"

func TestWithPostgresSSLMode(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{
			in:   "postgres://schick:schick_dev@localhost:5432/products?sslmode=disable",
			want: "postgres://schick:schick_dev@localhost:5432/products?sslmode=disable",
		},
		{
			in:   "postgres://schick:secret@schick-production.abc123.us-east-1.rds.amazonaws.com:5432/products",
			want: "postgres://schick:secret@schick-production.abc123.us-east-1.rds.amazonaws.com:5432/products?sslmode=require",
		},
		{
			in:   "host=localhost user=schick password=secret dbname=products",
			want: "host=localhost user=schick password=secret dbname=products sslmode=disable",
		},
	}

	for _, tc := range tests {
		if got := withPostgresSSLMode(tc.in); got != tc.want {
			t.Fatalf("withPostgresSSLMode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
