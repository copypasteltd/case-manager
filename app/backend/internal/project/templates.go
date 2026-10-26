package project

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
)

func (s *Service) applyTemplate(ctx context.Context, tx pgx.Tx, a core.Actor, projectID, rootID, ownerID, classification string, schema TemplateSchema) error {
	var err error
	schema, err = validTemplateSchema(schema)
	if err != nil {
		return err
	}
	// Reuse only the known standard logical directories. Other collisions fail.
	standard := map[string]bool{"合同": true, "交付": true, "工作资料": true}
	for _, name := range schema.Folders {
		key, _ := core.StorageNameKey(name)
		if standard[key] {
			continue
		}
		_, err = tx.Exec(ctx, `INSERT INTO folders(id,org_id,project_id,parent_id,root_scope,name,name_key,classification,state,creator_id)
   VALUES($1,$2,$3,$4,$5,$6,$7,$8,'active',$9)`, core.NewID(), a.OrgID, projectID, rootID, "project:"+projectID, name, key, classification, a.UserID)
		if err != nil {
			return err
		}
	}
	for i, t := range schema.Tasks {
		_, err = tx.Exec(ctx, `INSERT INTO tasks(id,org_id,project_id,title,body,priority,state,review_required,creator_id,sort_order)
   VALUES($1,$2,$3,$4,$5,$6,'todo',$7,$8,$9)`, core.NewID(), a.OrgID, projectID, t.Title, t.Body, t.Priority, t.ReviewRequired, a.UserID, i)
		if err != nil {
			return err
		}
	}
	for _, m := range schema.Milestones {
		_, err = tx.Exec(ctx, `INSERT INTO milestones(id,org_id,project_id,title,owner_id,state,creator_id)
   VALUES($1,$2,$3,$4,$5,'open',$6)`, core.NewID(), a.OrgID, projectID, m.Title, ownerID, a.UserID)
		if err != nil {
			return err
		}
	}
	return nil
}
