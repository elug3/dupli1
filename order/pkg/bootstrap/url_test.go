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

func TestResolveAPIBaseURL_RequiresBase(t *testing.T) {
	if _, err := resolveAPIBaseURL(Config{}); err == nil {
		t.Fatal("expected error")
	}
}
