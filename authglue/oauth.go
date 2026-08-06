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

// UnsafeTestStubOAuthVerifier creates a test-only OAuthVerifier that parses JSON body or query parameters.
// ⚠️ WARNING: This verifier performs NO external OAuth token exchange or ID token validation.
// It MUST ONLY be used in test environments or local sandbox mocks!
func UnsafeTestStubOAuthVerifier(defaultProvider string) OAuthVerifier {
	return func(ctx context.Context, r *http.Request) (*OAuthUser, error) {
		return parseOAuthUserFromRequest(r, defaultProvider)
	}
}

// OAuthCallbackHandler handles OAuth callback requests (e.g. `/auth/google/callback`).
// It uses the provided OAuthVerifier to obtain verified provider identity, performs find-or-create
// against the Mold `User` resource (`unique_together: [[provider, provider_user_id]]`), and issues
// a session cookie via app.IssueSessionForUser.
//
// Security Contract:
//   - verifier MUST NOT be nil. Passing a nil verifier is rejected with HTTP 500 (OAUTH_VERIFIER_REQUIRED).
func OAuthCallbackHandler(app *runtime.App, providerName string, verifier OAuthVerifier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if verifier == nil {
			writeError(w, http.StatusInternalServerError, "OAUTH_VERIFIER_REQUIRED", "OAuthCallbackHandler requires a non-nil OAuthVerifier")
			return
		}

		ctx := r.Context()
		oauthUser, err := verifier(ctx, r)

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
			if strings.Contains(err.Error(), "ACCOUNT_LINKING_REQUIRED") {
				writeError(w, http.StatusConflict, "ACCOUNT_LINKING_REQUIRED", "an account with this email already exists; please log in with email and password")
				return
			}
			if strings.Contains(err.Error(), "OAUTH_PROVIDER_CONFLICT") {
				writeError(w, http.StatusConflict, "OAUTH_PROVIDER_CONFLICT", err.Error())
				return
			}
			if strings.Contains(err.Error(), "UNIQUE constraint failed: users.email") {
				writeError(w, http.StatusConflict, "EMAIL_ALREADY_EXISTS", "email is already registered")
				return
			}
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

func getUserResourceIR() *resource.Resource {
	return &resource.Resource{
		Name:       "User",
		Table:      "users",
		Timestamps: true,
		SoftDelete: true,
		Fields: []resource.Field{
			{Name: "id", Type: resource.TypeInt, ClientWritable: false},
			{Name: "email", Type: resource.TypeEmail, Nullable: true, ClientWritable: true},
			{Name: "password", Type: resource.TypePassword, Nullable: true, ClientWritable: true},
			{Name: "name", Type: resource.TypeString, Nullable: true, ClientWritable: true},
			{Name: "provider", Type: resource.TypeString, Nullable: true, ClientWritable: true},
			{Name: "provider_user_id", Type: resource.TypeString, Nullable: true, ClientWritable: true},
			{Name: "role", Type: resource.TypeEnum, Nullable: false, Default: "user", ClientWritable: false},
		},
	}
}

func findOrCreateUser(ctx context.Context, app *runtime.App, ou *OAuthUser) (record map[string]any, createdNew bool, err error) {
	userResIR := getUserResourceIR()

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

	// 2. Query existing user by email to reject unverified auto-linking (Pre-Account Hijacking prevention)
	if ou.Email != "" {
		emailRecs, emailErr := app.Store().List(ctx, userResIR, storage.Query{
			Filter: map[string]any{
				"email": ou.Email,
			},
			Limit: 1,
		})
		if emailErr == nil && len(emailRecs) > 0 {
			existingUser := emailRecs[0]
			existingProvider, _ := existingUser["provider"].(string)

			if existingProvider == "" {
				return nil, false, fmt.Errorf("ACCOUNT_LINKING_REQUIRED: an account with email %s already exists; please log in with email and password", ou.Email)
			}
			return nil, false, fmt.Errorf("OAUTH_PROVIDER_CONFLICT: email %s is already registered with provider '%s'", ou.Email, existingProvider)
		}
	}

	// 3. Not found -> create new user record
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
