package workbench

// Task queues use ordinary access and assignment policy. Participation grants no access.
func taskQueries() []query {
	base := `SELECT t.id,t.version,NULL::uuid revision_id,t.project_id,NULL::uuid related_id,
 to_char(t.due_at AT TIME ZONE 'Asia/Hong_Kong','YYYY-MM-DD') due_on,t.title,t.state,t.updated_at
 FROM tasks t JOIN projects p ON p.org_id=t.org_id AND p.id=t.project_id
 WHERE t.org_id=$1 AND t.state NOT IN ('done','cancelled') AND p.state NOT IN ('archived','cancelled')
 AND can_access(t.org_id,$2,'task.read','task',t.id,p.id,t.creator_id,p.classification,$3,$4)`
	return []query{
		{"my_tasks", base + ` AND t.assignee_id=$2`},
		{"my_overdue_tasks", base + ` AND t.assignee_id=$2 AND t.due_at<transaction_timestamp()`},
		{"participating_tasks", base + ` AND $2=ANY(t.participant_ids)`},
		{"unassigned_tasks", base + ` AND t.assignee_id IS NULL AND p.state<>'closing'
   AND can_access(t.org_id,$2,'task.assign','task',t.id,p.id,t.creator_id,p.classification,$3,$4)
   AND can_access(t.org_id,$2,'task.update','task',t.id,p.id,t.creator_id,p.classification,$3,$4)`},
	}
}
