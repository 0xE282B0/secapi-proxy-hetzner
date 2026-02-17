package state

import (
	"context"
	"errors"
	"fmt"

	dbsqlc "github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/db/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool    *pgxpool.Pool
	queries *dbsqlc.Queries
}

type ResourceBinding struct {
	Tenant      string
	Workspace   string
	Kind        string
	SecaRef     string
	ProviderRef string
	Status      string
}

type OperationRecord struct {
	OperationID      string
	SecaRef          string
	ProviderActionID string
	Phase            string
	ErrorText        string
}

func New(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return &Store{pool: pool, queries: dbsqlc.New(pool)}, nil
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) UpsertResourceBinding(ctx context.Context, binding ResourceBinding) error {
	_, err := s.queries.UpsertResourceBinding(ctx, dbsqlc.UpsertResourceBindingParams{
		Tenant:      binding.Tenant,
		Workspace:   binding.Workspace,
		Kind:        binding.Kind,
		SecaRef:     binding.SecaRef,
		ProviderRef: binding.ProviderRef,
		Status:      binding.Status,
	})
	if err != nil {
		return fmt.Errorf("upsert resource binding: %w", err)
	}
	return nil
}

func (s *Store) GetResourceBinding(ctx context.Context, secaRef string) (*ResourceBinding, error) {
	row, err := s.queries.GetResourceBindingBySecaRef(ctx, secaRef)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get resource binding: %w", err)
	}
	return &ResourceBinding{
		Tenant:      row.Tenant,
		Workspace:   row.Workspace,
		Kind:        row.Kind,
		SecaRef:     row.SecaRef,
		ProviderRef: row.ProviderRef,
		Status:      row.Status,
	}, nil
}

func (s *Store) ListResourceBindings(ctx context.Context, tenant, workspace, kind string) ([]ResourceBinding, error) {
	rows, err := s.queries.ListResourceBindingsByScopeAndKind(ctx, dbsqlc.ListResourceBindingsByScopeAndKindParams{
		Tenant: tenant, Workspace: workspace, Kind: kind,
	})
	if err != nil {
		return nil, fmt.Errorf("list resource bindings: %w", err)
	}
	out := make([]ResourceBinding, 0, len(rows))
	for _, row := range rows {
		out = append(out, ResourceBinding{
			Tenant:      row.Tenant,
			Workspace:   row.Workspace,
			Kind:        row.Kind,
			SecaRef:     row.SecaRef,
			ProviderRef: row.ProviderRef,
			Status:      row.Status,
		})
	}
	return out, nil
}

func (s *Store) DeleteResourceBinding(ctx context.Context, secaRef string) error {
	if err := s.queries.DeleteResourceBindingBySecaRef(ctx, secaRef); err != nil {
		return fmt.Errorf("delete resource binding: %w", err)
	}
	return nil
}

func (s *Store) CreateOperation(ctx context.Context, operation OperationRecord) error {
	var providerActionID pgtype.Text
	if operation.ProviderActionID != "" {
		providerActionID = pgtype.Text{String: operation.ProviderActionID, Valid: true}
	}
	var errorText pgtype.Text
	if operation.ErrorText != "" {
		errorText = pgtype.Text{String: operation.ErrorText, Valid: true}
	}
	_, err := s.queries.CreateOperation(ctx, dbsqlc.CreateOperationParams{
		OperationID:      operation.OperationID,
		SecaRef:          operation.SecaRef,
		ProviderActionID: providerActionID,
		Phase:            operation.Phase,
		ErrorText:        errorText,
	})
	if err != nil {
		return fmt.Errorf("create operation: %w", err)
	}
	return nil
}
