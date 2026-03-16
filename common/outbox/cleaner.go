package outbox

import (
"context"
"log"
"time"
)

func (r *Repository) StartCleaner(ctx context.Context, interval, retention time.Duration) {
ticker := time.NewTicker(interval)
defer ticker.Stop()

for {
select {
case <-ctx.Done():
log.Println("[outbox] cleaner task stopped")
return
case <-ticker.C:
r.cleanOutdatedEvents(retention)
}
}
}

func (r *Repository) cleanOutdatedEvents(retention time.Duration) {
deadline := time.Now().Add(-retention)
result := r.db.Where("status = ? AND updated_at < ?", "published", deadline).Delete(&Event{})
if result.Error != nil {
log.Printf("[outbox] failed to clean outdated events: %v", result.Error)
return
}
if result.RowsAffected > 0 {
log.Printf("[outbox] successfully cleaned %d outdated events (older than %v)", result.RowsAffected, retention)
}
}
