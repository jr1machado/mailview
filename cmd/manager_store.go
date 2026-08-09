package main

import (
	"context"
	"errors"
	"sync"

	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/listmonk/internal/core"
	"github.com/knadh/listmonk/internal/mailview/tenant"
	"github.com/knadh/listmonk/internal/manager"
	"github.com/knadh/listmonk/internal/media"
	"github.com/knadh/listmonk/models"
	"github.com/lib/pq"
)

// store implements DataSource over the primary
// database.
type store struct {
	queries *models.Queries
	core    *core.Core
	media   media.Store
	db      *sqlx.DB

	tenantMu             sync.RWMutex
	campaignTenantByID   map[int]uuid.UUID
	campaignTenantByUUID map[string]uuid.UUID
	subscriberTenantByID map[int64]uuid.UUID
}

type runningCamp struct {
	CampaignID       int    `db:"campaign_id"`
	CampaignType     string `db:"campaign_type"`
	LastSubscriberID int    `db:"last_subscriber_id"`
	MaxSubscriberID  int    `db:"max_subscriber_id"`
	ListID           int    `db:"list_id"`
}

func newManagerStore(q *models.Queries, c *core.Core, m media.Store, db *sqlx.DB) *store {
	return &store{
		queries:              q,
		core:                 c,
		media:                m,
		db:                   db,
		campaignTenantByID:   make(map[int]uuid.UUID),
		campaignTenantByUUID: make(map[string]uuid.UUID),
		subscriberTenantByID: make(map[int64]uuid.UUID),
	}
}

func (s *store) campaignTenant(id int, campaignUUID string) (uuid.UUID, error) {
	s.tenantMu.RLock()
	if tenantID, ok := s.campaignTenantByID[id]; id > 0 && ok {
		s.tenantMu.RUnlock()
		return tenantID, nil
	}
	if tenantID, ok := s.campaignTenantByUUID[campaignUUID]; campaignUUID != "" && ok {
		s.tenantMu.RUnlock()
		return tenantID, nil
	}
	s.tenantMu.RUnlock()

	var parsedUUID any
	if campaignUUID != "" {
		parsed, err := uuid.FromString(campaignUUID)
		if err != nil {
			return uuid.Nil, err
		}
		parsedUUID = parsed
	}
	tenantIDs, err := s.activeTenantIDs()
	if err != nil {
		return uuid.Nil, err
	}
	for _, tenantID := range tenantIDs {
		tx, _, _, err := s.begin(tenantID)
		if err != nil {
			return uuid.Nil, err
		}
		var found bool
		err = tx.Get(&found, `SELECT EXISTS(SELECT 1 FROM campaigns WHERE ($1>0 AND id=$1) OR ($2::uuid IS NOT NULL AND uuid=$2))`, id, parsedUUID)
		_ = tx.Rollback()
		if err != nil {
			return uuid.Nil, err
		}
		if found {
			s.cacheCampaignTenant(id, campaignUUID, tenantID)
			return tenantID, nil
		}
	}
	return uuid.Nil, errors.New("campaign tenant not found")
}

func (s *store) activeTenantIDs() ([]uuid.UUID, error) {
	var tenantIDs []uuid.UUID
	err := s.db.Select(&tenantIDs, `SELECT id FROM mv_tenants WHERE status='active' OR id='00000000-0000-0000-0000-000000000001'`)
	return tenantIDs, err
}

func (s *store) cacheCampaignTenant(id int, campaignUUID string, tenantID uuid.UUID) {
	s.tenantMu.Lock()
	defer s.tenantMu.Unlock()
	if id > 0 {
		s.campaignTenantByID[id] = tenantID
	}
	if campaignUUID != "" {
		s.campaignTenantByUUID[campaignUUID] = tenantID
	}
}

func (s *store) subscriberTenant(id int64) (uuid.UUID, error) {
	s.tenantMu.RLock()
	if tenantID, ok := s.subscriberTenantByID[id]; ok {
		s.tenantMu.RUnlock()
		return tenantID, nil
	}
	s.tenantMu.RUnlock()
	tenantIDs, err := s.activeTenantIDs()
	if err != nil {
		return uuid.Nil, err
	}
	for _, tenantID := range tenantIDs {
		tx, _, _, err := s.begin(tenantID)
		if err != nil {
			return uuid.Nil, err
		}
		var found bool
		err = tx.Get(&found, `SELECT EXISTS(SELECT 1 FROM subscribers WHERE id=$1)`, id)
		_ = tx.Rollback()
		if err != nil {
			return uuid.Nil, err
		}
		if found {
			s.tenantMu.Lock()
			s.subscriberTenantByID[id] = tenantID
			s.tenantMu.Unlock()
			return tenantID, nil
		}
	}
	return uuid.Nil, errors.New("subscriber tenant not found")
}

