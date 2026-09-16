package bootstrap

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/elug3/dupli1/product/pkg/handler"
	natsinfra "github.com/elug3/dupli1/product/pkg/infra/nats"
	"github.com/elug3/dupli1/product/pkg/infra/pg"
	"github.com/elug3/dupli1/product/pkg/infra/ratelimit"
	s3store "github.com/elug3/dupli1/product/pkg/infra/s3"
	"github.com/elug3/dupli1/product/pkg/middleware"
	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/elug3/dupli1/product/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
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
func Bootstrap(ctx context.Context, cfg Config) (*App, error) {
	store, err := pg.NewProductStore(cfg.DatabaseConnString)
	if err != nil {
		return nil, err
	}

	var imgStore ports.ImageStore
	if cfg.S3Endpoint != "" {
		imgStore, err = s3store.NewImageStore(cfg.S3Endpoint, cfg.S3PublicEndpoint, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket)
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
	var natsSubscriber *natsinfra.Subscriber
	if cfg.NATSURL != "" {
		natsPublisher, err = natsinfra.NewPublisher(cfg.NATSURL)
		if err != nil {
			store.Close()
			return nil, err
		}
		eventPublisher = natsPublisher

		natsSubscriber, err = natsinfra.NewSubscriber(cfg.NATSURL)
		if err != nil {
			natsPublisher.Close()
			store.Close()
			return nil, err
		}
	}

	svc := service.NewProductSearchService(store, imgStore, eventPublisher)

	promotionStore, err := pg.NewPromotionStore(store.Pool())
	if err != nil {
		store.Close()
		return nil, err
	}
	promotionSvc := service.NewPromotionService(promotionStore).
		WithLedger(pg.NewPromotionRedemptionStore(store.Pool())).
		WithEntitlements(pg.NewPromotionEntitlementStore(store.Pool()))

	inventoryStore, err := pg.NewInventoryStore(store.Pool())
	if err != nil {
		store.Close()
		return nil, err
	}
	if err := store.SeedMockEcoBag(context.Background(), inventoryStore); err != nil {
		store.Close()
		return nil, fmt.Errorf("seed mock eco-bag: %w", err)
	}
	inventorySvc := service.NewInventoryService(inventoryStore, store)
	svc.WithInventory(inventoryStore)

	catalogStore := pg.NewCatalogStore(store.Pool())
	catalogSvc := service.NewCatalogService(catalogStore)

	guestCookie := handler.GuestCookieConfigFromEnv()
	h := handler.NewHandler(svc, promotionSvc, inventorySvc, catalogSvc).
		WithSettings(BuildSettings(cfg, guestCookie.Enabled)).
		WithViewStore(store).
		WithWishlistStore(store).
		WithGuestCookie(guestCookie)

	// New customers get their welcome promotional code from auth's
	// user.registered event. Issuing is keyed on the event's user id, so a
	// redelivery mints nothing. An unset code disables the subscriber, which
	// is what an environment without the campaign wants.
	welcomeIssuer := service.NewWelcomePromotionIssuer(promotionSvc, cfg.WelcomePromotionCode)
	if natsSubscriber != nil && welcomeIssuer.Enabled() {
		if err := welcomeIssuer.Register(ctx, natsSubscriber); err != nil {
			natsSubscriber.Close()
			natsPublisher.Close()
			store.Close()
			return nil, fmt.Errorf("subscribe welcome promotion issuer: %w", err)
		}
	}

	// Redeem and evaluate are unauthenticated and will answer for any string,
	// which is a free code-guessing oracle; before Phase 2 neither had any
	// limit. The limiter fails open, because it is not what makes a code safe:
	// checkout complete re-evaluates and the ledger caps actual use.
	throttle := newPromotionRateLimiter(cfg).Middleware(customerIDFromRequest)
	h = h.WithPromotionThrottle(throttle)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	requirePerm := func(perm string, next http.Handler) http.Handler {
		return middleware.RequireAuth(validator, middleware.RequireAnyPermission(perm)(next))
	}

	// requireAnyPerm accepts more than one permission name for the same route.
	// The promotion routes use it to also honour the pre-rename coupon.* set,
	// so an access token minted before the rename keeps working until that
	// window closes. See docs/product-promotion-rename.md.
	requireAnyPerm := func(next http.Handler, perms ...string) http.Handler {
		return middleware.RequireAuth(validator, middleware.RequireAnyPermission(perms...)(next))
	}

	mux.Handle("GET "+handler.RouteProducts, middleware.OptionalAuth(validator, h.SearchProductsHandler()))
	mux.Handle("GET "+handler.RoutePublicProduct, middleware.OptionalAuth(validator, h.GetProductHandler()))
	mux.Handle("GET "+handler.RouteWishlist, middleware.OptionalAuth(validator, http.HandlerFunc(h.ListWishlist)))
	mux.Handle("PUT "+handler.RouteProductWishlist, middleware.OptionalAuth(validator, http.HandlerFunc(h.AddWishlist)))
	mux.Handle("POST "+handler.RouteProductWishlist, middleware.OptionalAuth(validator, http.HandlerFunc(h.AddWishlist)))
	mux.Handle("DELETE "+handler.RouteProductWishlist, middleware.OptionalAuth(validator, http.HandlerFunc(h.RemoveWishlist)))
	mux.Handle("POST "+handler.RouteProducts, requirePerm(permissions.ProductCreate, h.CreateProductHandler()))
	mux.Handle("PUT "+handler.RouteProductByID, requirePerm(permissions.ProductUpdate, h.SingleProductHandler()))
	mux.Handle("DELETE "+handler.RouteProductByID, requirePerm(permissions.ProductDelete, h.SingleProductHandler()))
	mux.Handle("POST "+handler.RouteProductImages, requirePerm(permissions.ProductImageUpload, h.UploadImageHandler()))

	mux.Handle("POST "+handler.RouteVariants, requirePerm(permissions.ProductVariantCreate, h.CreateVariantHandler()))
	mux.Handle("PUT "+handler.RouteVariantBySKU, requirePerm(permissions.ProductVariantUpdate, h.VariantBySKUHandler()))
	mux.Handle("DELETE "+handler.RouteVariantBySKU, requirePerm(permissions.ProductVariantDelete, h.VariantBySKUHandler()))
	mux.Handle("POST "+handler.RouteVariantImages, requirePerm(permissions.ProductImageUpload, h.UploadVariantImageHandler()))

	handler.Mount(mux, "GET", handler.RouteCatalogBrands, requirePerm(permissions.ProductMasterRead, http.HandlerFunc(h.ListBrands)), handler.LegacyRouteCatalogBrands)
	handler.Mount(mux, "POST", handler.RouteCatalogBrands, requirePerm(permissions.ProductMasterWrite, http.HandlerFunc(h.CreateBrand)), handler.LegacyRouteCatalogBrands)
	handler.Mount(mux, "PATCH", handler.RouteCatalogBrandByCode, requirePerm(permissions.ProductMasterWrite, http.HandlerFunc(h.UpdateBrand)), handler.LegacyRouteCatalogBrandByCode)
	handler.Mount(mux, "DELETE", handler.RouteCatalogBrandByCode, requirePerm(permissions.ProductMasterWrite, http.HandlerFunc(h.DeleteBrand)), handler.LegacyRouteCatalogBrandByCode)
	handler.Mount(mux, "GET", handler.RouteCatalogStyles, requirePerm(permissions.ProductMasterRead, http.HandlerFunc(h.ListStyles)), handler.LegacyRouteCatalogStyles)
	handler.Mount(mux, "POST", handler.RouteCatalogStyles, requirePerm(permissions.ProductMasterWrite, http.HandlerFunc(h.CreateStyle)), handler.LegacyRouteCatalogStyles)
	handler.Mount(mux, "PATCH", handler.RouteCatalogStyleByCode, requirePerm(permissions.ProductMasterWrite, http.HandlerFunc(h.UpdateStyle)), handler.LegacyRouteCatalogStyleByCode)
	handler.Mount(mux, "DELETE", handler.RouteCatalogStyleByCode, requirePerm(permissions.ProductMasterWrite, http.HandlerFunc(h.DeleteStyle)), handler.LegacyRouteCatalogStyleByCode)
	handler.Mount(mux, "GET", handler.RouteCatalogColors, requirePerm(permissions.ProductMasterRead, http.HandlerFunc(h.ListColors)), handler.LegacyRouteCatalogColors)
	handler.Mount(mux, "POST", handler.RouteCatalogColors, requirePerm(permissions.ProductMasterWrite, http.HandlerFunc(h.CreateColor)), handler.LegacyRouteCatalogColors)
	handler.Mount(mux, "PATCH", handler.RouteCatalogColorByCode, requirePerm(permissions.ProductMasterWrite, http.HandlerFunc(h.UpdateColor)), handler.LegacyRouteCatalogColorByCode)
	handler.Mount(mux, "DELETE", handler.RouteCatalogColorByCode, requirePerm(permissions.ProductMasterWrite, http.HandlerFunc(h.DeleteColor)), handler.LegacyRouteCatalogColorByCode)
	handler.Mount(mux, "GET", handler.RouteCatalogSizes, requirePerm(permissions.ProductMasterRead, http.HandlerFunc(h.ListSizes)), handler.LegacyRouteCatalogSizes)
	handler.Mount(mux, "POST", handler.RouteCatalogSizes, requirePerm(permissions.ProductMasterWrite, http.HandlerFunc(h.CreateSize)), handler.LegacyRouteCatalogSizes)
	handler.Mount(mux, "PATCH", handler.RouteCatalogSizeByCode, requirePerm(permissions.ProductMasterWrite, http.HandlerFunc(h.UpdateSize)), handler.LegacyRouteCatalogSizeByCode)
	handler.Mount(mux, "DELETE", handler.RouteCatalogSizeByCode, requirePerm(permissions.ProductMasterWrite, http.HandlerFunc(h.DeleteSize)), handler.LegacyRouteCatalogSizeByCode)
	handler.Mount(mux, "GET", handler.RouteCatalogEditions, requirePerm(permissions.ProductMasterRead, http.HandlerFunc(h.ListEditions)), handler.LegacyRouteCatalogEditions)
	handler.Mount(mux, "POST", handler.RouteCatalogEditions, requirePerm(permissions.ProductMasterWrite, http.HandlerFunc(h.CreateEdition)), handler.LegacyRouteCatalogEditions)
	handler.Mount(mux, "PATCH", handler.RouteCatalogEditionByCode, requirePerm(permissions.ProductMasterWrite, http.HandlerFunc(h.UpdateEdition)), handler.LegacyRouteCatalogEditionByCode)
	handler.Mount(mux, "DELETE", handler.RouteCatalogEditionByCode, requirePerm(permissions.ProductMasterWrite, http.HandlerFunc(h.DeleteEdition)), handler.LegacyRouteCatalogEditionByCode)

	// Bag merchandising taxonomy — public reads for storefront filter UIs.
	handler.Mount(mux, "GET", handler.RouteCatalogMaster, http.HandlerFunc(h.GetMasterCatalog), handler.LegacyRouteCatalogMaster)
	handler.Mount(mux, "GET", handler.RouteCatalogSubCategories, http.HandlerFunc(h.ListSubCategories), handler.LegacyRouteCatalogSubCategories)
	handler.Mount(mux, "GET", handler.RouteCatalogBagStyles, http.HandlerFunc(h.ListBagStyles), handler.LegacyRouteCatalogBagStyles)
	handler.Mount(mux, "GET", handler.RouteCatalogTargets, http.HandlerFunc(h.ListTargets), handler.LegacyRouteCatalogTargets)

	handler.Mount(mux, "GET", handler.RoutePromotions,
		requireAnyPerm(http.HandlerFunc(h.ListPromotions), permissions.PromotionRead, permissions.CouponRead),
		handler.PreRenameRouteCoupons, handler.LegacyRouteCoupons)
	handler.Mount(mux, "POST", handler.RoutePromotions,
		requireAnyPerm(http.HandlerFunc(h.CreatePromotion), permissions.PromotionCreate, permissions.CouponCreate),
		handler.PreRenameRouteCoupons, handler.LegacyRouteCoupons)
	handler.Mount(mux, "PUT", handler.RoutePromotionByCode,
		requireAnyPerm(http.HandlerFunc(h.UpdatePromotion), permissions.PromotionUpdate, permissions.CouponUpdate),
		handler.PreRenameRouteCouponByCode, handler.LegacyRouteCouponByCode)
	handler.Mount(mux, "DELETE", handler.RoutePromotionByCode,
		requireAnyPerm(http.HandlerFunc(h.DeletePromotion), permissions.PromotionDelete, permissions.CouponDelete),
		handler.PreRenameRouteCouponByCode, handler.LegacyRouteCouponByCode)

	// Redeem and evaluate are unauthenticated and will answer for any string,
	// which is a free code-guessing oracle; before Phase 2 neither had any
	// limit. The limiter fails open, because it is not what makes a code safe:
	// complete re-evaluates and the ledger caps actual use.
	// Evaluating a code is public: a storefront previews the discount before
	// the customer commits, exactly as redeem already was.
	mux.Handle("POST "+handler.RouteEvaluatePromotion, throttle(http.HandlerFunc(h.EvaluatePromotion)))
	// Reserving, consuming and releasing move the usage ledger, so they are
	// service-to-service. Order holds promotion.redeem via its service account.
	mux.Handle("POST "+handler.RouteReservePromotion, requirePerm(permissions.PromotionRedeem, http.HandlerFunc(h.ReservePromotion)))
	mux.Handle("POST "+handler.RouteConsumePromotion, requirePerm(permissions.PromotionRedeem, http.HandlerFunc(h.ConsumePromotion)))
	mux.Handle("POST "+handler.RouteReleasePromotion, requirePerm(permissions.PromotionRedeem, http.HandlerFunc(h.ReleasePromotion)))

	// The wallet is ABAC: any signed-in customer reads their own, and the
	// customer id comes from the token rather than the request. POST because
	// the cart travels in the body to be judged against.
	mux.Handle("POST "+handler.RoutePromotionWallet, middleware.RequireAuth(validator, http.HandlerFunc(h.PromotionWallet)))
	mux.Handle("GET "+handler.RoutePromotionWallet, middleware.RequireAuth(validator, http.HandlerFunc(h.PromotionWallet)))
	mux.Handle("POST "+handler.RoutePromotionIssue, requirePerm(permissions.PromotionIssue, http.HandlerFunc(h.IssuePromotion)))
	mux.Handle("DELETE "+handler.RoutePromotionEntitlement, requirePerm(permissions.PromotionIssue, http.HandlerFunc(h.RevokePromotionEntitlement)))

	handler.Mount(mux, "PUT", handler.RouteInventoryItem, requirePerm(permissions.InventoryStockWrite, h.UpsertInventoryItemHandler()), handler.LegacyRouteInventoryItem)
	handler.Mount(mux, "POST", handler.RouteInventoryAdjust, requirePerm(permissions.InventoryStockWrite, h.AdjustInventoryItemHandler()), handler.LegacyRouteInventoryAdjust)
	handler.Mount(mux, "PUT", handler.RouteInventoryItemBySkuID, requirePerm(permissions.InventoryStockWrite, h.UpsertInventoryItemBySkuIDHandler()), handler.LegacyRouteInventoryItemBySkuID)
	handler.Mount(mux, "POST", handler.RouteInventoryAdjustBySkuID, requirePerm(permissions.InventoryStockWrite, h.AdjustInventoryItemBySkuIDHandler()), handler.LegacyRouteInventoryAdjustBySkuID)
	handler.Mount(mux, "POST", handler.RouteInventoryReservations, requirePerm(permissions.InventoryReservationManage, h.CreateReservationHandler()), handler.LegacyRouteInventoryReservations)
	handler.Mount(mux, "POST", handler.RouteInventoryReservationCommit, requirePerm(permissions.InventoryReservationManage, h.CommitReservationHandler()), handler.LegacyRouteInventoryReservationCommit)
	handler.Mount(mux, "POST", handler.RouteInventoryReservationRelease, requirePerm(permissions.InventoryReservationManage, h.ReleaseReservationHandler()), handler.LegacyRouteInventoryReservationRelease)

	return &App{
		Handler:       mux,
		natsPublisher: natsPublisher,
		close: func() error {
			if natsSubscriber != nil {
				natsSubscriber.Close()
			}
			if natsPublisher != nil {
				natsPublisher.Close()
			}
			store.Close()
			return nil
		},
	}, nil
}

// newPromotionRateLimiter budgets the public promotional-code endpoints.
//
// Redis when REDIS_URL is set, so the window is shared across tasks and the
// configured budget is the real one; otherwise a per-process window, which
// keeps local dev free of infrastructure at the cost of a budget multiplied by
// the task count. See pkg/infra/ratelimit for why failing open is the right
// default here.
func newPromotionRateLimiter(cfg Config) *ratelimit.Limiter {
	const (
		maxAttempts = 20
		window      = time.Minute
	)
	url := strings.TrimSpace(cfg.RedisURL)
	if url == "" {
		return ratelimit.New(ratelimit.NewMemoryCounter(), maxAttempts, window)
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		log.Printf("promotion rate limit: bad REDIS_URL, falling back to per-process window: %v", err)
		return ratelimit.New(ratelimit.NewMemoryCounter(), maxAttempts, window)
	}
	return ratelimit.New(ratelimit.NewRedisCounter(redis.NewClient(opts), "promo"), maxAttempts, window)
}

// customerIDFromRequest reads the caller's identity when the request happens to
// carry a token. These routes are public, so this is often empty and only the
// IP budget applies.
func customerIDFromRequest(r *http.Request) string {
	if claims, ok := authjwt.FromContext(r.Context()); ok {
		return claims.UserID
	}
	return ""
}
