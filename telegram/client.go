package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mallard/multilegaware/tfl"
)

const apiBase = "https://api.telegram.org"

// Client sends messages via the Telegram Bot API.
type Client struct {
	token      string
	httpClient *http.Client
}

func NewClient(token string) *Client {
	return &Client{
		token: token,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type sendMessageRequest struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

type sendMessageResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description,omitempty"`
}

// SendMessage posts a single MarkdownV2 message to the given chat.
func (c *Client) SendMessage(ctx context.Context, chatID, text string) error {
	payload := sendMessageRequest{
		ChatID:    chatID,
		Text:      text,
		ParseMode: "MarkdownV2",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", apiBase, c.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending Telegram message: %w", err)
	}
	defer resp.Body.Close()

	var result sendMessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decoding Telegram response: %w", err)
	}
	if !result.OK {
		return fmt.Errorf("Telegram API error: %s", result.Description)
	}
	return nil
}

// modeEmoji maps TfL mode names to display emojis.
func modeEmoji(mode string) string {
	switch mode {
	case "walking":
		return "🚶"
	case "tube":
		return "🚇"
	case "bus":
		return "🚌"
	case "national-rail":
		return "🚆"
	case "dlr":
		return "🚈"
	case "overground":
		return "🚆"
	case "elizabeth-line":
		return "🚇"
	case "tram":
		return "🚊"
	case "cable-car":
		return "🚡"
	case "coach":
		return "🚌"
	default:
		return "🚍"
	}
}

// escapeMarkdownV2 escapes characters that have special meaning in Telegram MarkdownV2.
func escapeMarkdownV2(s string) string {
	// Characters that must be escaped: _ * [ ] ( ) ~ ` > # + - = | { } . !
	replacer := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"~", "\\~",
		"`", "\\`",
		">", "\\>",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		"=", "\\=",
		"|", "\\|",
		"{", "\\{",
		"}", "\\}",
		".", "\\.",
		"!", "\\!",
	)
	return replacer.Replace(s)
}

// FormatJourney renders a single journey as a MarkdownV2 string.
func FormatJourney(index int, j tfl.Journey) string {
	var legs []string
	for _, leg := range j.Legs {
		emoji := modeEmoji(leg.Mode.Name)
		summary := escapeMarkdownV2(leg.Instruction.Summary)
		legs = append(legs, fmt.Sprintf("%s %s %d min", emoji, summary, leg.Duration))
	}

	header := fmt.Sprintf("*Journey %d* — %d mins", index, j.Duration)
	body := strings.Join(legs, " → ")
	return header + "\n" + body
}

// SendNoJourneysMessage sends the "no journeys found" error message.
func (c *Client) SendNoJourneysMessage(ctx context.Context, chatID, from, to string) error {
	text := fmt.Sprintf("❌ No journeys found between %s and %s\\.",
		escapeMarkdownV2(from), escapeMarkdownV2(to))
	return c.SendMessage(ctx, chatID, text)
}
