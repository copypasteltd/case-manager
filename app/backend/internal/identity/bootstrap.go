package identity

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"errors"
	"github.com/jackc/pgx/v5"
	"strings"
)

type BootstrapInput struct {
	Organization string
	Login        string
	DisplayName  string
	Password     string
}
type BootstrapResult struct {
	OrganizationID string `json:"organization_id"`
	UserID         string `json:"user_id"`
}

// System administrators receive configuration authority. Business content
// authority must be granted separately; even bootstrap does not bypass ACLs.
var systemActions = []string{"user.invite", "user.disable", "user.password_reset", "user.read", "role.read", "role.assign", "policy.manage", "config.manage", "skill.release", "file.import", "service.manage", "audit.read", "audit.export"}
var projectActions = []string{"project.create", "project.read", "project.update", "project.manage_members", "project.archive", "project.reopen",
	"task.read", "task.create", "task.update", "task.assign", "task.transition", "task.review", "task.reopen",
	"milestone.read", "milestone.create", "milestone.update", "deliverable.read", "deliverable.create", "deliverable.submit", "deliverable.decide",
	"risk.read", "risk.create", "risk.update", "issue.read", "issue.create", "issue.update", "change.read", "change.create", "change.update", "change.submit", "change.implement", "budget.read",
	"post.read", "post.write", "post.publish", "comment.write", "file.read", "file.upload", "file.update", "file.move", "file.trash", "file.restore", "folder.read", "folder.create", "folder.update"}

