package erapay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

type ProvisioningClient struct {
	baseURL string
	token   string
	http    *http.Client
}

type remoteTenantLink struct {
	Linked         bool      `json:"linked"`
	TenantID       uuid.UUID `json:"tenant_id"`
	MerchantID     uuid.UUID `json:"merchant_id"`
	MerchantStatus string    `json:"merchant_status"`
	OwnerRole      string    `json:"owner_role"`
	Created        bool      `json:"created"`
}

type EcosystemStatus struct {
	Active         bool   `json:"active"`
	MerchantStatus string `json:"merchant_status,omitempty"`
	OwnerRole      string `json:"owner_role,omitempty"`
}

func NewProvisioningClient(baseURL, token string) *ProvisioningClient {
	baseURL, token = strings.TrimRight(strings.TrimSpace(baseURL), "/"), strings.TrimSpace(token)
	if baseURL == "" || token == "" {
		return nil
	}
	return &ProvisioningClient{baseURL: baseURL, token: token, http: &http.Client{Timeout: 12 * time.Second}}
}

func (c *ProvisioningClient) Status(ctx context.Context, tenantID uuid.UUID) (remoteTenantLink, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/v1/integrations/arus/tenants/%s", c.baseURL, tenantID), nil)
	if err != nil {
		return remoteTenantLink{}, ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return remoteTenantLink{}, ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return remoteTenantLink{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return remoteTenantLink{}, ErrUnavailable
	}
	var link remoteTenantLink
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&link); err != nil {
		return remoteTenantLink{}, ErrUnavailable
	}
	return link, nil
}

func (c *ProvisioningClient) Ensure(ctx context.Context, tenantID uuid.UUID, business, owner, email, phone string) (remoteTenantLink, error) {
	body, err := json.Marshal(map[string]any{
		"tenant_id": tenantID, "business_name": business, "owner_name": owner,
		"owner_email": email, "owner_phone": phone, "accepted_terms": true,
	})
	if err != nil {
		return remoteTenantLink{}, ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/integrations/arus/tenants", bytes.NewReader(body))
	if err != nil {
		return remoteTenantLink{}, ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return remoteTenantLink{}, ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusConflict {
			var envelope struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&envelope) == nil && envelope.Error.Code == "MERCHANT_LINK_AMBIGUOUS" {
				return remoteTenantLink{}, ErrEcosystemAmbiguous
			}
		}
		return remoteTenantLink{}, ErrUnavailable
	}
	var link remoteTenantLink
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&link); err != nil || !link.Linked || link.OwnerRole != "merchant_owner" {
		return remoteTenantLink{}, ErrUnavailable
	}
	return link, nil
}

func (s *Service) WithProvisioningToken(token string) *Service {
	s.provisioner = NewProvisioningClient(s.baseURL, token)
	return s
}

func (s *Service) EcosystemStatus(ctx context.Context, a authz.Actor) (EcosystemStatus, error) {
	if a.Impersonator != uuid.Nil {
		return EcosystemStatus{}, ErrImpersonating
	}
	if s.provisioner == nil {
		return EcosystemStatus{}, ErrEcosystemNotConfigured
	}
	link, err := s.provisioner.Status(ctx, a.TenantID)
	if err != nil {
		return EcosystemStatus{}, err
	}
	return EcosystemStatus{
		Active:         link.Linked && link.OwnerRole == "merchant_owner",
		MerchantStatus: link.MerchantStatus,
		OwnerRole:      link.OwnerRole,
	}, nil
}

func (s *Service) ActivateEcosystem(ctx context.Context, a authz.Actor, acceptedTerms bool) (EcosystemStatus, error) {
	if a.Impersonator != uuid.Nil {
		return EcosystemStatus{}, ErrImpersonating
	}
	if !a.Perms.All {
		return EcosystemStatus{}, ErrEcosystemOwner
	}
	if !acceptedTerms {
		return EcosystemStatus{}, ErrEcosystemTerms
	}
	if s.provisioner == nil {
		return EcosystemStatus{}, ErrEcosystemNotConfigured
	}
	var business, owner, email, phone string
	var verified bool
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT t.name, u.name, u.email, COALESCE(u.phone, ''), u.email_verified_at IS NOT NULL
			FROM tenants t JOIN users u ON u.tenant_id = t.id
			WHERE t.id = $1 AND u.id = $2 AND t.active AND u.active`, a.TenantID, a.UserID).
			Scan(&business, &owner, &email, &phone, &verified)
	})
	if err != nil {
		return EcosystemStatus{}, err
	}
	if !verified {
		return EcosystemStatus{}, ErrEcosystemEmail
	}
	link, err := s.provisioner.Ensure(ctx, a.TenantID, business, owner, email, phone)
	if err != nil {
		return EcosystemStatus{}, err
	}
	return EcosystemStatus{Active: true, MerchantStatus: link.MerchantStatus, OwnerRole: link.OwnerRole}, nil
}
