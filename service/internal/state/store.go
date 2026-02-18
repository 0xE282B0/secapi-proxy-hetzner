package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	dbsqlc "github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/db/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool       *pgxpool.Pool
	queries    *dbsqlc.Queries
	tokenCodec *tokenCodec
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

type AuthResource struct {
	Tenant          string
	Name            string
	Labels          map[string]string
	Spec            map[string]any
	Status          map[string]any
	ResourceVersion int64
}

type WorkspaceResource struct {
	Tenant          string
	Name            string
	Region          string
	Labels          map[string]string
	Spec            map[string]any
	Status          map[string]any
	ResourceVersion int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type WorkspaceProviderCredential struct {
	Tenant      string
	Workspace   string
	Provider    string
	ProjectRef  string
	APIEndpoint string
	APIToken    string
}

func New(ctx context.Context, databaseURL, credentialsKey string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	codec, err := newTokenCodec(credentialsKey)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("init token codec: %w", err)
	}
	return &Store{pool: pool, queries: dbsqlc.New(pool), tokenCodec: codec}, nil
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

func (s *Store) UpsertRole(ctx context.Context, resource AuthResource) error {
	labelsJSON, err := json.Marshal(resource.Labels)
	if err != nil {
		return fmt.Errorf("marshal role labels: %w", err)
	}
	specJSON, err := json.Marshal(resource.Spec)
	if err != nil {
		return fmt.Errorf("marshal role spec: %w", err)
	}
	statusJSON, err := json.Marshal(resource.Status)
	if err != nil {
		return fmt.Errorf("marshal role status: %w", err)
	}
	_, err = s.queries.UpsertAuthRole(ctx, dbsqlc.UpsertAuthRoleParams{
		Tenant: resource.Tenant,
		Name:   resource.Name,
		Labels: labelsJSON,
		Spec:   specJSON,
		Status: statusJSON,
	})
	if err != nil {
		return fmt.Errorf("upsert role: %w", err)
	}
	return nil
}

func (s *Store) GetRole(ctx context.Context, tenant, name string) (*AuthResource, error) {
	row, err := s.queries.GetAuthRole(ctx, dbsqlc.GetAuthRoleParams{Tenant: tenant, Name: name})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get role: %w", err)
	}
	resource, err := authResourceFromRoleRow(row)
	if err != nil {
		return nil, err
	}
	return &resource, nil
}

func (s *Store) ListRoles(ctx context.Context, tenant string) ([]AuthResource, error) {
	rows, err := s.queries.ListAuthRolesByTenant(ctx, tenant)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	out := make([]AuthResource, 0, len(rows))
	for _, row := range rows {
		resource, convErr := authResourceFromRoleRow(row)
		if convErr != nil {
			return nil, convErr
		}
		out = append(out, resource)
	}
	return out, nil
}

func (s *Store) SoftDeleteRole(ctx context.Context, tenant, name string) (bool, error) {
	count, err := s.queries.SoftDeleteAuthRole(ctx, dbsqlc.SoftDeleteAuthRoleParams{Tenant: tenant, Name: name})
	if err != nil {
		return false, fmt.Errorf("soft delete role: %w", err)
	}
	return count > 0, nil
}

func (s *Store) UpsertRoleAssignment(ctx context.Context, resource AuthResource) error {
	labelsJSON, err := json.Marshal(resource.Labels)
	if err != nil {
		return fmt.Errorf("marshal role assignment labels: %w", err)
	}
	specJSON, err := json.Marshal(resource.Spec)
	if err != nil {
		return fmt.Errorf("marshal role assignment spec: %w", err)
	}
	statusJSON, err := json.Marshal(resource.Status)
	if err != nil {
		return fmt.Errorf("marshal role assignment status: %w", err)
	}
	_, err = s.queries.UpsertAuthRoleAssignment(ctx, dbsqlc.UpsertAuthRoleAssignmentParams{
		Tenant: resource.Tenant,
		Name:   resource.Name,
		Labels: labelsJSON,
		Spec:   specJSON,
		Status: statusJSON,
	})
	if err != nil {
		return fmt.Errorf("upsert role assignment: %w", err)
	}
	return nil
}

func (s *Store) GetRoleAssignment(ctx context.Context, tenant, name string) (*AuthResource, error) {
	row, err := s.queries.GetAuthRoleAssignment(ctx, dbsqlc.GetAuthRoleAssignmentParams{Tenant: tenant, Name: name})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get role assignment: %w", err)
	}
	resource, err := authResourceFromRoleAssignmentRow(row)
	if err != nil {
		return nil, err
	}
	return &resource, nil
}