func Bootstrap(ctx context.Context, db platform.DBTX, input BootstrapInput) (BootstrapResult, error) {
	var result BootstrapResult
	if err := core.Name(input.Organization); err != nil {
		return result, err
	}
	if err := core.Name(input.DisplayName); err != nil {
		return result, err
	}
	input.Login = strings.ToLower(strings.TrimSpace(input.Login))
	if err := core.Name(input.Login); err != nil {
		return result, err
	}
	hash, err := HashPassword(input.Password)
	if err != nil {
		return result, err
	}
	// The caller must supply a transaction through the migration identity.
	tx, ok := db.(pgx.Tx)
	if !ok {
		return result, errors.New("bootstrap requires an explicit transaction")
	}
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(731924802)"); err != nil {
		return result, err
	}
	var exists bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM organizations)").Scan(&exists); err != nil {
		return result, err
	}
	if exists {
		return result, errors.New("organization already initialized; bootstrap cannot add authority")
	}
	result = BootstrapResult{core.NewID(), core.NewID()}
	org, user := result.OrganizationID, result.UserID
	if _, err = tx.Exec(ctx, "INSERT INTO organizations(id,name) VALUES($1,$2)", org, input.Organization); err != nil {
		return result, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO users(id,org_id,login_key,display_name,status,credential_hash) VALUES($1,$2,$3,$4,'active',$5)", user, org, input.Login, input.DisplayName, hash); err != nil {
		return result, err
	}
	for _, level := range []string{"internal", "project_restricted", "confidential"} {
		if _, err = tx.Exec(ctx, "INSERT INTO classification_policies(id,org_id,level,policy_version,ai_allowed) VALUES($1,$2,$3,1,false)", core.NewID(), org, level); err != nil {
			return result, err
		}
	}
	closeoutReadActions := []string{"project.read", "task.read", "milestone.read", "deliverable.read", "risk.read", "issue.read", "change.read", "budget.read", "contract.read", "purchase.read", "purchase_variance.read", "claim.read", "cost.read", "payment.read", "subscription.read", "party.read", "file.read", "folder.read"}
	roles := []struct {
		key, name string
		mfa       bool
		actions   []string
	}{
		{"system_admin", "系统管理员", true, systemActions}, {"project_manager", "项目负责人", false, projectActions},
		{"project_change_reader", "项目变更与预算查阅人", false, []string{"change.read", "budget.read"}},
		{"project_closeout_reader", "项目结项核查查阅人", false, closeoutReadActions},
		{"project_archiver", "项目人工归档人", true, append(append([]string{}, closeoutReadActions...), "project.archive")},
		{"project_change_implementer", "项目变更实施人", true, []string{"project.read", "project.update", "change.read", "change.implement", "budget.read"}},
		{"deliverable_reviewer", "项目交付验收人", true, []string{"deliverable.read", "deliverable.decide"}},
		{"file_scan_reviewer", "文件扫描例外审核人", true, []string{"file.scan.waive", "folder.read", "file.read"}},
		{"file_retention_manager", "文件保留管理人", true, []string{"file.retention.manage", "folder.read", "file.read"}},
		{"party_manager", "往来资料管理人", false, []string{"party.read", "party.create", "party.update", "party.archive"}},
		{"contract_manager", "合同管理人", false, []string{"contract.read", "contract.create", "contract.update", "contract.archive"}},
		{"request_manager", "申请管理人", false, []string{"request.read", "request.create", "request.update", "request.submit", "request.withdraw"}},
		{"approval_reviewer", "审批处理人", false, []string{"request.read", "approval.decide"}},
		{"contract_reader", "合同查阅人", false, []string{"contract.read"}},
		{"contract_reconciler", "合同对账人", true, []string{"contract.read", "contract.reconcile"}},
		{"contract_closer", "合同完成与终止登记人", true, []string{"contract.read", "contract.close"}},
		{"purchase_manager", "采购管理人", false, []string{"purchase.read", "purchase.create", "purchase.update", "purchase.archive"}},
		{"purchase_reader", "采购查阅人", false, []string{"purchase.read"}},
		{"purchase_operator", "采购履行登记人", true, []string{"purchase.read", "purchase.execute"}},
		{"purchase_variance_manager", "采购差异提案管理人", false, []string{"purchase.read", "purchase_variance.read", "purchase_variance.create", "purchase_variance.update"}},
		{"purchase_variance_reader", "采购差异查阅人", false, []string{"purchase_variance.read"}},
		{"purchase_plan_activator", "采购执行基线启用人", true, []string{"purchase.read", "purchase_variance.read", "purchase_variance.activate"}},
		{"purchase_completer", "采购完成核对人", true, []string{"purchase.read", "purchase.complete"}},
		{"cost_recorder", "费用登记人", false, []string{"cost.read", "cost.record"}},
		{"cost_confirmer", "费用确认人", true, []string{"cost.read", "cost.confirm"}},
		{"cost_reverser", "费用冲销人", true, []string{"cost.read", "cost.reverse"}},
		{"cost_reader", "费用查阅人", false, []string{"cost.read"}},
		{"finance_reporter", "财务报表查阅人", false, []string{"cost.read", "report.read"}},
		{"finance_exporter", "财务报表导出人", false, []string{"report.read", "report.export"}},
		{"finance_fx_manager", "财务汇率管理人", true, []string{"report.read", "finance.fx.manage"}},
		{"finance_amortization_manager", "费用摊销管理人", true, []string{"cost.read", "cost.amortize", "report.read"}},
		{"payment_recorder", "资金登记人", false, []string{"payment.read", "payment.record"}},
		{"payment_reverser", "资金冲销人", true, []string{"payment.read", "payment.reverse"}},
		{"payment_reader", "资金查阅人", false, []string{"payment.read"}},
		{"cash_reporter", "现金报表查阅人", false, []string{"payment.read", "report.read"}},
		{"subscription_manager", "订阅管理人", true, []string{"subscription.read", "subscription.create", "subscription.update", "subscription.manage"}},
		{"subscription_reader", "订阅查阅人", false, []string{"subscription.read"}},
		{"forecast_reporter", "预测报表查阅人", false, []string{"subscription.read", "report.read"}},
		{"claim_submitter", "报销填报人", false, []string{"claim.read", "claim.create", "claim.update", "claim.archive"}},
		{"claim_reader", "报销查阅人", false, []string{"claim.read"}},
		{"contract_operator", "合同签署与履行登记人", true, []string{"contract.read", "contract.execute"}},
		{"agent_user", "公司秘书使用人", false, []string{"conversation.read", "conversation.create", "conversation.update", "conversation.manage_members", "agent.run"}},
		{"agent_plan_operator", "秘书项目计划操作人", true, []string{"agent.plan.read", "agent.plan.propose", "agent.plan.approve", "agent.plan.commit", "agent.plan.revoke"}},
		{"status_reader", "服务状态查阅人", false, []string{"service.read"}},
		{"notification_operator", "通知投递运维", true, []string{"notification.delivery.read"}},
		{"status_operator", "服务状态维护人", true, []string{"service.read", "service.manage", "incident.update"}},
		{"file_importer", "文件迁移操作人", true, []string{"file.import", "folder.read", "file.read", "file.upload", "file.update"}},
		{"member", "项目成员", false, []string{"project.read", "task.read", "task.create", "task.update", "task.transition", "milestone.read", "risk.read", "issue.read", "deliverable.read", "post.read", "post.write", "comment.write", "file.read", "file.upload", "folder.read"}},
	}
	for _, role := range roles {
		id := core.NewID()
		if _, err = tx.Exec(ctx, "INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES($1,$2,$3,$4,$5)", id, org, role.key, role.name, role.mfa); err != nil {
			return result, err
		}
		for _, action := range role.actions {
			if _, err = tx.Exec(ctx, "INSERT INTO role_actions(org_id,role_id,action) VALUES($1,$2,$3)", org, id, action); err != nil {
				return result, err
			}
		}
		if role.key == "system_admin" {
			if _, err = tx.Exec(ctx, "INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) VALUES($1,$2,$3,$4,'org','confidential')", core.NewID(), org, user, id); err != nil {
				return result, err
			}
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,executor_id,channel,request_id,action,resource_type,resource_id,result,reason)
        VALUES($1,$2,$3,$3,'bootstrap',$4,'organization.initialized','organization',$2,'succeeded','Initial system administrator; no business role assigned')`, core.NewID(), org, user, core.NewID()); err != nil {
		return result, err
	}
	return result, nil
}
