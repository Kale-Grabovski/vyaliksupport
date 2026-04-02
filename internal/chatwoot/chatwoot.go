package chatwoot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type Woot struct {
	URL      string // API URL (e.g. https://chat.example.com)
	APIToken string
}

func NewWoot(apiURL, apiToken string) *Woot {
	return &Woot{
		URL:      apiURL,
		APIToken: apiToken,
	}
}

// ConversationInfo contains data extracted from a Chatwoot conversation.
type ConversationInfo struct {
	SenderID   int64  // Chatwoot contact ID
	Identifier string // External identifier (e.g. telegram user ID as string)
	Phone      string // Phone number if available
	Name       string // Contact name
}

// GetConversationInfo fetches conversation metadata from Chatwoot API.
func (w *Woot) GetConversationInfo(accountID, convID int) (*ConversationInfo, error) {
	url := fmt.Sprintf("%s/api/v1/accounts/%d/conversations/%d", w.URL, accountID, convID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("api_access_token", w.APIToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result struct {
		ID   int `json:"id"`
		Meta struct {
			Sender struct {
				ID         int64  `json:"id"`
				Name       string `json:"name"`
				Email      string `json:"email"`
				Phone      string `json:"phone_number"`
				Identifier string `json:"identifier"`
			} `json:"sender"`
		} `json:"meta"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &ConversationInfo{
		SenderID:   result.Meta.Sender.ID,
		Identifier: result.Meta.Sender.Identifier,
		Phone:      result.Meta.Sender.Phone,
		Name:       result.Meta.Sender.Name,
	}, nil
}

// ExtractTelegramID tries to extract a Telegram user ID from the conversation info.
func (c *ConversationInfo) ExtractTelegramID() (int64, bool) {
	if c.Identifier != "" {
		if id, err := strconv.ParseInt(c.Identifier, 10, 64); err == nil && id > 0 {
			return id, true
		}
		if strings.HasPrefix(c.Identifier, "tg:") {
			if id, err := strconv.ParseInt(strings.TrimPrefix(c.Identifier, "tg:"), 10, 64); err == nil {
				return id, true
			}
		}
	}

	return 0, false
}

// SendMetadataNote pushes a silent note to the support dashboard.
func (w *Woot) SendMetadataNote(accountID, convID int, metadata string) error {
	payload := map[string]interface{}{
		"content":      metadata,
		"message_type": "outgoing",
		"private":      true,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/accounts/%d/conversations/%d/messages", w.URL, accountID, convID)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api_access_token", w.APIToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API rejected the request, status: %d", resp.StatusCode)
	}

	return nil
}
