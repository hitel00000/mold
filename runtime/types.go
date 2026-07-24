package runtime

import (
	"github.com/hitel00000/mold/transport"
	"github.com/hitel00000/mold/view"
)

// Type aliases to expose a unified public runtime surface so consumers
// do not need to directly import sub-packages like transport or view.

// ErrorEnvelope represents the standard API error response envelope.
type ErrorEnvelope = transport.ErrorEnvelope

// SuccessEnvelope represents the standard API single-item success response envelope.
type SuccessEnvelope = transport.SuccessEnvelope

// ListSuccessEnvelope represents the standard API list response envelope with pagination.
type ListSuccessEnvelope = transport.ListSuccessEnvelope

// PageData represents the data payload injected into custom view templates.
type PageData = view.PageData

// TemplateOverrides represents custom view template overrides registry.
type TemplateOverrides = view.TemplateOverrides
