package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/storage"
)

type ActionType string

const (
	ActionCreate ActionType = "create"
	ActionRead   ActionType = "read"
	ActionUpdate ActionType = "update"
	ActionDelete ActionType = "delete"
)

var (
	ErrUnauthorized        = errors.New("authentication required")
	ErrForbidden           = errors.New("permission denied")
	ErrPrivilegeEscalation = errors.New("role field can only be modified by admin users")
)

// Evaluate evaluates permissions for a given request and returns (HTTPStatusCode, Allowed, Error).
// Order of evaluation:
// 1. Check authentication credentials (401 Unauthorized if missing when required)
// 2. Check record existence / soft_delete (404 Not Found if missing or deleted)
// 3. Check permissions, ownership & role privilege escalation (403 Forbidden if mismatched)
func Evaluate(sess *Session, res *resource.Resource, action ActionType, rec storage.Record, payload storage.Record) (int, bool, error) {
	if res == nil {
		return http.StatusOK, true, nil
	}

	permSpec := getPermissionSpec(res, action)

	// 1. Check authentication credentials for non-public actions (401 Unauthorized)
	if permSpec != "public" && permSpec != "" {
		if sess == nil {
			return http.StatusUnauthorized, false, ErrUnauthorized
		}
	}

	// 2. Check role field privilege escalation on User resource payload writes
	if (action == ActionCreate || action == ActionUpdate) && payload != nil {
		if _, attemptsRoleWrite := payload["role"]; attemptsRoleWrite {
			if sess == nil || sess.Role != "admin" {
				return http.StatusForbidden, false, ErrPrivilegeEscalation
			}
		}
	}

	// 3. Check record existence & soft_delete (404 Not Found before ownership disclosure)
	if rec != nil && res.SoftDelete {
		if deletedAt, exists := rec["deleted_at"]; exists && deletedAt != nil {
			return http.StatusNotFound, false, storage.ErrNotFound
		}
	}

	// 4. Check permissions spec: public, authenticated, owner, role:<name>
	switch {
	case permSpec == "" || permSpec == "public":
		return http.StatusOK, true, nil

	case permSpec == "authenticated":
		if sess != nil {
			return http.StatusOK, true, nil
		}
		return http.StatusUnauthorized, false, ErrUnauthorized

	case permSpec == "owner":
		if sess == nil {
			return http.StatusUnauthorized, false, ErrUnauthorized
		}
		if res.Auth == nil || res.Auth.OwnershipField == "" {
			// If no ownership_field defined, fallback to authenticated
			return http.StatusOK, true, nil
		}
		if rec == nil {
			// On Create action, ownership is being assigned
			return http.StatusOK, true, nil
		}
		ownerVal, exists := rec[res.Auth.OwnershipField]
		if !exists || ownerVal == nil {
			return http.StatusOK, true, nil
		}
		if fmt.Sprintf("%v", ownerVal) == fmt.Sprintf("%v", sess.UserID) || sess.Role == "admin" {
			return http.StatusOK, true, nil
		}
		return http.StatusForbidden, false, ErrForbidden

	case strings.HasPrefix(permSpec, "role:"):
		requiredRole := strings.TrimPrefix(permSpec, "role:")
		if sess == nil {
			return http.StatusUnauthorized, false, ErrUnauthorized
		}
		if sess.Role == requiredRole || sess.Role == "admin" {
			return http.StatusOK, true, nil
		}
		return http.StatusForbidden, false, ErrForbidden

	default:
		return http.StatusOK, true, nil
	}
}

// Can is a simplified boolean evaluation wrapper used by View templates and Navigation rendering.
func Can(sess *Session, res *resource.Resource, action ActionType, rec storage.Record) bool {
	_, allowed, _ := Evaluate(sess, res, action, rec, nil)
	return allowed
}

func getPermissionSpec(res *resource.Resource, action ActionType) string {
	if res.Auth == nil {
		return "public"
	}
	switch action {
	case ActionCreate:
		return res.Auth.Permissions.Create
	case ActionRead:
		return res.Auth.Permissions.Read
	case ActionUpdate:
		return res.Auth.Permissions.Update
	case ActionDelete:
		return res.Auth.Permissions.Delete
	default:
		return "public"
	}
}
