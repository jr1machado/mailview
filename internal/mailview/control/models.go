package control

import (
	"encoding/json"
	"time"

	"github.com/gofrs/uuid/v5"
)

const (
	TenantStatusActive     = "active"
	TenantStatusSuspended  = "suspended"
	TenantStatusPending    = "pending"
	TenantStatusOffboarded = "offboarded"

	MembershipStatusActive    = "active"
	MembershipStatusInvited   = "invited"
	MembershipStatusSuspended = "suspended"
	MembershipStatusRemoved   = "removed"
)

type Tenant struct {
	ID        uuid.UUID `db:"id" json:"id"`
	Slug      string    `db:"slug" json:"slug"`
	Name      string    `db:"name" json:"name"`
	Status    string    `db:"status" json:"status"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

type Membership struct {
	ID        uuid.UUID `db:"id" json:"id"`
	TenantID  uuid.UUID `db:"tenant_id" json:"tenant_id"`
	UserID    int       `db:"user_id" json:"user_id"`
	Status    string    `db:"status" json:"status"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

type Role struct {
	ID       uuid.UUID  `db:"id" json:"id"`
	TenantID *uuid.UUID `db:"tenant_id" json:"tenant_id,omitempty"`
	Scope    string     `db:"scope" json:"scope"`
	Name     string     `db:"name" json:"name"`
	IsSystem bool       `db:"is_system" json:"is_system"`
}

type AuditEvent struct {
	ID           uuid.UUID       `db:"id" json:"id"`
	OccurredAt   time.Time       `db:"occurred_at" json:"occurred_at"`
	TenantID     *uuid.UUID      `db:"tenant_id" json:"tenant_id,omitempty"`
	ActorUserID  *int            `db:"actor_user_id" json:"actor_user_id,omitempty"`
	Action       string          `db:"action" json:"action"`
	ResourceType string          `db:"resource_type" json:"resource_type"`
	ResourceID   string          `db:"resource_id" json:"resource_id"`
	RequestID    string          `db:"request_id" json:"request_id,omitempty"`
	SourceIP     *string         `db:"source_ip" json:"source_ip,omitempty"`
	UserAgent    string          `db:"user_agent" json:"user_agent,omitempty"`
	Result       string          `db:"result" json:"result"`
	Reason       string          `db:"reason" json:"reason,omitempty"`
	Metadata     json.RawMessage `db:"metadata" json:"metadata"`
}

type Actor struct {
	UserID    int
	RequestID string
	SourceIP  string
	UserAgent string
}

type CreateTenantInput struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	OwnerUserID int    `json:"owner_user_id"`
}

type CreateMembershipInput struct {
	UserID  int         `json:"user_id"`
	RoleIDs []uuid.UUID `json:"role_ids"`
}
