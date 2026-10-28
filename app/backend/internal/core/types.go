package core

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var idPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func NewID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("secure randomness unavailable")
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
func ValidID(id string) bool { return idPattern.MatchString(id) }
func Digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func CanonicalJSON(b []byte) ([]byte, error) {
	var v any
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.UseNumber()
	if err := d.Decode(&v); err != nil {
		return nil, err
	}
	return json.Marshal(v)
}
func Classification(s string) bool {
	return s == "internal" || s == "project_restricted" || s == "confidential"
}
func Name(s string) error {
	n := len([]rune(strings.TrimSpace(s)))
	if n < 1 || n > 200 {
		return Invalid("name", "名称需为1至200个字符")
	}
	return nil
}
func Date(s string) error {
	if s == "" {
		return nil
	}
	if _, err := time.Parse(time.DateOnly, s); err != nil {
		return Invalid("date", "日期格式应为YYYY-MM-DD")
	}
	return nil
}
func DateRange(start, end string) error {
	if err := Date(start); err != nil {
		return err
	}
	if err := Date(end); err != nil {
		return err
	}
	if start != "" && end != "" && start > end {
		return Invalid("due_date", "结束日期不能早于开始日期")
	}
	return nil
}

type Actor struct {
	OrgID          string     `json:"-"`
	UserID         string     `json:"id"`
	SessionID      string     `json:"-"`
	DisplayName    string     `json:"display_name"`
	AuthVersion    int64      `json:"-"`
	OrgAuthVersion int64      `json:"-"`
	Channel        string     `json:"-"`
	MFAVerifiedAt  *time.Time `json:"-"`
	RunID          string     `json:"-"`
}

// CursorBinding preserves authority and domain boundaries without ambiguous
// delimiter concatenation. Actor's public JSON deliberately omits these fields.
func CursorBinding(a Actor, scope ...string) string {
	value, _ := json.Marshal(struct {
		Org, User, Channel, Run string
		UserVersion, OrgVersion int64
		Scope                   []string
	}{a.OrgID, a.UserID, a.Channel, a.RunID, a.AuthVersion, a.OrgAuthVersion, scope})
	return Digest(value)
}

type Command struct {
	AuthorizationMutation bool
	// Configuration writes that update the organization must acquire this lock
	// before any shared authorization lock, avoiding lock-upgrade deadlocks.
	ExclusiveOrganizationLock bool
	Actor                     Actor
	RequestID                 string
	Route                     string
	Key                       string
	Body                      []byte
}
type Event struct {
	BeforeDigest string
	Type         string
	ResourceType string
	ResourceID   string
	Version      int64
	Payload      map[string]any
}
type Result struct {
	Status      int
	Data        any
	Event       Event
	Events      []Event
	OperationID string
	Replayed    bool
}
type Page[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"next_cursor"`
}
type Nullable[T any] struct {
	Set   bool
	Value *T
}

func (o *Nullable[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Value = nil
		return nil
	}
	var value T
	if err := json.Unmarshal(b, &value); err != nil {
		return err
	}
	o.Value = &value
	return nil
}
func RequireVersion(version int64) error {
	if version < 1 || version > 9007199254740991 {
		return Invalid("expected_version", "请提供有效的资料版本")
	}
	return nil
}
func UnexpectedState(from, to string) *Error {
	return Fail(409, "INVALID_TRANSITION", fmt.Sprintf("不能由%s变更为%s", from, to))
}
