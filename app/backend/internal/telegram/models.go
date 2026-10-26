package telegram

import "time"

type Bot struct {
	ID       string `json:"bot_id"`
	Username string `json:"username"`
}
type Challenge struct {
	ID             string     `json:"id"`
	Version        int64      `json:"version"`
	BotID          string     `json:"bot_id"`
	TelegramUserID *string    `json:"telegram_user_id"`
	ExpiresAt      time.Time  `json:"expires_at"`
	ClaimedAt      *time.Time `json:"claimed_at"`
	ConfirmedAt    *time.Time `json:"confirmed_at"`
	CancelledAt    *time.Time `json:"cancelled_at"`
	CreatedAt      time.Time  `json:"created_at"`
	StartToken     string     `json:"start_token,omitempty"`
	StartURL       string     `json:"start_url,omitempty"`
}
type Binding struct {
	ID             string     `json:"id"`
	Version        int64      `json:"version"`
	BotID          string     `json:"bot_id"`
	TelegramUserID string     `json:"telegram_user_id"`
	State          string     `json:"state"`
	CreatedAt      time.Time  `json:"created_at"`
	RevokedAt      *time.Time `json:"revoked_at"`
}
type Status struct {
	IngressConfigured  bool        `json:"ingress_configured"`
	ExecutionAvailable bool        `json:"execution_available"`
	DeliveryAvailable  bool        `json:"delivery_available"`
	Bots               []Bot       `json:"bots"`
	Bindings           []Binding   `json:"bindings"`
	Challenges         []Challenge `json:"challenges"`
}
type ChallengeCreate struct {
	BotID  string `json:"bot_id"`
	Reason string `json:"reason"`
}
type ChallengeConfirm struct {
	ExpectedVersion int64  `json:"expected_version"`
	TelegramUserID  string `json:"telegram_user_id"`
	Reason          string `json:"reason"`
}
type ReasonInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	Reason          string `json:"reason"`
}
type Channel struct {
	ID              string    `json:"id"`
	Version         int64     `json:"version"`
	BotID           string    `json:"bot_id"`
	ChatID          string    `json:"chat_id"`
	TopicID         int64     `json:"topic_id"`
	Label           string    `json:"label"`
	ProjectID       *string   `json:"project_id"`
	AllowedSkillIDs []string  `json:"allowed_skill_ids"`
	DefaultSkillID  *string   `json:"default_skill_id"`
	DisclosureMode  string    `json:"disclosure_mode"`
	DeliveryEnabled bool      `json:"delivery_enabled"`
	State           string    `json:"state"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
type ChannelCreate struct {
	DeliveryEnabled bool     `json:"delivery_enabled"`
	BotID           string   `json:"bot_id"`
	ChatID          string   `json:"chat_id"`
	TopicID         int64    `json:"topic_id"`
	Label           string   `json:"label"`
	ProjectID       *string  `json:"project_id"`
	AllowedSkillIDs []string `json:"allowed_skill_ids"`
	DefaultSkillID  *string  `json:"default_skill_id"`
	Reason          string   `json:"reason"`
}
type ChannelPatch struct {
	DeliveryEnabled bool     `json:"delivery_enabled"`
	ExpectedVersion int64    `json:"expected_version"`
	Label           string   `json:"label"`
	ProjectID       *string  `json:"project_id"`
	AllowedSkillIDs []string `json:"allowed_skill_ids"`
	DefaultSkillID  *string  `json:"default_skill_id"`
	State           string   `json:"state"`
	Reason          string   `json:"reason"`
}
type Request struct {
	ID               string    `json:"id"`
	BotID            string    `json:"bot_id"`
	ChannelID        string    `json:"channel_id"`
	ChannelVersion   int64     `json:"channel_version"`
	State            string    `json:"state"`
	ReasonCode       string    `json:"reason_code"`
	Text             string    `json:"text"`
	ReceivedAt       time.Time `json:"received_at"`
	RequestedSkillID *string   `json:"requested_skill_id"`
	ConversationID   *string   `json:"conversation_id"`
	RunID            *string   `json:"run_id"`
	RunState         *string   `json:"run_state"`
}
type Revision struct {
	ID        string    `json:"id"`
	ChannelID string    `json:"channel_id"`
	Version   int64     `json:"version"`
	Source    string    `json:"source"`
	Reason    string    `json:"reason"`
	Snapshot  Channel   `json:"snapshot"`
	CreatedAt time.Time `json:"created_at"`
}
