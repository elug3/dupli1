package handler

import (
	"net/http"

	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/product/pkg/domain"
)

type wishlistMutationResponse struct {
	ProductID     string `json:"productId"`
	Wishlisted    bool   `json:"wishlisted"`
	WishlistCount int64  `json:"wishlistCount"`
}

type wishlistListResponse struct {
	Items []domain.Product `json:"items"`
}

// AddWishlist handles PUT|POST /api/v1/products/{id}/wishlist.
// Owner is JWT sub when authenticated, otherwise the guest cookie.
func (h *Handler) AddWishlist(w http.ResponseWriter, r *http.Request) {
	if h.wishlistStore == nil {
		h.respondError(w, http.StatusServiceUnavailable, "wishlist not available")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		h.respondError(w, http.StatusBadRequest, "missing product id")
		return
	}
	ownerKey, ok := h.resolveWishlistOwner(w, r)
	if !ok {
		return
	}
	if _, err := h.svc.GetPublicProduct(id); err != nil {
		h.respondServiceError(w, err)
		return
	}
	_, count, err := h.wishlistStore.AddWishlist(ownerKey, id)
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, wishlistMutationResponse{
		ProductID:     id,
		Wishlisted:    true,
		WishlistCount: count,
	})
}

// RemoveWishlist handles DELETE /api/v1/products/{id}/wishlist.
func (h *Handler) RemoveWishlist(w http.ResponseWriter, r *http.Request) {
	if h.wishlistStore == nil {
		h.respondError(w, http.StatusServiceUnavailable, "wishlist not available")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		h.respondError(w, http.StatusBadRequest, "missing product id")
		return
	}
	ownerKey, ok := h.resolveWishlistOwner(w, r)
	if !ok {
		return
	}
	_, count, err := h.wishlistStore.RemoveWishlist(ownerKey, id)
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, wishlistMutationResponse{
		ProductID:     id,
		Wishlisted:    false,
		WishlistCount: count,
	})
}

// ListWishlist handles GET /api/v1/products/wishlist.
func (h *Handler) ListWishlist(w http.ResponseWriter, r *http.Request) {
	if h.wishlistStore == nil {
		h.respondError(w, http.StatusServiceUnavailable, "wishlist not available")
		return
	}
	ownerKey, ok := h.resolveWishlistOwner(w, r)
	if !ok {
		return
	}
	ids, err := h.wishlistStore.ListWishlistProductIDs(ownerKey)
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	items := make([]domain.Product, 0, len(ids))
	for _, id := range ids {
		p, err := h.svc.GetPublicProduct(id)
		if err != nil {
			continue // skip deleted/non-public parents
		}
		items = append(items, *p)
	}
	h.respondJSON(w, http.StatusOK, wishlistListResponse{Items: items})
}

func (h *Handler) resolveWishlistOwner(w http.ResponseWriter, r *http.Request) (string, bool) {
	if claims, ok := authjwt.FromContext(r.Context()); ok && claims.UserID != "" {
		return "u:" + claims.UserID, true
	}
	if !h.guestCookie.Enabled {
		h.respondError(w, http.StatusUnauthorized, "authentication required")
		return "", false
	}
	guestID, minted := h.ensureGuestID(r)
	if minted {
		h.setGuestCookie(w, guestID)
	}
	return "g:" + guestID, true
}
