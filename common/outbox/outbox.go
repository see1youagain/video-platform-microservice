package outbox

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	StatusPending    = 0
	StatusProcessing = 1
	StatusSent       = 2
)

type Event struct {
	ID            uint64     `gorm:"primaryKey;autoIncrement"`
	AggregateType string     `gorm:"size:64;not null;index"`
	AggregateID   string     `gorm:"size:128;not null;index"`
	Topic         string     `gorm:"size:128;not null;index"`
	MessageKey    string     `gorm:"column:message_key;size:255;not null;default:''"`
	Payload       string     `gorm:"type:longtext;not null"`
	Headers       string     `gorm:"type:text"`
	Status        int8       `gorm:"not null;default:0;index"`
	RetryCount    int32      `gorm:"not null;default:0"`
	AvailableAt   time.Time  `gorm:"not null;index"`
	SentAt        *time.Time `gorm:"index"`
	LastError     string     `gorm:"type:text"`
	LockedBy      string     `gorm:"size:64;index"`
	LockedAt      *time.Time
	CreatedAt     time.Time  `gorm:"autoCreateTime"`
	UpdatedAt     time.Time  `gorm:"autoUpdateTime"`
}

func (Event) TableName() string {
	return "outbox_events"
}

type Message struct {
	AggregateType string
	AggregateID   string
	Topic         string
	MessageKey    string
	Payload       string
	Headers       string
}

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) InitSchema() error {
	return r.db.AutoMigrate(&Event{})
}

func (r *Repository) EnqueueTx(tx *gorm.DB, msg Message) error {
	e := Event{
		AggregateType: msg.AggregateType,
		AggregateID:   msg.AggregateID,
		Topic:         msg.Topic,
		MessageKey:    msg.MessageKey,
		Payload:       msg.Payload,
		Headers:       msg.Headers,
		Status:        StatusPending,
		AvailableAt:   time.Now(),
	}
	return tx.Create(&e).Error
}

func (r *Repository) Enqueue(msg Message) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return r.EnqueueTx(tx, msg)
	})
}

func (r *Repository) ClaimBatch(worker string, batchSize int) ([]Event, error) {
	if batchSize <= 0 {
		batchSize = 50
	}

	var claimed []Event
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var ids []uint64
		q := `
SELECT id
FROM outbox_events
WHERE status = ? AND available_at <= NOW()
ORDER BY id
LIMIT ?
FOR UPDATE SKIP LOCKED`
		if err := tx.Raw(q, StatusPending, batchSize).Scan(&ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			claimed = []Event{}
			return nil
		}

		now := time.Now()
		if err := tx.Model(&Event{}).
			Where("id IN ?", ids).
			Updates(map[string]any{
				"status":    StatusProcessing,
				"locked_by": worker,
				"locked_at": now,
			}).Error; err != nil {
			return err
		}

		if err := tx.Where("id IN ?", ids).Order("id ASC").Find(&claimed).Error; err != nil {
			return err
		}
		return nil
	})
	return claimed, err
}

func (r *Repository) MarkSent(id uint64) error {
	now := time.Now()
	return r.db.Model(&Event{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     StatusSent,
			"sent_at":    now,
			"last_error": "",
			"locked_by":  "",
			"locked_at":  nil,
		}).Error
}

func (r *Repository) MarkRetry(id uint64, errMsg string, currentRetry int32) error {
	nextRetry := currentRetry + 1
	backoffSeconds := int(math.Min(math.Pow(2, float64(nextRetry)), 300))
	if backoffSeconds < 1 {
		backoffSeconds = 1
	}
	nextAt := time.Now().Add(time.Duration(backoffSeconds) * time.Second)

	if len(errMsg) > 2000 {
		errMsg = errMsg[:2000]
	}
	return r.db.Model(&Event{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":      StatusPending,
			"retry_count": nextRetry,
			"available_at": nextAt,
			"last_error":  errMsg,
			"locked_by":   "",
			"locked_at":   nil,
		}).Error
}

type PublishFunc func(ctx context.Context, topic, key, payload string) error

type Dispatcher struct {
	repo         *Repository
	worker       string
	batchSize    int
	pollInterval time.Duration
	publish      PublishFunc
}

func NewDispatcher(repo *Repository, worker string, batchSize int, pollInterval time.Duration, publish PublishFunc) *Dispatcher {
	if strings.TrimSpace(worker) == "" {
		worker = "outbox-worker"
	}
	if batchSize <= 0 {
		batchSize = 50
	}
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	return &Dispatcher{
		repo:         repo,
		worker:       worker,
		batchSize:    batchSize,
		pollInterval: pollInterval,
		publish:      publish,
	}
}

func (d *Dispatcher) Run(ctx context.Context) {
	ticker := time.NewTicker(d.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.dispatchOnce(ctx)
		}
	}
}

func (d *Dispatcher) dispatchOnce(ctx context.Context) {
	events, err := d.repo.ClaimBatch(d.worker, d.batchSize)
	if err != nil {
		return
	}
	if len(events) == 0 {
		return
	}

	for _, evt := range events {
		err := d.publish(ctx, evt.Topic, evt.MessageKey, evt.Payload)
		if err != nil {
			_ = d.repo.MarkRetry(evt.ID, fmt.Sprintf("publish failed: %v", err), evt.RetryCount)
			continue
		}
		_ = d.repo.MarkSent(evt.ID)
	}
}
