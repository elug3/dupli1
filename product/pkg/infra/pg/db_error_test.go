package pg

import (
	"errors"
	"fmt"
	"testing"

	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
)

func TestWrapDBNoRows(t *testing.T) {
	err := wrapDB("get product", pgx.ErrNoRows)
	if !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if got := err.Error(); got != "get product: not found" {
		t.Fatalf("want wrapped message, got %q", got)
	}
}

func TestWrapDBUniqueViolation(t *testing.T) {
	err := wrapDB("create coupon", &pgconn.PgError{Code: "23505", Message: "duplicate key"})
	if !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
}

func TestWrapDBFKViolation(t *testing.T) {
	err := wrapDB("create variant", &pgconn.PgError{Code: "23503", Message: "fk"})
	if !errors.Is(err, ports.ErrInvalid) {
		t.Fatalf("want ErrInvalid, got %v", err)
	}
}

func TestWrapDBUnexpectedKeepsCause(t *testing.T) {
	cause := errors.New("connection reset")
	err := wrapDB("search products", cause)
	if errors.Is(err, ports.ErrNotFound) || errors.Is(err, ports.ErrConflict) || errors.Is(err, ports.ErrInvalid) {
		t.Fatalf("unexpected sentinel classification: %v", err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("want cause preserved, got %v", err)
	}
	if got := fmt.Sprintf("%v", err); got != "search products: connection reset" {
		t.Fatalf("want op prefix, got %q", got)
	}
}
