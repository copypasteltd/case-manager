package agent

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"unicode/utf8"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/fileref"
	"copypaste.hk/platform/internal/project"
	"github.com/jackc/pgx/v5"
)

func parseDeliverableSubmission(base mutationArguments, raw json.RawMessage, fields map[string]json.RawMessage) (mutationArguments, string, json.RawMessage, error) {
	required := []string{"id", "expected_version", "expected_revision_id", "file_version_ids", "description", "reason"}
	var in project.DeliverableSubmit
	var typed struct {
		ID string `json:"id"`
		project.DeliverableSubmit
	}
	if len(fields) != len(required) || strictArguments(raw, &typed) != nil {
		return base, "", nil, core.Invalid("mutation", "请完整核对交付、基础版本、原修订、正式文件版本、说明和原因")
	}
	in = typed.DeliverableSubmit
	for key, value := range fields {
		if !slices.Contains(required, key) || (key != "expected_revision_id" && string(value) == "null") {
			return base, "", nil, core.Invalid("mutation", "交付提交不得省略或增加授权字段")
		}
	}
	if in.ExpectedRevisionID != nil && !exactOAID(*in.ExpectedRevisionID) || len(in.FileVersionIDs) < 1 || len(in.FileVersionIDs) > 20 || strings.TrimSpace(in.Description) == "" || utf8.RuneCountInString(in.Description) > 20000 {
		return base, "", nil, core.Invalid("mutation", "请选择1至20个不同正式版本及有限交付说明")
	}
	seen := map[string]bool{}
	for _, id := range in.FileVersionIDs {
		if !exactOAID(id) || seen[id] {
			return base, "", nil, core.Invalid("file_version_ids", "正式文件版本必须准确且不重复")
		}
		seen[id] = true
	}
	return base, "deliverable", raw, nil
}

// Submission reads only exact selected attachment metadata. It does not read
// binary content, require an extractor, or treat a valid scan waiver as clean.
// Ordinary attachment rules admit company files and this project's files;
// every current recipient must independently admit the same AI metadata.
func deliverableSubmissionFiles(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, m mutationRecord, members []string) ([]fileref.Evidence, error) {
	var in project.DeliverableSubmit
	if err := json.Unmarshal(m.Arguments, &in); err != nil {
		return nil, err
	}
	var classification string
	if err := tx.QueryRow(ctx, "SELECT classification FROM projects WHERE org_id=$1 AND id=$2", a.OrgID, m.ProjectID).Scan(&classification); err != nil {
		return nil, err
	}
	if classRank(classification) > classRank(c.Classification) {
		return nil, core.Missing()
	}
	if _, err := tx.Exec(ctx, `SELECT id FROM files WHERE org_id=$1 AND id IN
 (SELECT file_id FROM file_versions WHERE org_id=$1 AND id=ANY($2::uuid[])) ORDER BY id FOR SHARE`, a.OrgID, in.FileVersionIDs); err != nil {
		return nil, err
	}
	out := []fileref.Evidence{}
	users := append([]string{a.UserID}, members...)
	for _, id := range in.FileVersionIDs {
		var evidence fileref.Evidence
		for _, user := range users {
			target := a
			target.UserID = user
			var err error
			evidence, err = fileref.ForApproval(ctx, tx, target, id, &m.ProjectID, classification)
			if err != nil {
				return nil, err
			}
			var sourceClass string
			if err = tx.QueryRow(ctx, "SELECT classification FROM files WHERE org_id=$1 AND id=$2", a.OrgID, evidence.FileID).Scan(&sourceClass); err != nil {
				return nil, err
			}
			if classRank(sourceClass) > classRank(c.Classification) {
				return nil, core.Missing()
			}
		}
		out = append(out, evidence)
	}
	return out, nil
}

func deliverableSubmissionAudience(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, m mutationRecord, members []string) error {
	if m.Tool != "submit_deliverable" {
		return nil
	}
	current, err := deliverableSubmissionFiles(ctx, tx, a, c, m, members)
	if err != nil || len(m.Before) == 0 {
		return err
	}
	var stored struct {
		Files []fileref.Evidence `json:"submission_files"`
	}
	if json.Unmarshal(m.Before, &stored) != nil {
		return runtimeFailure("RUNTIME_SOURCE_CHANGED")
	}
	before, _ := json.Marshal(stored.Files)
	after, _ := json.Marshal(current)
	if string(before) != string(after) {
		return runtimeFailure("RUNTIME_SOURCE_CHANGED")
	}
	return nil
}

func prepareDeliverableSubmission(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, m *mutationRecord, members []string) error {
	if m.Tool != "submit_deliverable" {
		return nil
	}
	var in project.DeliverableSubmit
	_ = json.Unmarshal(m.Arguments, &in)
	var before map[string]json.RawMessage
	if err := json.Unmarshal(m.Before, &before); err != nil {
		return err
	}
	var state string
	var revision *string
	_ = json.Unmarshal(before["state"], &state)
	_ = json.Unmarshal(before["current_revision_id"], &revision)
	if !slices.Contains([]string{"draft", "changes_requested", "accepted"}, state) {
		return core.Fail(409, "DELIVERABLE_NOT_SUBMITTABLE", "交付待验收，不能替换其固定依据")
	}
	if (revision == nil) != (in.ExpectedRevisionID == nil) || revision != nil && *revision != *in.ExpectedRevisionID {
		return core.Conflict()
	}
	files, err := deliverableSubmissionFiles(ctx, tx, a, c, *m, members)
	if err != nil {
		return err
	}
	before["submission_files"], _ = json.Marshal(files)
	m.Before, err = json.Marshal(before)
	return err
}

func deliverableSubmissionReceipt(m mutationRecord, raw json.RawMessage) bool {
	var r project.DeliverableReceipt
	var in project.DeliverableSubmit
	if json.Unmarshal(raw, &r) != nil || json.Unmarshal(m.Arguments, &in) != nil {
		return false
	}
	return r.ID == m.ResourceID && r.Version == m.BaseVersion+1 && r.RevisionID != nil && exactOAID(*r.RevisionID) && (in.ExpectedRevisionID == nil || *in.ExpectedRevisionID != *r.RevisionID)
}
