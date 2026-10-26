package platform

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"copypaste.hk/platform/internal/core"
)

type Cursor struct {
	ID        string    `json:"id"`
	UpdatedAt time.Time `json:"updated_at"`
	Binding   string    `json:"binding"`
}
type Pager struct{ Key []byte }

func (p Pager) Encode(c Cursor) string {
	payload, _ := json.Marshal(c)
	mac := hmac.New(sha256.New, p.Key)
	mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (p Pager) Decode(s, binding string) (Cursor, error) {
	var c Cursor
	parts := strings.Split(s, ".")
	if len(parts) != 2 || len(s) > 2048 {
		return c, core.Fail(400, "INVALID_CURSOR", "分页标识无效")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return c, core.Fail(400, "INVALID_CURSOR", "分页标识无效")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return c, core.Fail(400, "INVALID_CURSOR", "分页标识无效")
	}
	mac := hmac.New(sha256.New, p.Key)
	mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) || json.Unmarshal(payload, &c) != nil || c.Binding != binding || !core.ValidID(c.ID) || c.UpdatedAt.IsZero() {
		return c, core.Fail(400, "INVALID_CURSOR", "分页标识无效或筛选已改变")
	}
	return c, nil
}
