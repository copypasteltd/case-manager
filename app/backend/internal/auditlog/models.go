// Package auditlog exposes the organization security-metadata plane. It never
// returns arbitrary audit reasons, session identifiers or business payloads.
package auditlog

import (
	"context"
	"regexp"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5/pgxpool"
)

const MaxExportRows = 10000
const MaxExportBytes = 8 << 20
const ChunkBytes = 512 << 10
const Disclosure = "organization_metadata_v1"

type Service struct {
	DB       *pgxpool.Pool
	Commands platform.Commands
	Pager    platform.Pager
}

type Filter struct {
	From         string `json:"from"`
	To           string `json:"to"`
	ActorID      string `json:"actor_id"`
	ExecutorID   string `json:"executor_id"`
	Channel      string `json:"channel"`
	Action       string `json:"action"`
	Result       string `json:"result"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	RequestID    string `json:"request_id"`
	RunID        string `json:"run_id"`
	JobID        string `json:"job_id"`
}

var labelPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:-]{0,127}$`)

func (f Filter) Validate() error {
	from, e1 := time.Parse(time.RFC3339Nano, f.From)
	to, e2 := time.Parse(time.RFC3339Nano, f.To)
	if e1 != nil || e2 != nil || !from.Before(to) || to.Sub(from) > 366*24*time.Hour {
		return core.Invalid("from", "请明确选择不超过366天的起止时间，结束时间不包含在内")
	}
	for _, v := range []string{f.ActorID, f.ExecutorID, f.ResourceID, f.RunID, f.JobID} {
		if v != "" && !core.ValidID(v) {
			return core.Invalid("identifier", "审计标识须为有效UUID")
		}
	}
	for _, v := range []string{f.Channel, f.Action, f.Result, f.ResourceType, f.RequestID} {
		if v != "" && !labelPattern.MatchString(v) {
			return core.Invalid("filter", "审计筛选只接受1至128字符的准确技术标识")
		}
	}
	return nil
}

type Event struct {
	ID              string    `json:"id"`
	OccurredAt      time.Time `json:"occurred_at"`
	ActorID         *string   `json:"actor_id"`
	ExecutorID      *string   `json:"executor_id"`
	Channel         string    `json:"channel"`
	Action          string    `json:"action"`
	Result          string    `json:"result"`
	ResourceType    string    `json:"resource_type"`
	ResourceID      *string   `json:"resource_id"`
	ResourceVersion *string   `json:"resource_version"`
	RequestID       string    `json:"request_id"`
	RunID           *string   `json:"run_id"`
	JobID           *string   `json:"job_id"`
	BeforeDigest    *string   `json:"before_digest"`
	AfterDigest     *string   `json:"after_digest"`
}

type Page struct {
	core.Page[Event]
	Filters    Filter `json:"filters"`
	Disclosure string `json:"disclosure"`
}

type ExportCreate struct {
	Filters Filter `json:"filters"`
	Reason  string `json:"reason"`
}

type Export struct {
	ID          string    `json:"id"`
	Filters     Filter    `json:"filters"`
	Disclosure  string    `json:"disclosure"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	RowCount    int       `json:"row_count"`
	CSVBytes    int       `json:"csv_bytes"`
	CSVSHA256   string    `json:"csv_sha256"`
	State       string    `json:"state"`
	CanDownload bool      `json:"can_download"`
}

type Download struct {
	ExportID   string `json:"export_id"`
	Filename   string `json:"filename"`
	TotalBytes string `json:"total_size_bytes"`
	SHA256     string `json:"sha256"`
	Content    []byte `json:"-"`
}

func authority(ctx context.Context, db platform.DBTX, a core.Actor, export, mfa bool) error {
	if err := authz.RequireOrganization(ctx, db, a, "audit.read"); err != nil {
		return err
	}
	if export {
		if err := authz.RequireOrganization(ctx, db, a, "audit.export"); err != nil {
			return err
		}
	}
	return identity.RequireInteractiveSession(ctx, db, a, mfa)
}
