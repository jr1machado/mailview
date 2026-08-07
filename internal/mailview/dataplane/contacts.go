package dataplane

import (
	"context"
	"errors"
	"strings"

	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/listmonk/internal/mailview/tenant"
)

var ErrInvalid = errors.New("invalid contacts or lists input")

type Service struct{ db *sqlx.DB }

func New(db *sqlx.DB) *Service { return &Service{db: db} }

type List struct {
	ID     int       `db:"id" json:"id"`
	UUID   uuid.UUID `db:"uuid" json:"uuid"`
	Name   string    `db:"name" json:"name"`
	Status string    `db:"status" json:"status"`
}
type Subscriber struct {
	ID     int       `db:"id" json:"id"`
	UUID   uuid.UUID `db:"uuid" json:"uuid"`
	Email  string    `db:"email" json:"email"`
	Name   string    `db:"name" json:"name"`
	Status string    `db:"status" json:"status"`
}
type CreateListInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}
type CreateSubscriberInput struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	ListIDs []int  `json:"list_ids"`
}

func (s *Service) CreateList(ctx context.Context, in CreateListInput) (List, error) {
	var out List
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 160 {
		return out, ErrInvalid
	}
	err := tenant.InTransaction(ctx, s.db, func(tx *sqlx.Tx, scope tenant.Context) error {
		return tx.GetContext(ctx, &out, `INSERT INTO lists (tenant_id, uuid, name, type, optin, status, description) VALUES ($1,$2,$3,'private','single','active',$4) RETURNING id,uuid,name,status`, scope.TenantID, uuid.Must(uuid.NewV4()), in.Name, strings.TrimSpace(in.Description))
	})
	return out, err
}
func (s *Service) ListLists(ctx context.Context) ([]List, error) {
	var out []List
	err := tenant.InTransaction(ctx, s.db, func(tx *sqlx.Tx, scope tenant.Context) error {
		return tx.SelectContext(ctx, &out, `SELECT id,uuid,name,status FROM lists WHERE tenant_id=$1 ORDER BY id DESC`, scope.TenantID)
	})
	return out, err
}
func (s *Service) CreateSubscriber(ctx context.Context, in CreateSubscriberInput) (Subscriber, error) {
	var out Subscriber
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.Name = strings.TrimSpace(in.Name)
	if !strings.Contains(in.Email, "@") || in.Name == "" {
		return out, ErrInvalid
	}
	err := tenant.InTransaction(ctx, s.db, func(tx *sqlx.Tx, scope tenant.Context) error {
		if err := tx.GetContext(ctx, &out, `INSERT INTO subscribers (tenant_id,uuid,email,name,status) VALUES ($1,$2,$3,$4,'enabled') RETURNING id,uuid,email,name,status`, scope.TenantID, uuid.Must(uuid.NewV4()), in.Email, in.Name); err != nil {
			return err
		}
		for _, listID := range in.ListIDs {
			if listID < 1 {
				return ErrInvalid
			}
			result, err := tx.ExecContext(ctx, `INSERT INTO subscriber_lists (tenant_id,subscriber_id,list_id,status) SELECT $1,$2,id,'confirmed' FROM lists WHERE id=$3 AND tenant_id=$1`, scope.TenantID, out.ID, listID)
			if err != nil {
				return err
			}
			n, _ := result.RowsAffected()
			if n != 1 {
				return errors.New("list not found in tenant")
			}
		}
		return nil
	})
	return out, err
}
func (s *Service) ListSubscribers(ctx context.Context) ([]Subscriber, error) {
	var out []Subscriber
	err := tenant.InTransaction(ctx, s.db, func(tx *sqlx.Tx, scope tenant.Context) error {
		return tx.SelectContext(ctx, &out, `SELECT id,uuid,email,name,status FROM subscribers WHERE tenant_id=$1 ORDER BY id DESC`, scope.TenantID)
	})
	return out, err
}

func (s *Service) GetList(ctx context.Context, id int) (List, error) {
	var out List
	err := tenant.InTransaction(ctx, s.db, func(tx *sqlx.Tx, scope tenant.Context) error {
		return tx.GetContext(ctx, &out, `SELECT id,uuid,name,status FROM lists WHERE tenant_id=$1 AND id=$2`, scope.TenantID, id)
	})
	return out, err
}
func (s *Service) UpdateList(ctx context.Context, id int, in CreateListInput) (List, error) {
	var out List
	in.Name = strings.TrimSpace(in.Name)
	if id < 1 || in.Name == "" {
		return out, ErrInvalid
	}
	err := tenant.InTransaction(ctx, s.db, func(tx *sqlx.Tx, scope tenant.Context) error {
		return tx.GetContext(ctx, &out, `UPDATE lists SET name=$3,description=$4,updated_at=now() WHERE tenant_id=$1 AND id=$2 RETURNING id,uuid,name,status`, scope.TenantID, id, in.Name, strings.TrimSpace(in.Description))
	})
	return out, err
}
func (s *Service) DeleteList(ctx context.Context, id int) error {
	return tenant.InTransaction(ctx, s.db, func(tx *sqlx.Tx, scope tenant.Context) error {
		r, e := tx.ExecContext(ctx, `DELETE FROM lists WHERE tenant_id=$1 AND id=$2`, scope.TenantID, id)
		if e != nil {
			return e
		}
		n, _ := r.RowsAffected()
		if n != 1 {
			return errors.New("list not found")
		}
		return nil
	})
}
func (s *Service) GetSubscriber(ctx context.Context, id int) (Subscriber, error) {
	var out Subscriber
	err := tenant.InTransaction(ctx, s.db, func(tx *sqlx.Tx, scope tenant.Context) error {
		return tx.GetContext(ctx, &out, `SELECT id,uuid,email,name,status FROM subscribers WHERE tenant_id=$1 AND id=$2`, scope.TenantID, id)
	})
	return out, err
}
func (s *Service) UpdateSubscriber(ctx context.Context, id int, in CreateSubscriberInput) (Subscriber, error) {
	var out Subscriber
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.Name = strings.TrimSpace(in.Name)
	if id < 1 || !strings.Contains(in.Email, "@") || in.Name == "" {
		return out, ErrInvalid
	}
	err := tenant.InTransaction(ctx, s.db, func(tx *sqlx.Tx, scope tenant.Context) error {
		return tx.GetContext(ctx, &out, `UPDATE subscribers SET email=$3,name=$4,updated_at=now() WHERE tenant_id=$1 AND id=$2 RETURNING id,uuid,email,name,status`, scope.TenantID, id, in.Email, in.Name)
	})
	return out, err
}
func (s *Service) DeleteSubscriber(ctx context.Context, id int) error {
	return tenant.InTransaction(ctx, s.db, func(tx *sqlx.Tx, scope tenant.Context) error {
		r, e := tx.ExecContext(ctx, `DELETE FROM subscribers WHERE tenant_id=$1 AND id=$2`, scope.TenantID, id)
		if e != nil {
			return e
		}
		n, _ := r.RowsAffected()
		if n != 1 {
			return errors.New("subscriber not found")
		}
		return nil
	})
}
