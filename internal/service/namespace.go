package service

import (
	"context"

	"github.com/mikerudolph/artifacts/internal/store/meta"
	"github.com/mikerudolph/artifacts/internal/types"
)

func (s *Services) CreateNamespace(ctx context.Context, account types.AccountID, name string, j types.Jurisdiction) (types.Namespace, error) {
	nsName, err := types.ParseNamespaceName(name)
	if err != nil {
		return types.Namespace{}, err
	}
	if _, err := types.ParseJurisdiction(string(j)); err != nil {
		return types.Namespace{}, err
	}
	if err := s.meta.Accounts().Ensure(ctx, account); err != nil {
		return types.Namespace{}, err
	}
	return s.meta.Namespaces().Create(ctx, types.Namespace{
		AccountID:    account,
		Name:         nsName,
		Jurisdiction: j,
		CreatedAt:    s.now(),
	})
}

func (s *Services) GetNamespace(ctx context.Context, account types.AccountID, name string) (types.Namespace, error) {
	nsName, err := types.ParseNamespaceName(name)
	if err != nil {
		return types.Namespace{}, err
	}
	return s.meta.Namespaces().GetByName(ctx, account, nsName)
}

func (s *Services) ListNamespaces(ctx context.Context, account types.AccountID, page types.CursorPage) ([]types.Namespace, types.CursorResult, error) {
	return s.meta.Namespaces().List(ctx, account, page)
}

func (s *Services) ensureNamespace(ctx context.Context, account types.AccountID, name types.NamespaceName) (types.Namespace, error) {
	if err := s.meta.Accounts().Ensure(ctx, account); err != nil {
		return types.Namespace{}, err
	}
	ns, err := s.meta.Namespaces().GetByName(ctx, account, name)
	if err == nil {
		return ns, nil
	}
	if !meta.IsNotFound(err) {
		return types.Namespace{}, err
	}
	return s.meta.Namespaces().Create(ctx, types.Namespace{
		AccountID: account,
		Name:      name,
		CreatedAt: s.now(),
	})
}
