package identity

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"errors"
	"github.com/jackc/pgx/v5"
)

type DevelopmentAccount struct {
	Login       string `json:"login"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}
type DevelopmentSeed struct {
	Accounts []DevelopmentAccount `json:"accounts"`
}

// SeedDevelopment is only called by the explicit development CLI command.
// It starts from an empty organization and never overwrites existing identities.
func SeedDevelopment(ctx context.Context, tx pgx.Tx, input DevelopmentSeed) ([]string, error) {
	if len(input.Accounts) != 3 {
		return nil, errors.New("development seed requires exactly three accounts")
	}
	initial := input.Accounts[0]
	boot, err := Bootstrap(ctx, tx, BootstrapInput{Organization: "CopyPaste 开发验证", Login: initial.Login, DisplayName: initial.DisplayName, Password: initial.Password})
	if err != nil {
		return nil, err
	}
	ids := []string{boot.UserID}
	for index, account := range input.Accounts[1:] {
		hash, err := HashPassword(account.Password)
		if err != nil {
			return nil, err
		}
		if err = core.Name(account.DisplayName); err != nil {
			return nil, err
		}
		if err = core.Name(account.Login); err != nil {
			return nil, err
		}
		id := core.NewID()
		if _, err = tx.Exec(ctx, "INSERT INTO users(id,org_id,login_key,display_name,status,credential_hash) VALUES($1,$2,$3,$4,'active',$5)", id, boot.OrganizationID, account.Login, account.DisplayName, hash); err != nil {
			return nil, err
		}
		role := "member"
		if index == 0 {
			role = "project_manager"
		}
		if _, err = tx.Exec(ctx, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type)
            SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key=$4`, core.NewID(), boot.OrganizationID, id, role); err != nil {
			return nil, err
		}
		if err = authAudit(ctx, tx, boot.OrganizationID, boot.UserID, core.NewID(), "development.identity_created", "succeeded", "bootstrap"); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}
