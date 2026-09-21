package middleware

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	dbpkg "database"
)

const CustomerIdentityContextKey contextKey = "customer_identity"

// CustomerAuth accepts only short-lived opaque customer sessions. A licence
// key never grants access to orders, invoices or intervention records.
func CustomerAuth(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			parts := strings.Fields(r.Header.Get("Authorization"))
			if len(parts) != 2 || parts[0] != "Bearer" {
				http.Error(w, `{"error":"Authentification client requise"}`, http.StatusUnauthorized)
				return
			}
			identity, err := dbpkg.ValidateCustomerSessionIdentity(db, parts[1])
			if err != nil {
				http.Error(w, `{"error":"Session client invalide ou expirée"}`, http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), CustomerIdentityContextKey, identity)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