func (s *Store) ListRoleAssignments(ctx context.Context, tenant string) ([]AuthResource, error) {
	rows, err := s.queries.ListAuthRoleAssignmentsByTenant(ctx, tenant)
	if err != nil {
		return nil, fmt.Errorf("list role assignments: %w", err)
	}
	out := make([]AuthResource, 0, len(rows))
	for _, row := range rows {
		resource, convErr := authResourceFromRoleAssignmentRow(row)
		if convErr != nil {
			return nil, convErr
		}
		out = append(out, resource)
	}
	return out, nil
}

func (s *Store) SoftDeleteRoleAssignment(ctx context.Context, tenant, name string) (bool, error) {
	count, err := s.queries.SoftDeleteAuthRoleAssignment(ctx, dbsqlc.SoftDeleteAuthRoleAssignmentParams{Tenant: tenant, Name: name})
	if err != nil {
		return false, fmt.Errorf("soft delete role assignment: %w", err)
	}
	return count > 0, nil
}

func (s *Store) UpsertWorkspace(ctx context.Context, resource WorkspaceResource) (*WorkspaceResource, error) {
	labelsJSON, err := json.Marshal(resource.Labels)
	if err != nil {
		return nil, fmt.Errorf("marshal workspace labels: %w", err)
	}
	specJSON, err := json.Marshal(resource.Spec)
	if err != nil {
		return nil, fmt.Errorf("marshal workspace spec: %w", err)
	}
	statusJSON, err := json.Marshal(resource.Status)
	if err != nil {
		return nil, fmt.Errorf("marshal workspace status: %w", err)
	}
	row, err := s.queries.UpsertWorkspace(ctx, dbsqlc.UpsertWorkspaceParams{
		Tenant: resource.Tenant,
		Name:   resource.Name,
		Region: resource.Region,
		Labels: labelsJSON,
		Spec:   specJSON,
		Status: statusJSON,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert workspace: %w", err)
	}
	out, err := workspaceResourceFromRow(row)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Store) GetWorkspace(ctx context.Context, tenant, name string) (*WorkspaceResource, error) {
	row, err := s.queries.GetWorkspace(ctx, dbsqlc.GetWorkspaceParams{Tenant: tenant, Name: name})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get workspace: %w", err)
	}
	resource, err := workspaceResourceFromRow(row)
	if err != nil {
		return nil, err
	}
	return &resource, nil
}

func (s *Store) ListWorkspaces(ctx context.Context, tenant string) ([]WorkspaceResource, error) {
	rows, err := s.queries.ListWorkspacesByTenant(ctx, tenant)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	out := make([]WorkspaceResource, 0, len(rows))
	for _, row := range rows {
		resource, convErr := workspaceResourceFromRow(row)
		if convErr != nil {
			return nil, convErr
		}
		out = append(out, resource)
	}
	return out, nil
}

func (s *Store) SoftDeleteWorkspace(ctx context.Context, tenant, name string) (bool, error) {
	count, err := s.queries.SoftDeleteWorkspace(ctx, dbsqlc.SoftDeleteWorkspaceParams{Tenant: tenant, Name: name})
	if err != nil {
		return false, fmt.Errorf("soft delete workspace: %w", err)
	}
	return count > 0, nil
}

