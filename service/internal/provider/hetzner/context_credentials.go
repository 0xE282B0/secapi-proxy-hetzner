package hetzner

import (
	"context"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

type workspaceCredentialContextKey struct{}

type WorkspaceCredential struct {
	Token             string
	CloudAPIURL       string
	HetznerPrimaryURL string
}

func WithWorkspaceCredential(ctx context.Context, credential WorkspaceCredential) context.Context {
	if credential.Token == "" {
		return ctx
	}
	return context.WithValue(ctx, workspaceCredentialContextKey{}, credential)
}

func (s *RegionService) clientFor(ctx context.Context) *hcloud.Client {
	cred, ok := workspaceCredentialFromContext(ctx)
	if !ok || cred.Token == "" {
		return s.client
	}

	opts := []hcloud.ClientOption{hcloud.WithToken(cred.Token)}
	if cred.CloudAPIURL != "" {
		opts = append(opts, hcloud.WithEndpoint(cred.CloudAPIURL))
	} else {
		opts = append(opts, hcloud.WithEndpoint(s.cloudAPIURL))
	}
	if cred.HetznerPrimaryURL != "" {
		opts = append(opts, hcloud.WithHetznerEndpoint(cred.HetznerPrimaryURL))
	} else {
		opts = append(opts, hcloud.WithHetznerEndpoint(s.apiURL))
	}
	return hcloud.NewClient(opts...)
}

func workspaceCredentialFromContext(ctx context.Context) (WorkspaceCredential, bool) {
	if ctx == nil {
		return WorkspaceCredential{}, false
	}
	cred, ok := ctx.Value(workspaceCredentialContextKey{}).(WorkspaceCredential)
	return cred, ok
}
