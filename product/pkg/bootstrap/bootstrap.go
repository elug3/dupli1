package bootstrap

import (
	"context"
	"fmt"
	"net/http"

	"github.com/elug3/schick/product/pkg/authjwt"
	"github.com/elug3/schick/product/pkg/handler"
	"github.com/elug3/schick/product/pkg/infra/pg"
	s3store "github.com/elug3/schick/product/pkg/infra/s3"
	"github.com/elug3/schick/product/pkg/middleware"
	"github.com/elug3/schick/product/pkg/ports"
	"github.com/elug3/schick/product/pkg/service"
)

// App holds wired product service dependencies and the HTTP handler.
type App struct {
	Handler http.Handler
	close   func() error
}

// Close releases infrastructure resources opened during bootstrap.
func (a *App) Close() error {
	if a == nil || a.close == nil {
		return nil
	}
	return a.close()
}

// Bootstrap wires infrastructure, service, handler, and HTTP routes.
func Bootstrap(_ context.Context, cfg Config) (*App, error) {
	store, err := pg.NewProductStore(cfg.DatabaseConnString)
	if err != nil {
		return nil, err
	}

	var imgStore ports.ImageStore
	if cfg.S3Endpoint != "" {
		imgStore, err = s3store.NewImageStore(cfg.S3Endpoint, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket)
		if err != nil {
			store.Close()
			return nil, err
		}
	}

	validator, err := authjwt.NewAccessTokenValidator(cfg.JWKSURL, cfg.JWTSecret)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("auth validator: %w", err)
	}

	svc := service.NewProductSearchService(store, imgStore)
	couponSvc := service.NewCouponService()
	h := handler.NewHandler(svc, couponSvc)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	auth := func(next http.Handler) http.Handler {
		return middleware.RequireAuth(validator, next)
	}

	mux.Handle("GET "+handler.RouteProducts, auth(h.ListProductsHandler()))
	mux.Handle("POST "+handler.RouteProducts, auth(h.CreateProductHandler()))
	mux.Handle("GET "+handler.RouteManageProduct, auth(h.GetProductHandler()))
	mux.Handle("PUT "+handler.RouteProductByID, auth(h.SingleProductHandler()))
	mux.Handle("DELETE "+handler.RouteProductByID, auth(h.SingleProductHandler()))

	mux.Handle("PUT "+handler.RouteProductImage, auth(h.UploadImageHandler()))

	mux.Handle("GET "+handler.RouteCoupons, auth(http.HandlerFunc(h.ListCoupons)))
	mux.Handle("POST "+handler.RouteCoupons, auth(http.HandlerFunc(h.CreateCoupon)))
	mux.Handle("PUT "+handler.RouteCouponByCode, auth(http.HandlerFunc(h.UpdateCoupon)))
	mux.Handle("DELETE "+handler.RouteCouponByCode, auth(http.HandlerFunc(h.DeleteCoupon)))

	return &App{
		Handler: mux,
		close: func() error {
			store.Close()
			return nil
		},
	}, nil
}
