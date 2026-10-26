package telegram

import (
	"bytes"
	"encoding/json"
	"io"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"copypaste.hk/platform/internal/core"
)

type tgUser struct {
	ID    int64 `json:"id"`
	IsBot bool  `json:"is_bot"`
}
type tgChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}
type entity struct {
	Type   string  `json:"type"`
	Offset int     `json:"offset"`
	Length int     `json:"length"`
	User   *tgUser `json:"user"`
}
type message struct {
	ID         int64           `json:"message_id"`
	Date       int64           `json:"date"`
	From       *tgUser         `json:"from"`
	SenderChat json.RawMessage `json:"sender_chat"`
	Chat       tgChat          `json:"chat"`
	Topic      int64           `json:"message_thread_id"`
	Text       string          `json:"text"`
	Entities   []entity        `json:"entities"`
	Reply      *struct {
		From *tgUser `json:"from"`
	} `json:"reply_to_message"`
	NewMembers  []tgUser `json:"new_chat_members"`
	LeftMember  *tgUser  `json:"left_chat_member"`
	MigrateTo   int64    `json:"migrate_to_chat_id"`
	MigrateFrom int64    `json:"migrate_from_chat_id"`
}
type update struct {
	ID         int64
	Message    *message
	Membership *struct {
		Chat tgChat `json:"chat"`
	}
}

// Reject duplicate keys, multiple roots and unbounded nesting before any token
// or numeric identity is interpreted. Unknown Telegram fields remain supported.
func jsonNode(dec *json.Decoder, depth int) error {
	if depth > 32 {
		return core.Invalid("update", "消息嵌套超过限制")
	}
	token, err := dec.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for dec.More() {
			key, e := dec.Token()
			if e != nil {
				return e
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return core.Invalid("update", "消息包含重复字段")
			}
			seen[name] = true
			if e = jsonNode(dec, depth+1); e != nil {
				return e
			}
		}
	case '[':
		for dec.More() {
			if e := jsonNode(dec, depth+1); e != nil {
				return e
			}
		}
	default:
		return core.Invalid("update", "消息结构无效")
	}
	_, err = dec.Token()
	return err
}
func parseUpdate(raw []byte) (update, []byte, error) {
	var out update
	invalid := func() (update, []byte, error) {
		return out, nil, core.Invalid("update", "请提供单条有效的Telegram更新")
	}
	if len(raw) == 0 || len(raw) > 65536 || !utf8.Valid(raw) {
		return invalid()
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if jsonNode(decoder, 0) != nil {
		return invalid()
	}
	if _, err := decoder.Token(); err != io.EOF {
		return invalid()
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(raw, &root) != nil || root == nil || len(root) > 2 || strings.TrimSpace(string(root["update_id"])) == "null" || json.Unmarshal(root["update_id"], &out.ID) != nil || out.ID < 0 || out.ID > 9007199254740991 {
		return invalid()
	}
	for key, value := range root {
		if key == "update_id" {
			continue
		}
		switch key {
		case "message":
			if string(value) == "null" || json.Unmarshal(value, &out.Message) != nil {
				return invalid()
			}
		case "chat_member", "my_chat_member":
			if string(value) == "null" || json.Unmarshal(value, &out.Membership) != nil {
				return invalid()
			}
		}
	}
	canonical, err := core.CanonicalJSON(raw)
	if err != nil {
		return invalid()
	}
	return out, canonical, nil
}
func entityText(text string, e entity) (string, bool) {
	units := utf16.Encode([]rune(text))
	if e.Offset < 0 || e.Length <= 0 || e.Offset > len(units) || e.Length > len(units)-e.Offset {
		return "", false
	}
	end := e.Offset + e.Length
	if e.Offset > 0 && units[e.Offset] >= 0xDC00 && units[e.Offset] <= 0xDFFF && units[e.Offset-1] >= 0xD800 && units[e.Offset-1] <= 0xDBFF {
		return "", false
	}
	if end < len(units) && units[end] >= 0xDC00 && units[end] <= 0xDFFF && units[end-1] >= 0xD800 && units[end-1] <= 0xDBFF {
		return "", false
	}
	return string(utf16.Decode(units[e.Offset:end])), true
}
func triggered(m message, b BotConfig) bool {
	if m.Reply != nil && m.Reply.From != nil && m.Reply.From.IsBot && strconv.FormatInt(m.Reply.From.ID, 10) == b.ID {
		return true
	}
	for _, e := range m.Entities {
		text, ok := entityText(m.Text, e)
		if !ok {
			continue
		}
		switch e.Type {
		case "mention":
			if strings.EqualFold(text, "@"+b.Username) {
				return true
			}
		case "text_mention":
			if e.User != nil && e.User.IsBot && strconv.FormatInt(e.User.ID, 10) == b.ID {
				return true
			}
		case "bot_command":
			if e.Offset == 0 {
				command, target, has := strings.Cut(text, "@")
				if (command == "/ask" || command == "/brief" || command == "/incident") && (!has || strings.EqualFold(target, b.Username)) {
					return true
				}
			}
		}
	}
	return false
}
func startCode(m message, b BotConfig) (string, bool) {
	fields := strings.Fields(m.Text)
	if len(fields) != 2 {
		return "", false
	}
	if fields[0] != "/start" && !strings.EqualFold(fields[0], "/start@"+b.Username) {
		return "", false
	}
	for _, e := range m.Entities {
		if e.Type == "bot_command" && e.Offset == 0 {
			actual, ok := entityText(m.Text, e)
			if ok && actual == fields[0] {
				return fields[1], true
			}
		}
	}
	return "", false
}

// Task selection is from an explicit command or the administrator's default.
// A sole allowed task is unambiguous; multiple tasks never choose by array order.
func requestSkill(m message, b BotConfig, c Channel) (string, bool) {
	selected := ""
	for _, e := range m.Entities {
		if e.Type != "bot_command" || e.Offset != 0 {
			continue
		}
		text, ok := entityText(m.Text, e)
		if !ok {
			continue
		}
		command, target, has := strings.Cut(text, "@")
		if has && !strings.EqualFold(target, b.Username) {
			continue
		}
		if command == "/brief" {
			selected = "project-briefing"
		}
		if command == "/incident" {
			selected = "incident-summary"
		}
	}
	if selected == "" && c.DefaultSkillID != nil {
		selected = *c.DefaultSkillID
	}
	if selected == "" && len(c.AllowedSkillIDs) == 1 {
		selected = c.AllowedSkillIDs[0]
	}
	return selected, slices.Contains(c.AllowedSkillIDs, selected)
}