func (s *store) begin(tenantID uuid.UUID) (*sqlx.Tx, *models.Queries, *core.Core, error) {
	ctx := tenant.WithContext(context.Background(), tenant.Context{TenantID: tenantID})
	tx, _, err := tenant.Begin(ctx, s.db)
	if err != nil {
		return nil, nil, nil, err
	}
	return tx, s.queries.WithTx(tx), s.core.WithTx(tx), nil
}

// NextCampaigns retrieves active campaigns ready to be processed excluding
// campaigns that are also being processed. Additionally, it takes a map of campaignID:sentCount
// of campaigns that are being processed and updates them in the DB.
func (s *store) NextCampaigns(currentIDs []int64, sentCounts []int64) ([]*models.Campaign, error) {
	out := []*models.Campaign{}
	tenantIDs, err := s.activeTenantIDs()
	if err != nil {
		return nil, err
	}
	for _, tenantID := range tenantIDs {
		tx, q, _, err := s.begin(tenantID)
		if err != nil {
			return nil, err
		}
		var rows []*models.Campaign
		err = q.NextCampaigns.Select(&rows, pq.Int64Array(currentIDs), pq.Int64Array(sentCounts))
		if err == nil {
			err = tx.Commit()
		} else {
			tx.Rollback()
		}
		if err != nil {
			return nil, err
		}
		for _, campaign := range rows {
			s.cacheCampaignTenant(campaign.ID, campaign.UUID, tenantID)
		}
		out = append(out, rows...)
	}
	return out, nil
}

// NextSubscribers retrieves a subset of subscribers of a given campaign.
// Since batches are processed sequentially, the retrieval is ordered by ID,
// and every batch takes the last ID of the last batch and fetches the next
// batch above that.
func (s *store) NextSubscribers(campID, limit int) ([]models.Subscriber, error) {
	tenantID, err := s.campaignTenant(campID, "")
	if err != nil {
		return nil, err
	}
	tx, q, _, err := s.begin(tenantID)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var camps []runningCamp
	if err := q.GetRunningCampaign.Select(&camps, campID); err != nil {
		return nil, err
	}

	var listIDs []int
	for _, c := range camps {
		listIDs = append(listIDs, c.ListID)
	}

	if len(listIDs) == 0 {
		return nil, nil
	}

	var out []models.Subscriber
	err = q.NextCampaignSubscribers.Select(&out, camps[0].CampaignID, camps[0].CampaignType, camps[0].LastSubscriberID, camps[0].MaxSubscriberID, pq.Array(listIDs), limit)
	if err != nil {
		return nil, err
	}
	s.tenantMu.Lock()
	for i := range out {
		s.subscriberTenantByID[int64(out[i].ID)] = tenantID
	}
	s.tenantMu.Unlock()
	return out, tx.Commit()
}

// GetCampaign fetches a campaign from the database.
func (s *store) GetCampaign(campID int) (*models.Campaign, error) {
	tenantID, err := s.campaignTenant(campID, "")
	if err != nil {
		return nil, err
	}
	tx, q, _, err := s.begin(tenantID)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var out = &models.Campaign{}
	err = q.GetCampaign.Get(out, campID, nil, nil, "default")
	if err != nil {
		return nil, err
	}
	return out, tx.Commit()
}

