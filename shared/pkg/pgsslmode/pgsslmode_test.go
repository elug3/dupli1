package pgsslmode

import "testing"

func TestWithSSLMode(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "explicit sslmode is never overwritten",
			in:   "postgres://dupli1:dupli1_dev@localhost:5432/products?sslmode=disable",
			want: "postgres://dupli1:dupli1_dev@localhost:5432/products?sslmode=disable",
		},
		{
			name: "rds host defaults to require",
			in:   "postgres://dupli1:secret@dupli1-production.abc123.us-east-1.rds.amazonaws.com:5432/products",
			want: "postgres://dupli1:secret@dupli1-production.abc123.us-east-1.rds.amazonaws.com:5432/products?sslmode=require",
		},
		{
			name: "bare ip defaults to require",
			in:   "postgresql://postgres:password@172.17.0.2:5432",
			want: "postgresql://postgres:password@172.17.0.2:5432?sslmode=require",
		},
		{
			name: "key=value dsn on localhost defaults to disable",
			in:   "host=localhost user=dupli1 password=secret dbname=products",
			want: "host=localhost user=dupli1 password=secret dbname=products sslmode=disable",
		},
		{
			name: ".local suffix defaults to disable",
			in:   "postgres://dupli1:secret@postgres.dupli1.local:5432/dupli1_db",
			want: "postgres://dupli1:secret@postgres.dupli1.local:5432/dupli1_db?sslmode=disable",
		},
		// Every docker-compose Postgres service name must resolve to
		// "disable" — this is the list that had drifted per service before
		// this package existed.
		{
			name: "postgres-auth defaults to disable",
			in:   "postgres://dupli1:dupli1_dev@postgres-auth:5432/dupli1_db",
			want: "postgres://dupli1:dupli1_dev@postgres-auth:5432/dupli1_db?sslmode=disable",
		},
		{
			name: "postgres-product defaults to disable",
			in:   "postgres://dupli1:dupli1_dev@postgres-product:5432/products",
			want: "postgres://dupli1:dupli1_dev@postgres-product:5432/products?sslmode=disable",
		},
		{
			name: "postgres-order defaults to disable",
			in:   "postgres://dupli1:dupli1_dev@postgres-order:5432/orders",
			want: "postgres://dupli1:dupli1_dev@postgres-order:5432/orders?sslmode=disable",
		},
		{
			name: "postgres-cart defaults to disable",
			in:   "postgres://dupli1:dupli1_dev@postgres-cart:5432/cart",
			want: "postgres://dupli1:dupli1_dev@postgres-cart:5432/cart?sslmode=disable",
		},
		{
			name: "postgres-payment defaults to disable",
			in:   "postgres://dupli1:dupli1_dev@postgres-payment:5432/payments",
			want: "postgres://dupli1:dupli1_dev@postgres-payment:5432/payments?sslmode=disable",
		},
		{
			name: "postgres-notification defaults to disable",
			in:   "postgres://dupli1:dupli1_dev@postgres-notification:5432/notifications",
			want: "postgres://dupli1:dupli1_dev@postgres-notification:5432/notifications?sslmode=disable",
		},
		{
			name: "unrecognized docker hostname defaults to require, not disable",
			in:   "postgres://dupli1:dupli1_dev@postgres-inventory:5432/inventory",
			want: "postgres://dupli1:dupli1_dev@postgres-inventory:5432/inventory?sslmode=require",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := WithSSLMode(tc.in); got != tc.want {
				t.Fatalf("WithSSLMode(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
