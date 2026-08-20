package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

const (
	authorizationHeader = "Authorization"
	userCtx             = "userId"
	sessionCtx          = "sessionId"
)

func (h *Handler) userIdentityMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeaders := r.Header[authorizationHeader]
		if len(authHeaders) == 0 || authHeaders[0] == "" {
			newErrorResponse(w, http.StatusUnauthorized, "empty auth header", nil)
			return
		}
		header := authHeaders[0]
		headerParts := strings.Split(header, " ")
		if len(headerParts) != 2 || headerParts[0] != "Bearer" {
			newErrorResponse(w, http.StatusUnauthorized, "invalid auth header", nil)
			return
		}

		if len(headerParts[1]) == 0 {
			newErrorResponse(w, http.StatusUnauthorized, "token is empty", nil)
			return
		}

		userId, sessionId, err := h.authService.ParseToken(headerParts[1])
		if err != nil {
			newErrorResponse(w, http.StatusUnauthorized, "invalid token", err)
			return
		}
		ctx := context.WithValue(r.Context(), userCtx, userId)
		ctx = context.WithValue(ctx, sessionCtx, sessionId)

		next(w, r.WithContext(ctx))
	}
}

func getUserId(r *http.Request) (uint, error) {
	id := r.Context().Value(userCtx)

	idUint, ok := id.(uint)
	if !ok {
		return 0, errors.New("user id is of invalid type")
	}

	return idUint, nil
}

func getSessionId(r *http.Request) (uint, error) {
	id := r.Context().Value(userCtx)

	idUint, ok := id.(uint)
	if !ok {
		return 0, errors.New("user id is of invalid type")
	}

	return idUint, nil
}
