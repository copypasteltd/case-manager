package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	DB       *pgxpool.Pool
	Commands platform.Commands
	Pager    platform.Pager
}
type StepInput struct {
	Title          string `json:"title"`
	AssignedUserID string `json:"assigned_user_id"`
	RequiresMFA    bool   `json:"requires_mfa"`
}
type Step struct {
	Seq                int    `json:"seq"`
	Title              string `json:"title"`
	AssignedUserID     string `json:"assigned_user_id"`
	AssignedUserName   string `json:"assigned_user_name"`
	AssignedUserActive bool   `json:"assigned_user_active"`
	RequiresMFA        bool   `json:"requires_mfa"`
}
type DefinitionInput struct {
	ExpectedVersion    int64       `json:"expected_version"`
	Key                string      `json:"key"`
	Name               string      `json:"name"`
	Kind               string      `json:"kind"`
	Category           string      `json:"category"`
	Currency           string      `json:"currency"`
	MinAmount          string      `json:"min_amount"`
	MaxAmount          *string     `json:"max_amount"`
	Enabled            bool        `json:"enabled"`
	SelfApprovalPolicy string      `json:"self_approval_policy"`
	WithdrawalAllowed  bool        `json:"withdrawal_allowed"`
	Steps              []StepInput `json:"steps"`
	Reason             string      `json:"reason"`
}
type Definition struct {
	ID                 string    `json:"id"`
	Version            int64     `json:"version"`
	Key                string    `json:"key"`
	Name               string    `json:"name"`
	Kind               string    `json:"kind"`
	Category           string    `json:"category"`
	Currency           string    `json:"currency"`
	MinAmount          string    `json:"min_amount"`
	MaxAmount          *string   `json:"max_amount"`
	Enabled            bool      `json:"enabled"`
	SelfApprovalPolicy string    `json:"self_approval_policy"`
	WithdrawalAllowed  bool      `json:"withdrawal_allowed"`
	Steps              []Step    `json:"steps"`
	PublishedBy        string    `json:"published_by"`
	Reason             string    `json:"reason"`
	CreatedAt          time.Time `json:"created_at"`
}
type Filter struct {
	Kind, Cursor string
	Limit        int
}

const columns = `d.id,d.revision,d.key,d.name,d.kind,d.category,d.currency,d.min_amount::text,d.max_amount::text,
    d.enabled,d.self_approval_policy,d.withdrawal_allowed,d.published_by,d.reason,d.created_at,
    (SELECT coalesce(jsonb_agg(jsonb_build_object('seq',s.step_no,'title',s.title,'assigned_user_id',s.assigned_user_id,
        'assigned_user_name',u.display_name,'assigned_user_active',u.status='active','requires_mfa',s.requires_mfa) ORDER BY s.step_no),'[]'::jsonb)
        FROM workflow_definition_steps s JOIN users u ON u.org_id=s.org_id AND u.id=s.assigned_user_id
        WHERE s.org_id=d.org_id AND s.definition_id=d.id)`

