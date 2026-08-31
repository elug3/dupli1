package bootstrap

import "testing"

func TestResolveAPIBaseURL_PrefersGateway(t *testing.T) {
	got, err := resolveAPIBaseURL(Config{
		GatewayURL: "http://dupli1-proxy/",
		ProductURL: "http://dupli1-product:8080",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://dupli1-proxy" {
		t.Fatalf("got %q, want gateway without trailing slash", got)
	}
}

func TestResolveAPIBaseURL_FallsBackToProduct(t *testing.T) {
	got, err := resolveAPIBaseURL(Config{ProductURL: "http://localhost:8081/"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://localhost:8081" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveAPIBaseURL_EmptyGatewayDoesNotShadowProduct(t *testing.T) {
	// Regression: a non-empty GatewayURL default (e.g. http://localhost:8080) used to
	// win over DUPLI1_PRODUCT_URL in ECS, so order priced against itself and checkout
	// returned 422 unavailable_items for every sellable SKU.
	got, err := resolveAPIBaseURL(Config{
		GatewayURL: "  ",
		ProductURL: "http://product.dupli1.local:8080",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://product.dupli1.local:8080" {
		t.Fatalf("got %q, want product URL when gateway is blank", got)
	}
}

func TestResolveAPIBaseURL_RequiresBase(t *testing.T) {
	if _, err := resolveAPIBaseURL(Config{}); err == nil {
		t.Fatal("expected error")
	}
}
