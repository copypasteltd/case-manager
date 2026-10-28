package finance

import (
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type FXRate struct {
	Currency  string `json:"currency"`
	ValidFrom string `json:"valid_from"`
	ValidTo   string `json:"valid_to"`
	RateDate  string `json:"rate_date"`
	Rate      string `json:"rate"`
	Source    string `json:"source"`
}
type FXContent struct {
	Title        string   `json:"title"`
	BaseCurrency string   `json:"base_currency"`
	Method       string   `json:"method"`
	Rounding     string   `json:"rounding"`
	Rates        []FXRate `json:"rates"`
}
type FXBook struct {
	ID            string     `json:"id"`
	Revision      int64      `json:"revision"`
	Title         string     `json:"title"`
	BaseCurrency  string     `json:"base_currency"`
	RateCount     int        `json:"rate_count"`
	ContentSHA256 string     `json:"content_sha256"`
	CreatorID     string     `json:"creator_id"`
	Reason        string     `json:"reason"`
	CreatedAt     time.Time  `json:"created_at"`
	Content       *FXContent `json:"content,omitempty"`
}
type FXPolicy struct {
	Version int64   `json:"version"`
	Current *FXBook `json:"current"`
}
type FXBookCreate struct {
	ExpectedVersion int64     `json:"expected_version"`
	Content         FXContent `json:"content"`
	Acknowledged    bool      `json:"acknowledged"`
	Reason          string    `json:"reason"`
}

const fxBookColumns = `id,revision,title,base_currency,rate_count,content_sha256,creator_id,reason,created_at`

var fxRatePattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,9})(\.[0-9]{1,10})?$`)

func normalizeFXContent(ctx context.Context, db platform.DBTX, in FXContent) (FXContent, error) {
	in.Title = strings.TrimSpace(in.Title)
	if core.Name(in.Title) != nil || in.Method != "manual_interval" || in.Rounding != "scope_line_half_up" || len(in.Rates) > 2000 {
		return in, core.Invalid("content", "请明确标题、人工区间汇率和逐条范围金额舍入规则，最多2000条汇率")
	}
	var exponent int
	if err := db.QueryRow(ctx, "SELECT exponent FROM currency_definitions WHERE code=$1", in.BaseCurrency).Scan(&exponent); err != nil {
		return in, core.Invalid("base_currency", "请选择已配置的基础币种")
	}
	in.Rates = append([]FXRate{}, in.Rates...)
	slices.SortFunc(in.Rates, func(a, b FXRate) int {
		if v := strings.Compare(a.Currency, b.Currency); v != 0 {
			return v
		}
		return strings.Compare(a.ValidFrom, b.ValidFrom)
	})
	for i := range in.Rates {
		r := &in.Rates[i]
		r.Source = strings.TrimSpace(r.Source)
		start, e := time.Parse(time.DateOnly, r.ValidFrom)
		end, last := time.Parse(time.DateOnly, r.ValidTo)
		if e != nil || last != nil || !end.After(start) || end.Sub(start) > 366*24*time.Hour || core.Date(r.RateDate) != nil || core.Name(r.Source) != nil || !fxRatePattern.MatchString(r.Rate) || strings.Trim(r.Rate, "0.") == "" || r.Currency == in.BaseCurrency {
			return in, core.Invalid("rates", "汇率须为正十进制字符串；请填写来源、报价日及不超过366天的有效区间，结束日不包含在内")
		}
		if err := db.QueryRow(ctx, "SELECT exponent FROM currency_definitions WHERE code=$1", r.Currency).Scan(&exponent); err != nil {
			return in, core.Invalid("rates", "汇率币种尚未配置")
		}
		if i > 0 && in.Rates[i-1].Currency == r.Currency && in.Rates[i-1].ValidTo > r.ValidFrom {
			return in, core.Invalid("rates", "同一币种的有效区间不能重复或重叠")
		}
	}
	return in, nil
}
func fxReadAuthority(ctx context.Context, tx pgx.Tx, a core.Actor, project *string) error {
	if a.RunID != "" {
		return core.Missing()
	}
	if project != nil && !core.ValidID(*project) {
		return core.Invalid("project_id", "项目标识无效")
	}
	if err := reportIdentity(ctx, tx, a); err != nil {
		return err
	}
	return authz.Require(ctx, tx, a, "report.read", authz.Resource{Type: "report", ProjectID: project, OwnerID: &a.UserID, Classification: "internal"})
}
func readFXBook(ctx context.Context, tx pgx.Tx, org, id string, full bool) (FXBook, error) {
	var out FXBook
	if !core.ValidID(id) {
		return out, core.Missing()
	}
	var raw []byte
	query := "SELECT " + fxBookColumns
	if full {
		query += ",content"
	}
	query += " FROM finance_fx_books WHERE org_id=$1 AND id=$2"
	fields := []any{&out.ID, &out.Revision, &out.Title, &out.BaseCurrency, &out.RateCount, &out.ContentSHA256, &out.CreatorID, &out.Reason, &out.CreatedAt}
	if full {
		fields = append(fields, &raw)
	}
	if err := tx.QueryRow(ctx, query, org, id).Scan(fields...); err != nil {
		return out, platform.DatabaseError(err)
	}
	if full {
		if core.Digest(raw) != out.ContentSHA256 || json.Unmarshal(raw, &out.Content) != nil {
			return out, core.Fail(500, "FX_BOOK_INTEGRITY_FAILED", "汇率版本内容校验失败")
		}
	}
	return out, nil
}
func (s *Service) FXPolicy(ctx context.Context, a core.Actor, project *string) (FXPolicy, error) {
	var out FXPolicy
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = fxReadAuthority(ctx, tx, a, project); err != nil {
		return out, err
	}
	var id string
	err = tx.QueryRow(ctx, "SELECT id FROM finance_fx_books WHERE org_id=$1 ORDER BY revision DESC LIMIT 1", a.OrgID).Scan(&id)
	if err == pgx.ErrNoRows {
		return out, tx.Commit(ctx)
	}
	if err != nil {
		return out, err
	}
	book, err := readFXBook(ctx, tx, a.OrgID, id, false)
	if err != nil {
		return out, err
	}
	out.Current = &book
	out.Version = book.Revision
	return out, tx.Commit(ctx)
}
func (s *Service) FXBook(ctx context.Context, a core.Actor, id string, project *string) (FXBook, error) {
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return FXBook{}, err
	}
	defer tx.Rollback(ctx)
	if err = fxReadAuthority(ctx, tx, a, project); err != nil {
		return FXBook{}, err
	}
	out, err := readFXBook(ctx, tx, a.OrgID, id, true)
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}
func (s *Service) FXBooks(ctx context.Context, a core.Actor, project *string, cursor string, limit int) (core.Page[FXBook], error) {
	out := core.Page[FXBook]{Items: []FXBook{}}
	if limit < 1 || limit > 100 {
		return out, core.Invalid("limit", "每页需为1至100个汇率版本")
	}
	scope := ""
	if project != nil {
		scope = *project
	}
	bound := core.CursorBinding(a, "finance-fx-books", scope)
	var at *time.Time
	var id *string
	if cursor != "" {
		v, err := s.Pager.Decode(cursor, bound)
		if err != nil {
			return out, err
		}
		at, id = &v.UpdatedAt, &v.ID
	}
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = fxReadAuthority(ctx, tx, a, project); err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, "SELECT "+fxBookColumns+" FROM finance_fx_books WHERE org_id=$1 AND ($2::timestamptz IS NULL OR (created_at,id)<($2,$3::uuid)) ORDER BY created_at DESC,id DESC LIMIT $4", a.OrgID, at, id, limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var v FXBook
		if err = rows.Scan(&v.ID, &v.Revision, &v.Title, &v.BaseCurrency, &v.RateCount, &v.ContentSHA256, &v.CreatorID, &v.Reason, &v.CreatedAt); err != nil {
			rows.Close()
			return out, err
		}
		out.Items = append(out.Items, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		v := out.Items[len(out.Items)-1]
		c := s.Pager.Encode(platform.Cursor{UpdatedAt: v.CreatedAt, ID: v.ID, Binding: bound})
		out.NextCursor = &c
	}
	return out, tx.Commit(ctx)
}
func (s *Service) PublishFXBook(ctx context.Context, c core.Command, in FXBookCreate) (core.Result, error) {
	if in.ExpectedVersion < 0 || !in.Acknowledged || core.Name(in.Reason) != nil || c.Actor.RunID != "" {
		return core.Result{}, core.Invalid("content", "请由员工确认汇率口径和发布依据，并固定当前版本")
	}
	var content FXContent
	var revision int64
	c.ExclusiveOrganizationLock = true
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		if err := identity.RequireInteractiveSession(ctx, tx, c.Actor, true); err != nil {
			return err
		}
		if err := authz.RequireOrganization(ctx, tx, c.Actor, "finance.fx.manage"); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, "SELECT coalesce(max(revision),0) FROM finance_fx_books WHERE org_id=$1", c.Actor.OrgID).Scan(&revision); err != nil {
			return err
		}
		// Command replay also checks current identity/permissions, but does not
		// invalidate its own exact receipt after a newer book has been published.
		var err error
		content, err = normalizeFXContent(ctx, tx, in.Content)
		return err
	}, func(tx pgx.Tx) (core.Result, error) {
		if revision != in.ExpectedVersion {
			return core.Result{}, core.Fail(409, "FX_POLICY_CHANGED", "汇率版本已更新，请重新读取后发布")
		}
		raw, _ := json.Marshal(content)
		raw, err := core.CanonicalJSON(raw)
		if err != nil {
			return core.Result{}, err
		}
		id := core.NewID()
		_, err = tx.Exec(ctx, `INSERT INTO finance_fx_books(id,org_id,revision,creator_id,title,base_currency,rate_count,content,content_sha256,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, c.Actor.OrgID, revision+1, c.Actor.UserID, content.Title, content.BaseCurrency, len(content.Rates), raw, core.Digest(raw), in.Reason)
		if err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE organizations SET base_currency=$2 WHERE id=$1", c.Actor.OrgID, content.BaseCurrency); err != nil {
			return core.Result{}, err
		}
		out, err := readFXBook(ctx, tx, c.Actor.OrgID, id, true)
		return core.Result{Status: 201, Data: out, Event: core.Event{Type: "finance.fx_book.published", ResourceType: "finance_fx_book", ResourceID: id, Version: revision + 1, Payload: map[string]any{"reason": in.Reason, "base_currency": content.BaseCurrency, "content_sha256": core.Digest(raw), "rate_count": len(content.Rates)}}}, err
	})
}
