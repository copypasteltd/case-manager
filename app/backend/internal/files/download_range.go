package files

import (
	"strconv"
	"strings"

	"copypaste.hk/platform/internal/core"
)

// DownloadRange deliberately accepts a single closed range only. It never
// turns an invalid range or If-Range mismatch into a whole-object transfer.
func DownloadRange(value string, size int64) (offset, length int64, partial bool, err error) {
	bad := func(code, message string) (int64, int64, bool, error) {
		return 0, 0, false, core.Fail(416, code, message)
	}
	if size < 0 {
		return bad("DOWNLOAD_METADATA_INVALID", "文件大小无效")
	}
	if value == "" {
		if size > DownloadRangeBytes {
			return bad("DOWNLOAD_RANGE_REQUIRED", "大文件必须按每次最多32MiB的范围读取")
		}
		return 0, size, false, nil
	}
	if len(value) > 100 || !strings.HasPrefix(value, "bytes=") || strings.Contains(value, ",") {
		return bad("DOWNLOAD_RANGE_INVALID", "仅支持单个明确起止位置的字节范围")
	}
	first, last, ok := strings.Cut(strings.TrimPrefix(value, "bytes="), "-")
	if !ok || first == "" || last == "" {
		return bad("DOWNLOAD_RANGE_INVALID", "请提供完整的字节起止位置")
	}
	start, er1 := strconv.ParseInt(first, 10, 64)
	end, er2 := strconv.ParseInt(last, 10, 64)
	if er1 != nil || er2 != nil || strconv.FormatInt(start, 10) != first || strconv.FormatInt(end, 10) != last ||
		start < 0 || end < start || end >= size {
		return bad("DOWNLOAD_RANGE_INVALID", "读取范围超出固定版本")
	}
	if end-start+1 > DownloadRangeBytes {
		return bad("DOWNLOAD_RANGE_TOO_LARGE", "每次读取范围不能超过32MiB")
	}
	return start, end - start + 1, true, nil
}