func (s *Store) UpsertWorkspaceProviderCredential(ctx context.Context, cred WorkspaceProviderCredential) (*WorkspaceProviderCredential, error) {
	encryptedToken, err := s.tokenCodec.Encrypt(cred.APIToken)
	if err != nil {
		return nil, fmt.Errorf("encrypt workspace provider credential token: %w", err)
	}
	var projectRef pgtype.Text
	if cred.ProjectRef != "" {
		projectRef = pgtype.Text{String: cred.ProjectRef, Valid: true}
	}
	var apiEndpoint pgtype.Text
	if cred.APIEndpoint != "" {
		apiEndpoint = pgtype.Text{String: cred.APIEndpoint, Valid: true}
	}
	row, err := s.queries.UpsertWorkspaceProviderCredential(ctx, dbsqlc.UpsertWorkspaceProviderCredentialParams{
		Tenant:            cred.Tenant,
		Workspace:         cred.Workspace,
		Provider:          cred.Provider,
		ProjectRef:        projectRef,
		ApiEndpoint:       apiEndpoint,
		ApiTokenEncrypted: encryptedToken,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert workspace provider credential: %w", err)
	}
	out, convErr := s.workspaceProviderCredentialFromRow(row)
	if convErr != nil {
		return nil, convErr
	}
	return &out, nil
}

func (s *Store) GetWorkspaceProviderCredential(ctx context.Context, tenant, workspace, provider string) (*WorkspaceProviderCredential, error) {
	row, err := s.queries.GetWorkspaceProviderCredential(ctx, dbsqlc.GetWorkspaceProviderCredentialParams{
		Tenant: tenant, Workspace: workspace, Provider: provider,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get workspace provider credential: %w", err)
	}
	out, convErr := s.workspaceProviderCredentialFromRow(row)
	if convErr != nil {
		return nil, convErr
	}
	return &out, nil
}

func (s *Store) SoftDeleteWorkspaceProviderCredential(ctx context.Context, tenant, workspace, provider string) (bool, error) {
	count, err := s.queries.SoftDeleteWorkspaceProviderCredential(ctx, dbsqlc.SoftDeleteWorkspaceProviderCredentialParams{
		Tenant: tenant, Workspace: workspace, Provider: provider,
	})
	if err != nil {
		return false, fmt.Errorf("soft delete workspace provider credential: %w", err)
	}
	return count > 0, nil
}

func authResourceFromRoleRow(row dbsqlc.AuthRole) (AuthResource, error) {
	labels := map[string]string{}
	if err := json.Unmarshal(row.Labels, &labels); err != nil {
		return AuthResource{}, fmt.Errorf("unmarshal role labels: %w", err)
	}
	spec := map[string]any{}
	if err := json.Unmarshal(row.Spec, &spec); err != nil {
		return AuthResource{}, fmt.Errorf("unmarshal role spec: %w", err)
	}
	status := map[string]any{}
	if err := json.Unmarshal(row.Status, &status); err != nil {
		return AuthResource{}, fmt.Errorf("unmarshal role status: %w", err)
	}
	return AuthResource{
		Tenant:          row.Tenant,
		Name:            row.Name,
		Labels:          labels,
		Spec:            spec,
		Status:          status,
		ResourceVersion: row.ResourceVersion,
	}, nil
}

func authResourceFromRoleAssignmentRow(row dbsqlc.AuthRoleAssignment) (AuthResource, error) {
	labels := map[string]string{}
	if err := json.Unmarshal(row.Labels, &labels); err != nil {
		return AuthResource{}, fmt.Errorf("unmarshal role assignment labels: %w", err)
	}
	spec := map[string]any{}
	if err := json.Unmarshal(row.Spec, &spec); err != nil {
		return AuthResource{}, fmt.Errorf("unmarshal role assignment spec: %w", err)
	}
	status := map[string]any{}
	if err := json.Unmarshal(row.Status, &status); err != nil {
		return AuthResource{}, fmt.Errorf("unmarshal role assignment status: %w", err)
	}
	return AuthResource{
		Tenant:          row.Tenant,
		Name:            row.Name,
		Labels:          labels,
		Spec:            spec,
		Status:          status,
		ResourceVersion: row.ResourceVersion,
	}, nil
}

func workspaceResourceFromRow(row dbsqlc.Workspace) (WorkspaceResource, error) {
	labels := map[string]string{}
	if err := json.Unmarshal(row.Labels, &labels); err != nil {
		return WorkspaceResource{}, fmt.Errorf("unmarshal workspace labels: %w", err)
	}
	spec := map[string]any{}
	if err := json.Unmarshal(row.Spec, &spec); err != nil {
		return WorkspaceResource{}, fmt.Errorf("unmarshal workspace spec: %w", err)
	}
	status := map[string]any{}
	if err := json.Unmarshal(row.Status, &status); err != nil {
		return WorkspaceResource{}, fmt.Errorf("unmarshal workspace status: %w", err)
	}
	return WorkspaceResource{
		Tenant:          row.Tenant,
		Name:            row.Name,
		Region:          row.Region,
		Labels:          labels,
		Spec:            spec,
		Status:          status,
		ResourceVersion: row.ResourceVersion,
		CreatedAt:       row.CreatedAt.Time.UTC(),
		UpdatedAt:       row.UpdatedAt.Time.UTC(),
	}, nil
}

func (s *Store) workspaceProviderCredentialFromRow(row dbsqlc.WorkspaceProviderCredential) (WorkspaceProviderCredential, error) {
	out := WorkspaceProviderCredential{
		Tenant:    row.Tenant,
		Workspace: row.Workspace,
		Provider:  row.Provider,
	}
	token, err := s.tokenCodec.Decrypt(row.ApiTokenEncrypted)
	if err != nil {
		return WorkspaceProviderCredential{}, fmt.Errorf("decrypt workspace provider credential token: %w", err)
	}
	out.APIToken = token
	if row.ProjectRef.Valid {
		out.ProjectRef = row.ProjectRef.String
	}
	if row.ApiEndpoint.Valid {
		out.APIEndpoint = row.ApiEndpoint.String
	}
	return out, nil
}