func scanDefinition(row pgx.Row) (Definition, error) {
	var d Definition
	var raw []byte
	err := row.Scan(&d.ID, &d.Version, &d.Key, &d.Name, &d.Kind, &d.Category, &d.Currency, &d.MinAmount, &d.MaxAmount,
		&d.Enabled, &d.SelfApprovalPolicy, &d.WithdrawalAllowed, &d.PublishedBy, &d.Reason, &d.CreatedAt, &raw)
	if err != nil {
		return d, platform.DatabaseError(err)
	}
	err = json.Unmarshal(raw, &d.Steps)
	return d, err
}
func (s *Service) Definition(ctx context.Context, a core.Actor, id string) (Definition, error) {
	if err := authz.RequireOrganization(ctx, s.DB, a, "config.manage"); err != nil {
		return Definition{}, err
	}
	if !core.ValidID(id) {
		return Definition{}, core.Missing()
	}
	return scanDefinition(s.DB.QueryRow(ctx, "SELECT "+columns+" FROM workflow_definitions d WHERE d.org_id=$1 AND d.id=$2", a.OrgID, id))
}
func (s *Service) Definitions(ctx context.Context, a core.Actor, f Filter, historyID string) (core.Page[Definition], error) {
	out := core.Page[Definition]{Items: []Definition{}}
	if err := authz.RequireOrganization(ctx, s.DB, a, "config.manage"); err != nil {
		return out, err
	}
	if f.Kind != "" && !validKind(f.Kind) {
		return out, core.Invalid("kind", "流程业务类型无效")
	}
	key := ""
	if historyID != "" {
		if !core.ValidID(historyID) {
			return out, core.Missing()
		}
		if err := s.DB.QueryRow(ctx, "SELECT key FROM workflow_definitions WHERE org_id=$1 AND id=$2", a.OrgID, historyID).Scan(&key); err != nil {
			return out, platform.DatabaseError(err)
		}
	}
	binding := core.Digest([]byte(fmt.Sprint(a.OrgID, "/", a.UserID, "/", a.AuthVersion, "/", a.OrgAuthVersion, "/", a.Channel, "/", a.RunID, "/workflow/", f.Kind, "/", key)))
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	var at *time.Time
	var before *string
	if f.Cursor != "" {
		c, e := s.Pager.Decode(f.Cursor, binding)
		if e != nil {
			return out, e
		}
		at = &c.UpdatedAt
		before = &c.ID
	}
	rows, err := s.DB.Query(ctx, "SELECT "+columns+` FROM workflow_definitions d WHERE d.org_id=$1 AND ($2='' OR d.kind=$2)
        AND (($3<>'' AND d.key=$3) OR ($3='' AND NOT EXISTS(SELECT 1 FROM workflow_definitions later
            WHERE later.org_id=d.org_id AND later.key=d.key AND later.revision>d.revision)))
        AND ($4::timestamptz IS NULL OR (d.created_at,d.id)<($4,$5::uuid)) ORDER BY d.created_at DESC,d.id DESC LIMIT $6`,
		a.OrgID, f.Kind, key, at, before, f.Limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		d, e := scanDefinition(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > f.Limit {
		out.Items = out.Items[:f.Limit]
		last := out.Items[f.Limit-1]
		cursor := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.CreatedAt, Binding: binding})
		out.NextCursor = &cursor
	}
	return out, nil
}
func validKind(value string) bool {
	return slices.Contains([]string{"contract", "purchase", "claim", "change", "purchase_variance"}, value)
}

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{2,63}$`)

func validateDefinition(in DefinitionInput) error {
	if in.ExpectedVersion < 0 || !keyPattern.MatchString(in.Key) || !validKind(in.Kind) || core.Name(in.Name) != nil || core.Name(in.Reason) != nil ||
		strings.TrimSpace(in.Category) != in.Category || len([]rune(in.Category)) < 1 || len([]rune(in.Category)) > 50 ||
		!slices.Contains([]string{"allow", "deny"}, in.SelfApprovalPolicy) || len(in.Steps) < 1 || len(in.Steps) > 10 {
		return core.Invalid("definition", "请明确规则代码、类型、类别、本人审批和顺序步骤")
	}
	seen := map[string]bool{}
	for _, step := range in.Steps {
		if !core.ValidID(step.AssignedUserID) || core.Name(step.Title) != nil || seen[step.AssignedUserID] {
			return core.Invalid("steps", "须明确有效审批人和步骤名称，同一流程不可重复指定同一个人")
		}
		seen[step.AssignedUserID] = true
	}
	return nil
}
func currencyAmount(ctx context.Context, db platform.DBTX, currency, value string) (core.Decimal, error) {
	var exponent int
	if err := db.QueryRow(ctx, "SELECT exponent FROM currency_definitions WHERE code=$1", currency).Scan(&exponent); err != nil {
		if err == pgx.ErrNoRows {
			return core.Decimal{}, core.Invalid("currency", "币种不在已支持的精度目录中")
		}
		return core.Decimal{}, err
	}
	return core.CheckCurrencyAmount(currency, value, map[string]int{currency: exponent})
}
func (s *Service) PublishDefinition(ctx context.Context, c core.Command, in DefinitionInput) (core.Result, error) {
	for i := range in.Steps {
		in.Steps[i].AssignedUserID = strings.ToLower(in.Steps[i].AssignedUserID)
	}
	if err := validateDefinition(in); err != nil {
		return core.Result{}, err
	}
	c.AuthorizationMutation = true
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		if err := identity.RequireInteractiveSession(ctx, tx, c.Actor, true); err != nil {
			return err
		}
		return authz.RequireOrganization(ctx, tx, c.Actor, "config.manage")
	}, func(tx pgx.Tx) (core.Result, error) {
		minimum, err := currencyAmount(ctx, tx, in.Currency, in.MinAmount)
		if err != nil {
			return core.Result{}, err
		}
		var maximum *string
		if in.MaxAmount != nil {
			upper, e := currencyAmount(ctx, tx, in.Currency, *in.MaxAmount)
			if e != nil {
				return core.Result{}, e
			}
			if upper.Cmp(minimum) <= 0 {
				return core.Result{}, core.Invalid("max_amount", "金额上界必须大于含下界的起点")
			}
			text := upper.String()
			maximum = &text
		}
		var current int64
		if err = tx.QueryRow(ctx, "SELECT coalesce(max(revision),0) FROM workflow_definitions WHERE org_id=$1 AND key=$2", c.Actor.OrgID, in.Key).Scan(&current); err != nil {
			return core.Result{}, err
		}
		if current != in.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		for _, step := range in.Steps {
			var valid bool
			if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE org_id=$1 AND id=$2 AND (NOT $3 OR status='active'))", c.Actor.OrgID, step.AssignedUserID, in.Enabled).Scan(&valid); err != nil {
				return core.Result{}, err
			}
			if !valid {
				return core.Result{}, core.Invalid("assigned_user_id", "启用流程必须指定本组织当前有效的审批人")
			}
		}
		if in.Enabled {
			var overlap bool
			err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workflow_definitions d WHERE d.org_id=$1 AND d.key<>$2
                AND d.enabled AND d.kind=$3 AND d.currency=$4 AND (d.category=$5 OR d.category='*' OR $5='*')
                AND (d.max_amount IS NULL OR d.max_amount>$6::numeric) AND ($7::numeric IS NULL OR d.min_amount<$7::numeric)
                AND NOT EXISTS(SELECT 1 FROM workflow_definitions later WHERE later.org_id=d.org_id AND later.key=d.key AND later.revision>d.revision))`,
				c.Actor.OrgID, in.Key, in.Kind, in.Currency, in.Category, minimum.String(), maximum).Scan(&overlap)
			if err != nil {
				return core.Result{}, err
			}
			if overlap {
				return core.Result{}, core.Fail(409, "WORKFLOW_RULE_OVERLAP", "与另一条当前启用规则的类型、币种、类别及金额区间重叠")
			}
		}
		id := core.NewID()
		steps, err := json.Marshal(in.Steps)
		if err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO workflow_definitions(id,org_id,key,revision,name,kind,category,currency,min_amount,max_amount,enabled,
            self_approval_policy,withdrawal_allowed,published_by,reason,steps) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
			id, c.Actor.OrgID, in.Key, current+1, strings.TrimSpace(in.Name), in.Kind, in.Category, in.Currency, minimum.String(), maximum,
			in.Enabled, in.SelfApprovalPolicy, in.WithdrawalAllowed, c.Actor.UserID, in.Reason, steps); err != nil {
			return core.Result{}, err
		}
		for i, step := range in.Steps {
			if _, err = tx.Exec(ctx, "INSERT INTO workflow_definition_steps(org_id,definition_id,step_no,title,assigned_user_id,requires_mfa) VALUES($1,$2,$3,$4,$5,$6)",
				c.Actor.OrgID, id, i+1, step.Title, step.AssignedUserID, step.RequiresMFA); err != nil {
				return core.Result{}, err
			}
		}
		d, err := scanDefinition(tx.QueryRow(ctx, "SELECT "+columns+" FROM workflow_definitions d WHERE d.id=$1", id))
		if err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", c.Actor.OrgID); err != nil {
			return core.Result{}, err
		}
		return core.Result{Status: 201, Data: d, Event: core.Event{Type: "workflow.definition.published", ResourceType: "workflow_definition", ResourceID: id, Version: d.Version,
			Payload: map[string]any{"key": in.Key, "reason": in.Reason}}}, nil
	})
}

