package main

import (
	"flag"
	"io"
	"testing"

	order "github.com/elug3/dupli1/order/pkg"
)

func configureForTest(t *testing.T) order.ServerOptions {
	t.Helper()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	opts, err := ConfigureOptions(fs, nil)
	if err != nil {
		t.Fatalf("ConfigureOptions: %v", err)
	}
	return opts
}

func TestApplyEnvShippingFeeWon(t *testing.T) {
	t.Setenv("DUPLI1_ORDER_SHIPPING_FEE_WON", "15000")
	t.Setenv("DUPLI1_ORDER_SHIPPING_FEE_CENTS", "0")
	opts := configureForTest(t)
	if opts.ShippingFeeWon != 15000 {
		t.Fatalf("ShippingFeeWon = %d, want 15000 from WON env", opts.ShippingFeeWon)
	}
}

func TestApplyEnvShippingFeeKrwAlias(t *testing.T) {
	t.Setenv("DUPLI1_ORDER_SHIPPING_FEE_WON", "")
	t.Setenv("DUPLI1_ORDER_SHIPPING_FEE_KRW", "12000")
	t.Setenv("DUPLI1_ORDER_SHIPPING_FEE_CENTS", "0")
	opts := configureForTest(t)
	if opts.ShippingFeeWon != 12000 {
		t.Fatalf("ShippingFeeWon = %d, want 12000 from leftover KRW env", opts.ShippingFeeWon)
	}
}

func TestApplyEnvShippingFeeCentsAlias(t *testing.T) {
	t.Setenv("DUPLI1_ORDER_SHIPPING_FEE_WON", "")
	t.Setenv("DUPLI1_ORDER_SHIPPING_FEE_KRW", "")
	t.Setenv("DUPLI1_ORDER_SHIPPING_FEE_CENTS", "0")
	opts := configureForTest(t)
	if opts.ShippingFeeWon != 0 {
		t.Fatalf("ShippingFeeWon = %d, want 0 from CENTS alias (free)", opts.ShippingFeeWon)
	}
}

func TestApplyEnvShippingFeeWonWinsOverCents(t *testing.T) {
	t.Setenv("DUPLI1_ORDER_SHIPPING_FEE_WON", "18000")
	t.Setenv("DUPLI1_ORDER_SHIPPING_FEE_CENTS", "0")
	opts := configureForTest(t)
	if opts.ShippingFeeWon != 18000 {
		t.Fatalf("ShippingFeeWon = %d, want WON to win over CENTS", opts.ShippingFeeWon)
	}
}

func TestApplyEnvShippingFeeInvalidKeepsDefault(t *testing.T) {
	t.Setenv("DUPLI1_ORDER_SHIPPING_FEE_WON", "-1")
	t.Setenv("DUPLI1_ORDER_SHIPPING_FEE_CENTS", "")
	opts := configureForTest(t)
	if opts.ShippingFeeWon != order.DefaultShippingFeeWon {
		t.Fatalf("ShippingFeeWon = %d, want default %d for invalid WON", opts.ShippingFeeWon, order.DefaultShippingFeeWon)
	}
}

func TestApplyEnvShippingFeeUnsetUsesDefault(t *testing.T) {
	t.Setenv("DUPLI1_ORDER_SHIPPING_FEE_WON", "")
	t.Setenv("DUPLI1_ORDER_SHIPPING_FEE_KRW", "")
	t.Setenv("DUPLI1_ORDER_SHIPPING_FEE_CENTS", "")
	opts := configureForTest(t)
	if opts.ShippingFeeWon != order.DefaultShippingFeeWon {
		t.Fatalf("ShippingFeeWon = %d, want default %d when unset", opts.ShippingFeeWon, order.DefaultShippingFeeWon)
	}
}
