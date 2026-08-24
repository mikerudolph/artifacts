package types

// Scope is a repo token permission.
type Scope string

const (
	ScopeRead  Scope = "read"
	ScopeWrite Scope = "write"
)

// ParseScope validates a token scope. Empty defaults to write.
func ParseScope(s string) (Scope, error) {
	switch s {
	case "", string(ScopeWrite):
		return ScopeWrite, nil
	case string(ScopeRead):
		return ScopeRead, nil
	default:
		return "", ErrInvalidScope
	}
}

// TokenState is the lifecycle of a repo token.
type TokenState string

const (
	TokenActive  TokenState = "active"
	TokenExpired TokenState = "expired"
	TokenRevoked TokenState = "revoked"
)

// ParseTokenState validates a stored token state.
func ParseTokenState(s string) (TokenState, error) {
	switch s {
	case string(TokenActive):
		return TokenActive, nil
	case string(TokenExpired):
		return TokenExpired, nil
	case string(TokenRevoked):
		return TokenRevoked, nil
	default:
		return "", ErrInvalidState
	}
}

const tokenStateAll TokenState = "all"

// ParseTokenListState accepts active, expired, revoked, or all. Empty defaults to active.
func ParseTokenListState(s string) (TokenState, error) {
	if s == "all" {
		return tokenStateAll, nil
	}
	if s == "" {
		return TokenActive, nil
	}
	return ParseTokenState(s)
}

// IsTokenStateAll reports whether a list filter includes every state.
func IsTokenStateAll(s TokenState) bool {
	return s == tokenStateAll
}

// RepoStatus is the control-plane lifecycle of a repository.
type RepoStatus string

const (
	RepoReady     RepoStatus = "ready"
	RepoImporting RepoStatus = "importing"
	RepoForking   RepoStatus = "forking"
	RepoDeleting  RepoStatus = "deleting"
)

// ParseRepoStatus validates a repository status.
func ParseRepoStatus(s string) (RepoStatus, error) {
	switch RepoStatus(s) {
	case RepoReady, RepoImporting, RepoForking, RepoDeleting:
		return RepoStatus(s), nil
	default:
		return "", ErrInvalidStatus
	}
}

// Jurisdiction restricts where a namespace's data may live. Empty is unrestricted.
type Jurisdiction string

const (
	JurisdictionEU Jurisdiction = "eu"
	JurisdictionUS Jurisdiction = "us"
)

// ParseJurisdiction validates a jurisdiction. Empty is unrestricted.
func ParseJurisdiction(s string) (Jurisdiction, error) {
	switch s {
	case "":
		return "", nil
	case string(JurisdictionEU), string(JurisdictionUS):
		return Jurisdiction(s), nil
	default:
		return "", ErrInvalidJurisdiction
	}
}

// RepoSortField is a list-repos sort key.
type RepoSortField string

const (
	SortCreatedAt  RepoSortField = "created_at"
	SortUpdatedAt  RepoSortField = "updated_at"
	SortLastPushAt RepoSortField = "last_push_at"
	SortName       RepoSortField = "name"
)

// ParseRepoSortField validates a sort field. Empty defaults to created_at.
func ParseRepoSortField(s string) (RepoSortField, error) {
	switch s {
	case "", string(SortCreatedAt):
		return SortCreatedAt, nil
	case string(SortUpdatedAt), string(SortLastPushAt), string(SortName):
		return RepoSortField(s), nil
	default:
		return "", ErrInvalidSort
	}
}

// SortDirection is asc or desc.
type SortDirection string

const (
	SortAsc  SortDirection = "asc"
	SortDesc SortDirection = "desc"
)

// ParseSortDirection validates a sort direction. Empty defaults to desc.
func ParseSortDirection(s string) (SortDirection, error) {
	switch s {
	case "", string(SortDesc):
		return SortDesc, nil
	case string(SortAsc):
		return SortAsc, nil
	default:
		return "", ErrInvalidSort
	}
}
