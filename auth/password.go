package auth

import (
	"fmt"

	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/storage"
	"golang.org/x/crypto/bcrypt"
)

// HashPassword generates a bcrypt hash from a plain text password.
func HashPassword(password string) (string, error) {
	if len([]byte(password)) > 72 {
		return "", fmt.Errorf("password exceeds maximum allowed bcrypt length of 72 bytes")
	}
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(bytes), nil
}

// CheckPasswordHash verifies whether a plain text password matches a bcrypt hash.
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// ProcessPasswordFields hashes plain text password fields in the record after record validation.
func ProcessPasswordFields(res *resource.Resource, record storage.Record) (storage.Record, error) {
	if res == nil || record == nil {
		return record, nil
	}

	processed := make(storage.Record)
	for k, v := range record {
		processed[k] = v
	}

	for _, f := range res.Fields {
		if f.Type == resource.TypePassword {
			if val, exists := processed[f.Name]; exists && val != nil {
				if plainStr, ok := val.(string); ok && plainStr != "" {
					// Hash only if not already a bcrypt hash (bcrypt hashes start with $2a$, $2b$, or $2y$)
					if !isBcryptHash(plainStr) {
						hashed, err := HashPassword(plainStr)
						if err != nil {
							return nil, err
						}
						processed[f.Name] = hashed
					}
				}
			}
		}
	}

	return processed, nil
}

// StripPasswordFields removes password fields from records before serializing to API/View responses.
func StripPasswordFields(res *resource.Resource, rec storage.Record) storage.Record {
	if rec == nil || res == nil {
		return rec
	}

	sanitized := make(storage.Record)
	passwordFields := make(map[string]bool)

	for _, f := range res.Fields {
		if f.Type == resource.TypePassword {
			passwordFields[f.Name] = true
		}
	}

	for k, v := range rec {
		if !passwordFields[k] {
			sanitized[k] = v
		}
	}

	return sanitized
}

// StripPasswordFieldsList applies StripPasswordFields to a list of records.
func StripPasswordFieldsList(res *resource.Resource, records []storage.Record) []storage.Record {
	if records == nil {
		return nil
	}
	sanitized := make([]storage.Record, len(records))
	for i, rec := range records {
		sanitized[i] = StripPasswordFields(res, rec)
	}
	return sanitized
}

func isBcryptHash(s string) bool {
	return len(s) == 60 && (s[:4] == "$2a$" || s[:4] == "$2b$" || s[:4] == "$2y$")
}
