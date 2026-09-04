package models

// Agency is the root entity for an agency in mwanachama-backend-git. Each
// deployment is single-tenant, so there is at most one Agency row —
// [gitManager.ensureAgencyEntity] lazily creates it on first use.
type Agency struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}
