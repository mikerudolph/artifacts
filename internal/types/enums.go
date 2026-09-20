package types

type Scope string

const (
	ScopeRead  Scope = "read"
	ScopeWrite Scope = "write"
)

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

type TokenState string

const (
	TokenActive  TokenState = "active"
	TokenExpired TokenState = "expired"
	TokenRevoked TokenState = "revoked"
)

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

func ParseTokenListState(s string) (TokenState, error) {
	if s == "all" {
		return tokenStateAll, nil
	}
	if s == "" {
		return TokenActive, nil
	}
	return ParseTokenState(s)
}

func IsTokenStateAll(s TokenState) bool {
	return s == tokenStateAll
}

type RepoStatus string

const (
	RepoReady     RepoStatus = "ready"
	RepoCreating  RepoStatus = "creating"
	RepoImporting RepoStatus = "importing"
	RepoForking   RepoStatus = "forking"
	RepoDeleting  RepoStatus = "deleting"
	RepoFailed    RepoStatus = "failed"
	RepoDeleted   RepoStatus = "deleted"
)

func ParseRepoStatus(s string) (RepoStatus, error) {
	switch RepoStatus(s) {
	case RepoReady, RepoCreating, RepoImporting, RepoForking, RepoDeleting, RepoFailed, RepoDeleted:
		return RepoStatus(s), nil
	default:
		return "", ErrInvalidStatus
	}
}

type Jurisdiction string

const (
	JurisdictionEU Jurisdiction = "eu"
	JurisdictionUS Jurisdiction = "us"
)

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

type RepoSortField string

const (
	SortCreatedAt  RepoSortField = "created_at"
	SortUpdatedAt  RepoSortField = "updated_at"
	SortLastPushAt RepoSortField = "last_push_at"
	SortName       RepoSortField = "name"
)

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

type SortDirection string

const (
	SortAsc  SortDirection = "asc"
	SortDesc SortDirection = "desc"
)

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
