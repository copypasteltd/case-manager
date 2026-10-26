package files

import (
	"context"
	"path"
	"strings"
	"unicode/utf8"

	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
)

const AgentCreateMaxBytes = 48 * 1024

// Creation fixes an existing directory and name, never an implicit overwrite.
func AgentCreateTarget(ctx context.Context, tx pgx.Tx, a core.Actor, folderID string, version int64, name, media string, collision bool) (Folder, error) {
	var out Folder
	if !core.ValidID(a.RunID) || !organizationID(folderID) || core.RequireVersion(version) != nil {
		return out, core.Invalid("file_create", "请选择准确目标目录和目录修订")
	}
	key, err := core.StorageNameKey(name)
	if err != nil {
		return out, err
	}
	ext := strings.ToLower(path.Ext(name))
	if !((media == "text/plain" && ext == ".txt") || (media == "text/markdown" && (ext == ".md" || ext == ".markdown"))) {
		return out, core.Invalid("media_type", "秘书新建支持.txt文本或.md／.markdown文档，格式须与文件名一致")
	}
	out, err = uploadTarget(ctx, tx, a, folderID, nil)
	if err != nil {
		return out, err
	}
	if !writable(ctx, tx, a.OrgID, out.ProjectID) {
		return out, core.Fail(409, "PROJECT_READ_ONLY", "项目当前阶段不允许新建文件")
	}
	if out.Version != version {
		return out, core.Fail(409, "FILE_CREATE_TARGET_CHANGED", "目标目录已变化，请重新核对新建位置")
	}
	if collision {
		if err = namespaceLock(ctx, tx, a, out.ProjectID); err != nil {
			return out, err
		}
		var exists bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM storage_entries WHERE org_id=$1 AND namespace_key=$2 AND name_key=$3)", a.OrgID, "folder/"+folderID, key).Scan(&exists); err != nil {
			return out, err
		}
		if exists {
			return out, core.Fail(409, "FILE_CREATE_NAME_CONFLICT", "目标目录已有同名资料；新建操作不会覆盖，请更换名称后重新发送")
		}
	}
	return out, nil
}

func AgentCreateText(text string) (AgentTextDiff, error) {
	var out AgentTextDiff
	if len(text) < 1 || len(text) > AgentCreateMaxBytes || !utf8.ValidString(text) || strings.ContainsRune(text, 0) || agentLineEnding(text) == "mixed" {
		return out, core.Invalid("content", "新建正文需为1字节至48KiB的UTF-8文本，不能含NUL、裸CR或混合换行")
	}
	runes := []rune(text)
	return AgentTextDiff{AddedChars: len(runes), RemovedSHA256: core.Digest(nil), AddedSHA256: core.Digest([]byte(text)), After: string(runes[:min(1000, len(runes))]), BeforeComplete: true, AfterComplete: len(runes) <= 1000, LineEnding: agentLineEnding(text), UTF8BOM: strings.HasPrefix(text, "\ufeff")}, nil
}
