// Package notification projects committed business events into employees' own
// in-app inboxes. Every delivery and every read rechecks the target's access.
package notification

import (
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Service struct {
	DB       *pgxpool.Pool
	Commands platform.Commands
	Pager    platform.Pager
}
type Target struct {
	Type        string  `json:"type"`
	ID          string  `json:"id"`
	ContainerID *string `json:"container_id"`
	CommentID   *string `json:"comment_id,omitempty"`
	VersionID   *string `json:"version_id,omitempty"`
	ReplicaID   *string `json:"replica_id,omitempty"`
	WebPath     string  `json:"web_path"`
}
type Notification struct {
	ID                string     `json:"id"`
	Category          string     `json:"category"`
	Title             string     `json:"title"`
	ResourceLabel     string     `json:"resource_label"`
	CurrentState      string     `json:"current_state"`
	AttentionRequired bool       `json:"attention_required"`
	Read              bool       `json:"read"`
	DeliverySeq       int64      `json:"delivery_seq"`
	ReadSeq           int64      `json:"read_seq"`
	BusinessEventID   string     `json:"business_event_id"`
	TemplateVersion   int        `json:"template_version"`
	Target            Target     `json:"target"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	ReadAt            *time.Time `json:"read_at"`
}
type Summary struct {
	UnreadCount    int       `json:"unread_count"`
	AttentionCount int       `json:"attention_count"`
	CountComplete  bool      `json:"count_complete"`
	AsOf           time.Time `json:"as_of"`
}
type ReadInput struct {
	DeliverySeq int64 `json:"delivery_seq"`
}
type Filter struct {
	State, Category, Cursor string
	Limit                   int
}
type Failure struct {
	JobID         string     `json:"job_id"`
	State         string     `json:"state"`
	SafeCode      *string    `json:"safe_code"`
	Attempt       int        `json:"attempt"`
	MaxAttempts   int        `json:"max_attempts"`
	FirstFailedAt *time.Time `json:"first_failed_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}
type thread struct {
	// Only delivery candidates carry an event basis; ordinary inbox reads
	// present current state, while a delayed event cannot deliver newer facts.
	CandidateEventType                                                            string
	CandidateEventVersion                                                         int64
	ID, OrgID, RecipientID, Category, ResourceType, ResourceID, DedupKey, EventID string
	ContextID                                                                     *string
	DeliveredSeq, ReadSeq, Version                                                int64
	CreatedAt, UpdatedAt, EventOccurredAt                                         time.Time
	ReadAt                                                                        *time.Time
}

var categories = map[string]string{
	"mention": "有人在任务讨论中提到你", "task": "任务待推进", "approval": "有申请待你审批", "request_result": "业务申请有新进展",
	"contract_due": "合同到期提醒", "subscription_due": "订阅账期待核对", "replica_failure": "文件副本异常",
	"incident": "服务事件有新进展", "secretary": "秘书任务有新结果",
	"service_resource": "主机资源状态有新进展",
}

const threadColumns = `n.id,n.org_id,n.recipient_id,n.category,n.resource_type,n.resource_id,n.context_id,n.dedup_key,
 n.delivered_seq,n.read_seq,n.version,n.created_at,n.updated_at,n.read_at,d.business_event_id,d.event_occurred_at`
const threadJoin = ` FROM notification_threads n JOIN notification_deliveries d ON d.org_id=n.org_id AND d.thread_id=n.id AND d.id=n.latest_delivery_id `