// SelectDefinition runs in the later submission transaction. It deliberately
// rechecks ambiguity even though the publishing path rejects overlapping rules.
func SelectDefinition(ctx context.Context, tx pgx.Tx, org, kind, category, currency, amount string) (Definition, error) {
	value, err := currencyAmount(ctx, tx, currency, amount)
	if err != nil {
		return Definition{}, err
	}
	rows, err := tx.Query(ctx, "SELECT "+columns+` FROM workflow_definitions d WHERE d.org_id=$1 AND d.kind=$2 AND d.currency=$3
        AND (d.category=$4 OR d.category='*') AND d.enabled AND d.min_amount<=$5::numeric AND (d.max_amount IS NULL OR d.max_amount>$5::numeric)
        AND NOT EXISTS(SELECT 1 FROM workflow_definitions later WHERE later.org_id=d.org_id AND later.key=d.key AND later.revision>d.revision)
        ORDER BY d.key LIMIT 2`, org, kind, currency, category, value.String())
	if err != nil {
		return Definition{}, err
	}
	var matches []Definition
	for rows.Next() {
		d, e := scanDefinition(rows)
		if e != nil {
			rows.Close()
			return Definition{}, e
		}
		matches = append(matches, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return Definition{}, err
	}
	if len(matches) == 0 {
		return Definition{}, core.Fail(409, "WORKFLOW_RULE_REQUIRED", "没有匹配的已启用审批制度")
	}
	if len(matches) != 1 {
		return Definition{}, core.Fail(409, "WORKFLOW_RULE_AMBIGUOUS", "审批规则存在歧义，须先修订制度")
	}
	if len(matches[0].Steps) == 0 {
		return Definition{}, core.Fail(409, "WORKFLOW_STEPS_REQUIRED", "审批制度缺少明确顺序步骤")
	}
	for _, step := range matches[0].Steps {
		if !step.AssignedUserActive {
			return Definition{}, core.Fail(409, "WORKFLOW_APPROVER_UNAVAILABLE", "制度指定的审批人已停用，不能自动跳过")
		}
	}
	return matches[0], nil
}
