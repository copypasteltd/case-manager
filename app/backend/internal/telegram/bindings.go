package telegram

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

// 16 UUID bytes plus a 16-byte MAC fit Telegram's 64-character /start limit.
// Tokens are recreated only for their original live portal login. They never
// enter the operation ledger, audit payload, inbox text or a group response.
func (s *Service) startToken(c Challenge, org, user, session string) string {
	id, _ := hex.DecodeString(strings.ReplaceAll(c.ID, "-", ""))
	mac := hmac.New(sha256.New, s.Config.MFAKey)
	fmt.Fprintf(mac, "copypaste/telegram-binding/v1\n%s\n%s\n%s\n%s\n%s\n%d", c.ID, org, user, session, c.BotID, c.ExpiresAt.UnixMicro())
	return base64.RawURLEncoding.EncodeToString(append(id, mac.Sum(nil)[:16]...))
}
func tokenID(token string) (string, bool) {
	bytes, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(bytes) != 32 || base64.RawURLEncoding.EncodeToString(bytes) != token {
		return "", false
	}
	id := hex.EncodeToString(bytes[:16])
	return id[:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:], true
}
func (s *Service) CreateChallenge(ctx context.Context, c core.Command, in ChallengeCreate) (core.Result, error) {
	if core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("reason", "请填写绑定原因")
	}
	if _, ok := s.bot(c.Actor.OrgID, in.BotID); !ok {
		return core.Result{}, core.Fail(503, "TELEGRAM_NOT_CONFIGURED", "当前Bot尚未配置接收入口")
	}
	return (platform.Commands{DB: s.DB}).Do(ctx, c, func(tx pgx.Tx) error { return authority(ctx, tx, c.Actor, false, true) }, func(tx pgx.Tx) (core.Result, error) {
		if err := ownerLock(ctx, tx, c.Actor.OrgID, c.Actor.UserID, in.BotID); err != nil {
			return core.Result{}, err
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM telegram_bindings WHERE org_id=$1 AND user_id=$2 AND bot_id=$3 AND state='active')`, c.Actor.OrgID, c.Actor.UserID, in.BotID).Scan(&exists); err != nil {
			return core.Result{}, err
		}
		if exists {
			return core.Result{}, core.Fail(409, "TELEGRAM_ALREADY_BOUND", "请先撤销当前Bot的绑定")
		}
		if _, err := tx.Exec(ctx, `UPDATE telegram_binding_challenges SET cancelled_at=clock_timestamp(),version=version+1 WHERE org_id=$1 AND user_id=$2 AND bot_id=$3 AND cancelled_at IS NULL AND confirmed_at IS NULL`, c.Actor.OrgID, c.Actor.UserID, in.BotID); err != nil {
			return core.Result{}, err
		}
		id := core.NewID()
		value, err := scanChallenge(tx.QueryRow(ctx, `INSERT INTO telegram_binding_challenges(id,org_id,user_id,bot_id,session_id,actor_auth_version,expires_at) VALUES($1,$2,$3,$4,$5,$6,clock_timestamp()+interval '10 minutes') RETURNING `+challengeColumns, id, c.Actor.OrgID, c.Actor.UserID, in.BotID, c.Actor.SessionID, c.Actor.AuthVersion))
		if err != nil {
			return core.Result{}, err
		}
		return eventResult(value, "telegram.binding_challenge_created", id, in.Reason, value.Version, true), nil
	})
}
func (s *Service) DecorateChallenge(ctx context.Context, a core.Actor, result core.Result) (core.Result, error) {
	previous, err := decodeResult[Challenge](result.Data)
	if err != nil {
		return result, err
	}
	tx, err := s.snapshot(ctx, a, false)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	value, err := scanChallenge(tx.QueryRow(ctx, "SELECT "+challengeColumns+" FROM telegram_binding_challenges WHERE org_id=$1 AND user_id=$2 AND id=$3", a.OrgID, a.UserID, previous.ID))
	if err != nil {
		return result, err
	}
	var session string
	if err = tx.QueryRow(ctx, "SELECT session_id FROM telegram_binding_challenges WHERE org_id=$1 AND id=$2", a.OrgID, value.ID).Scan(&session); err != nil {
		return result, err
	}
	bot, ok := s.bot(a.OrgID, value.BotID)
	var usable bool
	if err = tx.QueryRow(ctx, `SELECT expires_at>clock_timestamp() AND claimed_at IS NULL AND confirmed_at IS NULL AND cancelled_at IS NULL FROM telegram_binding_challenges WHERE id=$1`, value.ID).Scan(&usable); err != nil {
		return result, err
	}
	if usable && session == a.SessionID && ok {
		if err = identity.RequireInteractiveSession(ctx, tx, a, true); err != nil {
			return result, err
		}
		value.StartToken = s.startToken(value, a.OrgID, a.UserID, session)
		value.StartURL = "https://t.me/" + bot.Username + "?start=" + value.StartToken
	}
	result.Data = value
	return result, nil
}
func (s *Service) ConfirmChallenge(ctx context.Context, c core.Command, id string, in ChallengeConfirm) (core.Result, error) {
	if !core.ValidID(id) {
		return core.Result{}, core.Missing()
	}
	if core.Name(in.Reason) != nil || core.RequireVersion(in.ExpectedVersion) != nil || !numericID(in.TelegramUserID, false) {
		return core.Result{}, core.Invalid("binding", "请核对当前版本、数字Telegram身份和原因")
	}
	return (platform.Commands{DB: s.DB}).Do(ctx, c, func(tx pgx.Tx) error { return authority(ctx, tx, c.Actor, false, true) }, func(tx pgx.Tx) (core.Result, error) {
		challenge, err := scanChallenge(tx.QueryRow(ctx, "SELECT "+challengeColumns+" FROM telegram_binding_challenges WHERE org_id=$1 AND user_id=$2 AND id=$3", c.Actor.OrgID, c.Actor.UserID, id))
		if err != nil {
			return core.Result{}, err
		}
		if _, ok := s.bot(c.Actor.OrgID, challenge.BotID); !ok {
			return core.Result{}, core.Fail(503, "TELEGRAM_NOT_CONFIGURED", "当前Bot尚未配置接收入口")
		}
		if err = ownerLock(ctx, tx, c.Actor.OrgID, c.Actor.UserID, challenge.BotID); err != nil {
			return core.Result{}, err
		}
		challenge, err = scanChallenge(tx.QueryRow(ctx, "SELECT "+challengeColumns+" FROM telegram_binding_challenges WHERE org_id=$1 AND user_id=$2 AND id=$3 FOR UPDATE", c.Actor.OrgID, c.Actor.UserID, id))
		if err != nil {
			return core.Result{}, err
		}
		if challenge.Version != in.ExpectedVersion {
			return core.Result{}, core.Fail(409, "VERSION_CONFLICT", "绑定证明已更新，请重新读取")
		}
		var valid bool
		if err = tx.QueryRow(ctx, `SELECT expires_at>clock_timestamp() AND session_id=$2::uuid AND actor_auth_version=$3 FROM telegram_binding_challenges WHERE id=$1`, id, c.Actor.SessionID, c.Actor.AuthVersion).Scan(&valid); err != nil {
			return core.Result{}, err
		}
		if !valid || challenge.ClaimedAt == nil || challenge.TelegramUserID == nil || *challenge.TelegramUserID != in.TelegramUserID || challenge.CancelledAt != nil || challenge.ConfirmedAt != nil {
			return core.Result{}, core.Fail(409, "TELEGRAM_PROOF_INVALID", "请在原登录中核对未过期的私聊身份证明")
		}
		if _, err = tx.Exec(ctx, `INSERT INTO telegram_identity_owners(bot_id,telegram_user_id,org_id,user_id) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, challenge.BotID, in.TelegramUserID, c.Actor.OrgID, c.Actor.UserID); err != nil {
			return core.Result{}, err
		}
		var matches bool
		if err = tx.QueryRow(ctx, `SELECT org_id=$3::uuid AND user_id=$4::uuid FROM telegram_identity_owners WHERE bot_id=$1 AND telegram_user_id=$2`, challenge.BotID, in.TelegramUserID, c.Actor.OrgID, c.Actor.UserID).Scan(&matches); err != nil {
			return core.Result{}, err
		}
		if !matches {
			return core.Result{}, core.Fail(409, "TELEGRAM_IDENTITY_IN_USE", "该数字身份不能用于此成员，请核对绑定归属")
		}
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM telegram_bindings WHERE org_id=$1 AND user_id=$2 AND bot_id=$3 AND state='active')`, c.Actor.OrgID, c.Actor.UserID, challenge.BotID).Scan(&exists); err != nil {
			return core.Result{}, err
		}
		if exists {
			return core.Result{}, core.Fail(409, "TELEGRAM_ALREADY_BOUND", "请先撤销当前绑定")
		}
		bindingID := core.NewID()
		value, err := scanBinding(tx.QueryRow(ctx, `INSERT INTO telegram_bindings(id,org_id,user_id,bot_id,telegram_user_id,challenge_id,state) VALUES($1,$2,$3,$4,$5,$6,'active') RETURNING `+bindingColumns, bindingID, c.Actor.OrgID, c.Actor.UserID, challenge.BotID, in.TelegramUserID, id))
		if err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE telegram_binding_challenges SET confirmed_at=clock_timestamp(),version=version+1 WHERE id=$1`, id); err != nil {
			return core.Result{}, err
		}
		return eventResult(value, "telegram.binding_confirmed", value.ID, in.Reason, value.Version, true), nil
	})
}
func (s *Service) RevokeBinding(ctx context.Context, c core.Command, id string, in ReasonInput) (core.Result, error) {
	if !core.ValidID(id) {
		return core.Result{}, core.Missing()
	}
	if core.Name(in.Reason) != nil || core.RequireVersion(in.ExpectedVersion) != nil {
		return core.Result{}, core.Invalid("reason", "请填写撤销原因并核对版本")
	}
	return (platform.Commands{DB: s.DB}).Do(ctx, c, func(tx pgx.Tx) error { return authority(ctx, tx, c.Actor, false, false) }, func(tx pgx.Tx) (core.Result, error) {
		value, err := scanBinding(tx.QueryRow(ctx, "SELECT "+bindingColumns+" FROM telegram_bindings WHERE org_id=$1 AND user_id=$2 AND id=$3 FOR UPDATE", c.Actor.OrgID, c.Actor.UserID, id))
		if err != nil {
			return core.Result{}, err
		}
		if value.Version != in.ExpectedVersion {
			return core.Result{}, core.Fail(409, "VERSION_CONFLICT", "绑定已更新，请重新读取")
		}
		if value.State != "active" {
			return core.Result{}, core.Fail(409, "TELEGRAM_BINDING_REVOKED", "此绑定已撤销")
		}
		value, err = scanBinding(tx.QueryRow(ctx, `UPDATE telegram_bindings SET state='revoked',revoked_at=clock_timestamp(),version=version+1 WHERE id=$1 RETURNING `+bindingColumns, id))
		if err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE telegram_inbox SET state='withdrawn',reason_code='binding_revoked' WHERE org_id=$1 AND binding_id=$2 AND (state='awaiting_executor' OR (state='dispatched' AND EXISTS(SELECT 1 FROM telegram_run_links l JOIN agent_runs r ON r.org_id=l.org_id AND r.id=l.run_id WHERE l.org_id=telegram_inbox.org_id AND l.inbox_id=telegram_inbox.id AND r.state IN ('queued','running','awaiting_approval','committing'))))`, c.Actor.OrgID, id); err != nil {
			return core.Result{}, err
		}
		return eventResult(value, "telegram.binding_revoked", id, in.Reason, value.Version, false), nil
	})
}
func (s *Service) CancelChallenge(ctx context.Context, c core.Command, id string, in ReasonInput) (core.Result, error) {
	if !core.ValidID(id) {
		return core.Result{}, core.Missing()
	}
	if core.RequireVersion(in.ExpectedVersion) != nil || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("reason", "请核对版本并填写取消原因")
	}
	return (platform.Commands{DB: s.DB}).Do(ctx, c, func(tx pgx.Tx) error { return authority(ctx, tx, c.Actor, false, false) }, func(tx pgx.Tx) (core.Result, error) {
		value, err := scanChallenge(tx.QueryRow(ctx, "SELECT "+challengeColumns+" FROM telegram_binding_challenges WHERE org_id=$1 AND user_id=$2 AND id=$3 FOR UPDATE", c.Actor.OrgID, c.Actor.UserID, id))
		if err != nil {
			return core.Result{}, err
		}
		if value.Version != in.ExpectedVersion {
			return core.Result{}, core.Fail(409, "VERSION_CONFLICT", "绑定证明已更新，请重新读取")
		}
		if value.ConfirmedAt != nil || value.CancelledAt != nil {
			return core.Result{}, core.Fail(409, "TELEGRAM_PROOF_INVALID", "此绑定流程已经结束")
		}
		value, err = scanChallenge(tx.QueryRow(ctx, `UPDATE telegram_binding_challenges SET cancelled_at=clock_timestamp(),version=version+1 WHERE id=$1 RETURNING `+challengeColumns, id))
		if err != nil {
			return core.Result{}, err
		}
		return eventResult(value, "telegram.binding_challenge_cancelled", id, in.Reason, value.Version, false), nil
	})
}
