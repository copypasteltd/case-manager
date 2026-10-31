package workflow

// PersonalFollowupsSQL follows Requests' metadata authority, including the
// additional project-budget and original-purchase checks. It never reads the
// immutable payload, attachment names or another applicant's work.
const PersonalFollowupsSQL = `SELECT r.id,r.version,r.current_revision_id revision_id,r.project_id,
 r.title,r.state,r.updated_at FROM requests r ` + businessHeads + `
 WHERE r.org_id=$1 AND r.applicant_id=$2 AND r.state IN ('draft','returned')
 AND can_access(r.org_id,$2,'request.read','request',r.id,r.project_id,r.applicant_id,r.classification,$3,$4)
 AND can_access(b.org_id,$2,b.kind||'.read',b.kind,b.id,b.project_id,b.creator_id,b.classification,$3,$4)
 AND (b.kind<>'change' OR EXISTS(SELECT 1 FROM projects cp WHERE cp.org_id=b.org_id AND cp.id=b.project_id
 AND can_access(cp.org_id,$2,'budget.read','project',cp.id,cp.id,cp.creator_id,cp.classification,$3,$4)))
 AND (b.kind<>'purchase_variance' OR EXISTS(SELECT 1 FROM spend_documents p WHERE p.org_id=b.org_id AND p.id=b.purchase_id
 AND can_access(p.org_id,$2,'purchase.read','purchase',p.id,p.project_id,p.creator_id,p.classification,$3,$4)))`
