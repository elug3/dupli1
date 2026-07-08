package bootstrap

import (
	"context"
	"fmt"
	"net/http"

	"github.com/elug3/dupli1/product/pkg/authjwt"
	"github.com/elug3/dupli1/product/pkg/handler"
	natsinfra "github.com/elug3/dupli1/product/pkg/infra/nats"
	"github.com/elug3/dupli1/product/pkg/infra/pg"
	s3store "github.com/elug3/dupli1/product/pkg/infra/s3"
	"github.com/elug3/dupli1/product/pkg/middleware"
	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/elug3/dupli1/product/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/permissions"
)

// App holds wired product service dependencies and the HTTP handler.
type App struct {
	Handler       http.Handler
	natsPublisher *natsinfra.Publisher
	close         func() error
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

	var eventPublisher ports.EventPublisher
	var natsPublisher *natsinfra.Publisher
	if cfg.NATSURL != "" {
		natsPublisher, err = natsinfra.NewPublisher(cfg.NATSURL)
		if err != nil {
			store.Close()
			return nil, err
		}
		eventPublisher = natsPublisher
	}

	svc := service.NewProductSearchService(store, imgStore, eventPublisher)
	couponStore, err := pg.NewCouponStore(store.Pool())
	if err != nil {
		store.Close()
		return nil, err
	}
	couponSvc := service.NewCouponService(couponStore)
	h := handler.NewHandler(svc, couponSvc)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	requirePerm := func(perm string, next http.Handler) http.Handler {
		return middleware.RequireAuth(validator, middleware.RequireAnyPermission(perm)(next))
	}

	mux.Handle("GET "+handler.RouteProducts, middleware.OptionalAuth(validator, h.SearchProductsHandler()))
	mux.Handle("POST "+handler.RouteProducts, requirePerm(permissions.ProductCreate, h.CreateProductHandler()))
	mux.Handle("PUT "+handler.RouteProductByID, requirePerm(permissions.ProductUpdate, h.SingleProductHandler()))
	mux.Handle("DELETE "+handler.RouteProductByID, requirePerm(permissions.ProductDelete, h.SingleProductHandler()))
	mux.Handle("POST "+handler.RouteProductImages, requirePerm(permissions.ProductImageUpload, h.UploadImageHandler()))

	mux.Handle("POST "+handler.RouteVariants, requirePerm(permissions.ProductVariantCreate, h.CreateVariantHandler()))
	mux.Handle("PUT "+handler.RouteVariantBySKU, requirePerm(permissions.ProductVariantUpdate, h.VariantBySKUHandler()))
	mux.Handle("DELETE "+handler.RouteVariantBySKU, requirePerm(permissions.ProductVariantDelete, h.VariantBySKUHandler()))
	mux.Handle("POST "+handler.RouteVariantImages, requirePerm(permissions.ProductImageUpload, h.UploadVariantImageHandler()))

	mux.Handle("GET "+handler.RouteCoupons, requirePerm(permissions.CouponRead, http.HandlerFunc(h.ListCoupons)))
	mux.Handle("POST "+handler.RouteCoupons, requirePerm(permissions.CouponCreate, http.HandlerFunc(h.CreateCoupon)))
	mux.Handle("PUT "+handler.RouteCouponByCode, requirePerm(permissions.CouponUpdate, http.HandlerFunc(h.UpdateCoupon)))
	mux.Handle("DELETE "+handler.RouteCouponByCode, requirePerm(permissions.CouponDelete, http.HandlerFunc(h.DeleteCoupon)))

	return &App{
		Handler:       mux,
		natsPublisher: natsPublisher,
		close: func() error {
			if natsPublisher != nil {
				natsPublisher.Close()
			}
			store.Close()
			return nil
		},
	}, nil
}