// UpdateCampaignStatus updates a campaign's status.
func (s *store) UpdateCampaignStatus(campID int, status string) error {
	tenantID, err := s.campaignTenant(campID, "")
	if err != nil {
		return err
	}
	tx, q, _, err := s.begin(tenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = q.UpdateCampaignStatus.Exec(campID, status); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateCampaignCounts updates a campaign's status.
func (s *store) UpdateCampaignCounts(campID int, toSend int, sent int, lastSubID int) error {
	tenantID, err := s.campaignTenant(campID, "")
	if err != nil {
		return err
	}
	tx, q, _, err := s.begin(tenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = q.UpdateCampaignCounts.Exec(campID, toSend, sent, lastSubID); err != nil {
		return err
	}
	return tx.Commit()
}

// GetAttachment fetches a media attachment blob.
func (s *store) GetAttachment(campaignID, mediaID int) (models.Attachment, error) {
	tenantID, err := s.campaignTenant(campaignID, "")
	if err != nil {
		return models.Attachment{}, err
	}
	tx, _, co, err := s.begin(tenantID)
	if err != nil {
		return models.Attachment{}, err
	}
	defer tx.Rollback()
	m, err := co.GetMedia(mediaID, "", "", s.media)
	if err != nil {
		return models.Attachment{}, err
	}

	b, err := s.media.GetBlob(m.URL)
	if err != nil {
		return models.Attachment{}, err
	}

	out := models.Attachment{
		Name:    m.Filename,
		Content: b,
		Header:  manager.MakeAttachmentHeader(m.Filename, "base64", m.ContentType),
	}
	return out, tx.Commit()
}

// GetInlineAttachmentByFilename fetches a media item by filename and returns
// it as an inline attachment along with the Content-ID value. The lookup is
// uniform across filesystem and S3 providers because both use the same media
// store interface; the first match for a given filename is returned.
func (s *store) GetInlineAttachmentByFilename(campaignID int, filename string) (models.Attachment, string, error) {
	tenantID := uuid.Must(uuid.FromString("00000000-0000-0000-0000-000000000001"))
	var err error
	if campaignID > 0 {
		tenantID, err = s.campaignTenant(campaignID, "")
		if err != nil {
			return models.Attachment{}, "", err
		}
	}
	tx, _, co, err := s.begin(tenantID)
	if err != nil {
		return models.Attachment{}, "", err
	}
	defer tx.Rollback()
	if campaignID > 0 {
		filename = tenantID.String() + "/" + filename
	}
	m, err := co.GetMedia(0, "", filename, s.media)
	if err != nil {
		return models.Attachment{}, "", err
	}

	b, err := s.media.GetBlob(m.URL)
	if err != nil {
		return models.Attachment{}, "", err
	}

	cid := manager.MakeContentID(m.Filename)
	out := models.Attachment{
		Name:     m.Filename,
		Content:  b,
		Header:   manager.MakeInlineAttachmentHeader(m.Filename, "", m.ContentType, cid),
		IsInline: true,
	}
	if err := tx.Commit(); err != nil {
		return models.Attachment{}, "", err
	}
	return out, cid, nil
}

// CreateLink registers a URL with a UUID for tracking clicks and returns the UUID.
func (s *store) CreateLink(campaignUUID, url string) (string, error) {
	tenantID, err := s.campaignTenant(0, campaignUUID)
	if err != nil {
		return "", err
	}
	tx, q, _, err := s.begin(tenantID)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	// Create a new UUID for the URL. If the URL already exists in the DB
	// the UUID in the database is returned.
	uu, err := uuid.NewV4()
	if err != nil {
		return "", err
	}

	var out string
	if err := q.CreateLink.Get(&out, uu, url); err != nil {
		return "", err
	}

	return out, tx.Commit()
}

// RecordBounce records a bounce event and returns the bounce count.
func (s *store) RecordBounce(b models.Bounce) (int64, int, error) {
	var res = struct {
		SubscriberID int64 `db:"subscriber_id"`
		Num          int   `db:"num"`
	}{}

	err := s.queries.UpdateCampaignStatus.Select(&res,
		b.SubscriberUUID,
		b.Email,
		b.CampaignUUID,
		b.Type,
		b.Source,
		b.Meta)

	return res.SubscriberID, res.Num, err
}

// BlocklistSubscriber blocklists a subscriber permanently.
func (s *store) BlocklistSubscriber(id int64) error {
	tenantID, err := s.subscriberTenant(id)
	if err != nil {
		return err
	}
	tx, q, _, err := s.begin(tenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = q.BlocklistSubscribers.Exec(pq.Int64Array{id}); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteSubscriber deletes a subscriber from the DB.
func (s *store) DeleteSubscriber(id int64) error {
	tenantID, err := s.subscriberTenant(id)
	if err != nil {
		return err
	}
	tx, q, _, err := s.begin(tenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = q.DeleteSubscribers.Exec(pq.Int64Array{id}); err != nil {
		return err
	}
	return tx.Commit()
}
