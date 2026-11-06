package agent

import (
	"copypaste.hk/platform/internal/core"
	"strings"
)

// External human assessments remain external facts. Their version, complete
// dataset/result digests and immutable reference are preserved, never inferred
// from a successful model run or from a local synthetic check.
type SkillEvaluation struct {
	SetVersion    string `json:"set_version"`
	SetSHA256     string `json:"set_sha256"`
	ResultsSHA256 string `json:"results_sha256"`
	Reference     string `json:"reference"`
	Passed        int    `json:"passed"`
	Failed        int    `json:"failed"`
	Skipped       int    `json:"skipped"`
}

func (v SkillEvaluation) Validate(requirePass bool) error {
	sha := func(s string) bool { return len(s) == 64 && strings.Trim(s, "0123456789abcdef") == "" }
	if core.Name(v.SetVersion) != nil || core.Name(v.Reference) != nil || !sha(v.SetSHA256) || !sha(v.ResultsSHA256) || v.Passed < 0 || v.Failed < 0 || v.Skipped < 0 || v.Passed+v.Failed+v.Skipped < 1 || v.Passed+v.Failed+v.Skipped > 10000 || requirePass && (v.Passed < 1 || v.Failed != 0 || v.Skipped != 0) {
		return core.Invalid("evaluation", "请固定评测集版本、完整摘要、结果引用及真实评定数量；正式发布须全部通过")
	}
	return nil
}
