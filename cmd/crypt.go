package cmd

import (
	"encoding/base64"
	"fmt"
	"os"

	"vyaliksupport/internal/crypto"
	"vyaliksupport/internal/domain"

	"github.com/spf13/cobra"
)

var cryptCmd = &cobra.Command{
	Use:   "crypt",
	Short: "Encrypt payload for ntfy testing",
	RunE:  runCrypt,
}

var (
	payloadFile string
)

func runCrypt(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	if cfg.Ntfy.EncryptKey == "" {
		return fmt.Errorf("encrypt_key not configured")
	}

	// Read payload from file
	if payloadFile == "" {
		payloadFile = "payload.txt"
	}

	data, err := os.ReadFile(payloadFile)
	if err != nil {
		return fmt.Errorf("failed to read payload file: %w", err)
	}

	text := string(data)
	if text == "" {
		return fmt.Errorf("payload file is empty")
	}

	// Create payload
	payload := &domain.Payload{
		Content: domain.Content{
			Type: domain.ContentTypeText,
			Text: text,
		},
	}

	// Marshal to JSON
	payloadBytes, err := payload.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Encrypt
	ciphertext, err := crypto.Encrypt(payloadBytes, cfg.Ntfy.EncryptKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt: %w", err)
	}

	// Encode to base64
	encoded := base64.StdEncoding.EncodeToString(ciphertext)

	fmt.Println(encoded)
	return nil
}

func init() {
	rootCmd.AddCommand(cryptCmd)
	cryptCmd.Flags().StringVar(&payloadFile, "file", "payload.txt", "payload text file")
}
