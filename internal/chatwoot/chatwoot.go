package chatwoot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/zap"
)

type Woot struct {
	URL      string // API URL (e.g. https://chat.example.com)
	APIToken string
	lg       *zap.Logger
}

func NewWoot(apiURL, apiToken string, lg *zap.Logger) *Woot {
	return &Woot{
		URL:      apiURL,
		APIToken: apiToken,
		lg:       lg,
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

// FindOrCreateConversation looks up a contact by identifier and returns an existing
// conversation or creates a new one in the specified inbox.
func (w *Woot) FindOrCreateConversation(accountID, inboxID int, identifier string) (int, error) {
	// 1. Try to find an existing contact by identifier.
	contactID, err := w.findContactByIdentifier(accountID, identifier)
	if err != nil {
		// Contact not found — create a new one.
		contactID, err = w.createContact(accountID, identifier)
		if err != nil {
			return 0, fmt.Errorf("failed to create contact: %w", err)
		}
	}

	// 2. Look for an existing conversation with this contact in the inbox.
	convID, err := w.findConversationByContact(accountID, contactID, inboxID)
	if err == nil {
		return convID, nil // existing conversation found
	}

	// 3. No suitable conversation — create a new one.
	return w.createConversation(accountID, inboxID, contactID)
}

// findContactByIdentifier searches for a contact with the given external identifier.
func (w *Woot) findContactByIdentifier(accountID int, identifier string) (int, error) {
	url := fmt.Sprintf("%s/api/v1/accounts/%d/contacts?identifier=%s", w.URL, accountID, identifier)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("api_access_token", w.APIToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusUnprocessableEntity {
		return 0, fmt.Errorf("contact not found")
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result struct {
		Payload []struct {
			ID         int    `json:"id"`
			Identifier string `json:"identifier"`
		} `json:"payload"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}

	if len(result.Payload) == 0 {
		return 0, fmt.Errorf("contact not found")
	}

	return result.Payload[0].ID, nil
}

// createContact registers a new contact in Chatwoot.
func (w *Woot) createContact(accountID int, identifier string) (int, error) {
	url := fmt.Sprintf("%s/api/v1/accounts/%d/contacts", w.URL, accountID)

	payload := map[string]interface{}{
		"identifier": identifier,
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api_access_token", w.APIToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}

	return result.ID, nil
}

// findConversationByContact returns an existing conversation for the contact in the inbox.
func (w *Woot) findConversationByContact(accountID, contactID, inboxID int) (int, error) {
	url := fmt.Sprintf("%s/api/v1/accounts/%d/contacts/%d/conversations", w.URL, accountID, contactID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("api_access_token", w.APIToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result struct {
		Payload []struct {
			ID      int `json:"id"`
			InboxID int `json:"inbox_id"`
		} `json:"payload"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}

	// Return first conversation that belongs to the target inbox.
	for _, conv := range result.Payload {
		if conv.InboxID == inboxID {
			return conv.ID, nil
		}
	}

	return 0, fmt.Errorf("no conversation found in inbox %d", inboxID)
}

// createConversation opens a new conversation in the specified inbox for the given contact.
func (w *Woot) createConversation(accountID, inboxID, contactID int) (int, error) {
	url := fmt.Sprintf("%s/api/v1/accounts/%d/conversations", w.URL, accountID)

	payload := map[string]interface{}{
		"inbox_id":   inboxID,
		"contact_id": contactID,
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api_access_token", w.APIToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result struct {
		ID int `json:"contact_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}

	return result.ID, nil
}

// SendMessage posts a public message into an existing Chatwoot conversation.
func (w *Woot) SendMessage(accountID, convID int, content string) error {
	url := fmt.Sprintf("%s/api/v1/accounts/%d/conversations/%d/messages", w.URL, accountID, convID)

	payload := map[string]interface{}{
		"content":      content,
		"message_type": "incoming",
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

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

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	return nil
}

// AttachmentInfo contains info for sending attachments to Chatwoot.
type AttachmentInfo struct {
	URL      string
	FileName string
	MimeType string
}

// SendMessageWithAttachments sends a message with file attachments to Chatwoot.
func (w *Woot) SendMessageWithAttachments(accountID, convID int, content string, attachments []AttachmentInfo) error {
	if len(attachments) == 0 {
		return w.SendMessage(accountID, convID, content)
	}

	url := fmt.Sprintf("%s/api/v1/accounts/%d/conversations/%d/messages", w.URL, accountID, convID)

	payload := map[string]interface{}{
		"content":      content,
		"message_type": "incoming",
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

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

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	// Now send attachments one by one
	for _, att := range attachments {
		if err := w.sendAttachment(accountID, convID, att); err != nil {
			w.lg.Error("failed to send attachment", zap.String("url", att.URL), zap.Error(err))
			// Continue with other attachments
		}
	}

	return nil
}

// sendAttachment sends a single file attachment to Chatwoot.
func (w *Woot) sendAttachment(accountID, convID int, att AttachmentInfo) error {
	url := fmt.Sprintf("%s/api/v1/accounts/%d/conversations/%d/messages", w.URL, accountID, convID)

	// Download the file first
	resp, err := http.Get(att.URL)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download file: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read file data: %w", err)
	}

	// Create multipart form request
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add content field
	writer.WriteField("content", "")
	writer.WriteField("message_type", "incoming")

	// Add file
	part, err := writer.CreateFormFile("attachments[]", att.FileName)
	if err != nil {
		return fmt.Errorf("failed to create form file: %w", err)
	}
	part.Write(data)

	writer.Close()

	req, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("api_access_token", w.APIToken)

	client := &http.Client{}
	httpResp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusCreated {
		return fmt.Errorf("API returned status %d", httpResp.StatusCode)
	}

	return nil
}
