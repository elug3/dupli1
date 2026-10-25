package handler

import (
	"net/http"

	"github.com/elug3/dupli1/payment/pkg/domain"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/shared/pkg/permissions"
)

// methods answers GET /api/v1/payments/methods: which payment methods
// checkout can offer right now. Public like /settings, because the storefront
// asks before anyone signs in; it carries no operator detail beyond the
// shopper-facing message.
func (h *Handler) methods(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	statuses, err := h.svc.MethodStatuses(r.Context())
	if err != nil {
		respondServiceError(w, err)
		return
	}
	// Switches flip at runtime, so a cached answer would keep offering a
	// method after it was blocked.
	w.Header().Set("Cache-Control", "no-store")
	respondJSON(w, http.StatusOK, map[string]any{"methods": statuses})
}

// killSwitchRoutes serves GET /api/v1/payments/kill-switches and
// PUT /api/v1/payments/kill-switches/{key}.
func (h *Handler) killSwitchRoutes(w http.ResponseWriter, r *http.Request, rest []string) {
	claims, _ := authjwt.FromContext(r.Context())
	switch {
	case len(rest) == 0 && r.Method == http.MethodGet:
		if h.jwtValidator != nil && !permissions.CanReadKillSwitches(claims.Permissions) {
			respondError(w, http.StatusForbidden, "forbidden: insufficient permission")
			return
		}
		switches, err := h.svc.ListKillSwitches(r.Context())
		if err != nil {
			respondServiceError(w, err)
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"kill_switches": switches})
	case len(rest) == 1 && r.Method == http.MethodPut:
		if h.jwtValidator != nil && !permissions.CanManageKillSwitches(claims.Permissions) {
			respondError(w, http.StatusForbidden, "forbidden: insufficient permission")
			return
		}
		var req struct {
			Engaged *bool  `json:"engaged"`
			Message string `json:"message"`
			Reason  string `json:"reason"`
		}
		if err := decodeJSON(r, &req); err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		// Required rather than defaulting to false: a body that forgot it
		// would otherwise release a switch someone engaged on purpose.
		if req.Engaged == nil {
			respondError(w, http.StatusBadRequest, "engaged is required")
			return
		}
		sw, err := h.svc.SetKillSwitch(r.Context(), domain.KillSwitchChange{
			Key:       rest[0],
			Engaged:   *req.Engaged,
			Message:   req.Message,
			Reason:    req.Reason,
			UpdatedBy: operatorName(claims),
		})
		if err != nil {
			respondServiceError(w, err)
			return
		}
		respondJSON(w, http.StatusOK, sw)
	case len(rest) <= 1:
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
	default:
		respondError(w, http.StatusNotFound, "not found")
	}
}

// operatorName is who the console shows as having flipped a switch: the
// email when the token carries one, which reads better than a user id.
func operatorName(claims authjwt.Claims) string {
	if claims.Email != "" {
		return claims.Email
	}
	return claims.UserID
}
