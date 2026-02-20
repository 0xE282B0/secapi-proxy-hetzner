package hetzner

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

type SecurityGroup struct {
	Name      string
	Labels    map[string]string
	Rules     []SecurityGroupRule
	CreatedAt time.Time
}

type SecurityGroupRule struct {
	Direction string
}

type SecurityGroupCreateRequest struct {
	Name   string
	Labels map[string]string
}

func (s *RegionService) ListSecurityGroups(ctx context.Context) ([]SecurityGroup, error) {
	if !s.configured {
		return nil, ErrNotConfigured
	}
	items, err := s.clientFor(ctx).Firewall.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]SecurityGroup, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		out = append(out, securityGroupFromHCloud(item))
	}
	return out, nil
}

func (s *RegionService) GetSecurityGroup(ctx context.Context, name string) (*SecurityGroup, error) {
	if !s.configured {
		return nil, ErrNotConfigured
	}
	item, _, err := s.clientFor(ctx).Firewall.GetByName(ctx, strings.TrimSpace(name))
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, nil
	}
	group := securityGroupFromHCloud(item)
	return &group, nil
}

func (s *RegionService) CreateOrUpdateSecurityGroup(ctx context.Context, req SecurityGroupCreateRequest) (*SecurityGroup, bool, error) {
	if !s.configured {
		return nil, false, ErrNotConfigured
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, false, invalidRequestError("security group name is required")
	}

	existing, _, err := s.clientFor(ctx).Firewall.GetByName(ctx, name)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		updated, _, updateErr := s.clientFor(ctx).Firewall.Update(ctx, existing, hcloud.FirewallUpdateOpts{
			Labels: req.Labels,
		})
		if updateErr != nil {
			return nil, false, updateErr
		}
		group := securityGroupFromHCloud(updated)
		return &group, false, nil
	}

	created, _, err := s.clientFor(ctx).Firewall.Create(ctx, hcloud.FirewallCreateOpts{
		Name:   name,
		Labels: req.Labels,
	})
	if err != nil {
		return nil, false, err
	}
	if created.Firewall == nil {
		return nil, false, fmt.Errorf("hetzner returned empty firewall")
	}
	group := securityGroupFromHCloud(created.Firewall)
	return &group, true, nil
}

func (s *RegionService) DeleteSecurityGroup(ctx context.Context, name string) (bool, error) {
	if !s.configured {
		return false, ErrNotConfigured
	}
	item, _, err := s.clientFor(ctx).Firewall.GetByName(ctx, strings.TrimSpace(name))
	if err != nil {
		return false, err
	}
	if item == nil {
		return false, nil
	}
	_, err = s.clientFor(ctx).Firewall.Delete(ctx, item)
	if err != nil {
		return false, err
	}
	return true, nil
}

func securityGroupFromHCloud(item *hcloud.Firewall) SecurityGroup {
	rules := make([]SecurityGroupRule, 0, len(item.Rules))
	for _, rule := range item.Rules {
		if direction := strings.TrimSpace(string(rule.Direction)); direction != "" {
			rules = append(rules, SecurityGroupRule{
				Direction: strings.ToLower(direction),
			})
		}
	}
	return SecurityGroup{
		Name:      strings.ToLower(strings.TrimSpace(item.Name)),
		Labels:    item.Labels,
		Rules:     rules,
		CreatedAt: item.Created,
	}
}
