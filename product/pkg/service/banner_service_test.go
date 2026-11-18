package service_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/elug3/dupli1/product/pkg/service"
)

type fakeImages struct {
	keys  []string
	types []string
}

func (f *fakeImages) Upload(_ context.Context, key string, r io.Reader, _ int64, contentType string) (string, error) {
	_, _ = io.ReadAll(r)
	f.keys = append(f.keys, key)
	f.types = append(f.types, contentType)
	return "https://cdn.example/" + key, nil
}

func banner(image string) domain.Banner {
	return domain.Banner{ImageURL: image, Title: domain.LocalizedText{Ko: " 겨울 컬렉션 "}, LinkURL: "/category/new"}
}

func TestBannerLifecycle(t *testing.T) {
	ctx := context.Background()
	svc := service.NewBannerService(memory.NewBannerStore(), nil)

	b, err := svc.Create(ctx, banner("/heroes/a.jpg"), "staff@dupli1.com")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if b.ID == "" || b.Placement != domain.BannerPlacementHomeHero || b.Active || b.Title.Ko != "겨울 컬렉션" {
		t.Fatalf("created = %+v", b)
	}
	// A new banner waits until staff show it.
	if live, _ := svc.Live(ctx, ""); len(live) != 0 {
		t.Fatalf("an inactive banner is live: %+v", live)
	}
	if _, err := svc.SetActive(ctx, b.ID, true, "staff"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	second, _ := svc.Create(ctx, banner("/heroes/b.jpg"), "staff")
	second.Active, second.SortOrder = true, 5
	if _, err := svc.Update(ctx, second.ID, *second, "staff"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	live, err := svc.Live(ctx, domain.BannerPlacementHomeHero)
	if err != nil || len(live) != 2 || live[0].ID != b.ID {
		t.Fatalf("live = %+v %v, want a then b", live, err)
	}

	if _, err := svc.SetActive(ctx, b.ID, false, "staff"); err != nil {
		t.Fatal(err)
	}
	if live, _ := svc.Live(ctx, ""); len(live) != 1 || live[0].ID != second.ID {
		t.Fatalf("after taking a down, live = %+v", live)
	}
	if err := svc.Delete(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, second.ID); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("second delete = %v, want not found", err)
	}
	all, _ := svc.List(ctx)
	if len(all) != 1 {
		t.Fatalf("List = %d, want 1", len(all))
	}
}

func TestBannerValidation(t *testing.T) {
	ctx := context.Background()
	svc := service.NewBannerService(memory.NewBannerStore(), nil)
	cases := map[string]domain.Banner{
		"no image":          {},
		"javascript link":   {ImageURL: "/a.jpg", LinkURL: "javascript:alert(1)"},
		"protocol relative": {ImageURL: "/a.jpg", LinkURL: "//evil.example"},
		"http link":         {ImageURL: "/a.jpg", LinkURL: "http://example.com"},
		"bad image":         {ImageURL: "data:image/png;base64,xx"},
		"unknown place":     {ImageURL: "/a.jpg", Placement: "sidebar"},
		"negative order":    {ImageURL: "/a.jpg", SortOrder: -1},
		"long title":        {ImageURL: "/a.jpg", Title: domain.LocalizedText{En: strings.Repeat("x", 121)}},
	}
	for name, b := range cases {
		if _, err := svc.Create(ctx, b, "staff"); !errors.Is(err, ports.ErrInvalid) {
			t.Errorf("%s: err = %v, want invalid", name, err)
		}
	}
	if _, err := svc.Create(ctx, domain.Banner{ImageURL: "https://cdn.example/a.jpg", LinkURL: "https://brand.example/x"}, "staff"); err != nil {
		t.Errorf("https image and link: %v", err)
	}
}

func TestBannerImageUpload(t *testing.T) {
	ctx := context.Background()
	if _, err := service.NewBannerService(memory.NewBannerStore(), nil).UploadImage(ctx, []byte("x")); !errors.Is(err, service.ErrBannerImagesUnavailable) {
		t.Fatalf("no store: %v", err)
	}
	images := &fakeImages{}
	svc := service.NewBannerService(memory.NewBannerStore(), images)
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 32)...)
	url, err := svc.UploadImage(ctx, png)
	if err != nil {
		t.Fatalf("UploadImage: %v", err)
	}
	if !strings.HasPrefix(url, "https://cdn.example/banners/") || !strings.HasSuffix(url, ".png") || images.types[0] != "image/png" {
		t.Fatalf("url %q type %v", url, images.types)
	}
	if _, err := svc.UploadImage(ctx, []byte("<svg onload=alert(1)>")); !errors.Is(err, ports.ErrInvalid) {
		t.Fatalf("svg/text: %v, want invalid", err)
	}
	if _, err := svc.UploadImage(ctx, make([]byte, service.BannerImageMaxBytes+1)); !errors.Is(err, ports.ErrInvalid) {
		t.Fatalf("too large: %v, want invalid", err)
	}
}
