package authglue

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/runtime"
	"github.com/hitel00000/mold/storage"
)

// OAuthUser represents verified identity returned from an OAuth provider.
type OAuthUser struct {
	Provider       string `json:"provider"`
	ProviderUserID string `json:"provider_user_id"`
	Email          string `json:"email,omitempty"`
	Name           string `json:"name,omitempty"`
}

// OAuthVerifier is a function that exchanges credentials/codes or verifies ID tokens to return an OAuthUser.
type OAuthVerifier func(ctx context.Context, r *http.Request) (*OAuthUser, error)

// OAuthCallbackHandler handles OAuth callback requests (e.g. `/auth/google/callback`).
// It uses the provided OAuthVerifier to obtain verified provider identity, performs find-or-create
// against the Mold `User` resource (`unique_together: [[provider, provider_user_id]]`), and issues
// a session cookie via app.IssueSessionForUser.
func OAuthCallbackHandler(app *runtime.App, providerName string, verifier OAuthVerifier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		var oauthUser *OAuthUser
		var err error

		if verifier != nil {
			oauthUser, err = verifier(ctx, r)
		} else {
			oauthUser, err = parseOAuthUserFromRequest(r, providerName)
		}

		if err != nil || oauthUser == nil || oauthUser.ProviderUserID == "" {
			msg := "failed to verify oauth identity"
			if err != nil {
				msg = err.Error()
			}
			writeError(w, http.StatusBadRequest, "OAUTH_VERIFICATION_FAILED", msg)
			return
		}

		if oauthUser.Provider == "" {
			oauthUser.Provider = providerName
		}

		// Find or Create User with unique_together: [[provider, provider_user_id]]
		userRec, createdNew, err := findOrCreateUser(ctx, app, oauthUser)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "OAUTH_USER_FAILED", err.Error())
			return
		}

		// Sanitize User record before returning to prevent password or internal fields from leaking
		sanitized, err := app.SanitizeRecord("User", userRec)
		if err != nil {
			sanitized = userRec
		}

		userID, ok := extractUserID(sanitized["id"])
		if !ok {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "unexpected user id type")
			return
		}

		userRole, _ := sanitized["role"].(string)
		if userRole == "" {
			userRole = "user"
		}

		cookieVal, _, err := app.IssueSessionForUser(ctx, userID, userRole)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "SESSION_ISSUE_FAILED", err.Error())
			return
		}

		status := http.StatusOK
		if createdNew {
			status = http.StatusCreated
		}

		w.Header().Set("Set-Cookie", cookieVal)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": sanitized})
	}
}

func findOrCreateUser(ctx context.Context, app *runtime.App, ou *OAuthUser) (record map[string]any, createdNew bool, err error) {
	userResIR := &resource.Resource{
		Name:       "User",
		Table:      "users",
		Timestamps: true,
		SoftDelete: true,
	}

	// 1. Query existing user by provider and provider_user_id
	existingRecs, queryErr := app.Store().List(ctx, userResIR, storage.Query{
		Filter: map[string]any{
			"provider":         ou.Provider,
			"provider_user_id": ou.ProviderUserID,
		},
		Limit: 1,
	})
	if queryErr == nil && len(existingRecs) > 0 {
		return existingRecs[0], false, nil
	}

	// 2. Not found -> create new user record
	payload := map[string]any{
		"provider":         ou.Provider,
		"provider_user_id": ou.ProviderUserID,
	}
	if ou.Email != "" {
		payload["email"] = ou.Email
	}
	if ou.Name != "" {
		payload["name"] = ou.Name
	}

	created, createErr := app.CreateRecord(ctx, "User", payload)
	if createErr != nil {
		// Concurrent request retry query fallback
		retryRecs, retryErr := app.Store().List(ctx, userResIR, storage.Query{
			Filter: map[string]any{
				"provider":         ou.Provider,
				"provider_user_id": ou.ProviderUserID,
			},
			Limit: 1,
		})
		if retryErr == nil && len(retryRecs) > 0 {
			return retryRecs[0], false, nil
		}
		return nil, false, createErr
	}

	return created, true, nil
}

func parseOAuthUserFromRequest(r *http.Request, defaultProvider string) (*OAuthUser, error) {
	var body struct {
		Provider       string `json:"provider"`
		ProviderUserID string `json:"provider_user_id"`
		Email          string `json:"email"`
		Name           string `json:"name"`
		Code           string `json:"code"`
	}

	if r.Body != nil && strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}

	p := body.Provider
	if p == "" {
		p = defaultProvider
	}
	pid := body.ProviderUserID
	if pid == "" {
		pid = r.URL.Query().Get("provider_user_id")
	}
	if pid == "" && body.Code != "" {
		pid = "user_" + body.Code
	}

	if pid == "" {
		return nil, fmt.Errorf("provider_user_id or code required")
	}

	email := body.Email
	if email == "" {
		email = r.URL.Query().Get("email")
	}
	name := body.Name
	if name == "" {
		name = r.URL.Query().Get("name")
	}

	return &OAuthUser{
		Provider:       p,
		ProviderUserID: pid,
		Email:          email,
		Name:           name,
	}, nil
}
