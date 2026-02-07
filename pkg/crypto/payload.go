package crypto

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

// NodePayload contains the decoded SECRET_KEY payload
type NodePayload struct {
	CACertPem    string `json:"caCertPem"`
	NodeCertPem  string `json:"nodeCertPem"`
	NodeKeyPem   string `json:"nodeKeyPem"`
	JWTPublicKey string `json:"jwtPublicKey"`
}

// ParseNodePayload decodes and parses the SECRET_KEY
func ParseNodePayload(secretKey string) (*NodePayload, error) {
	if secretKey == "" {
		return nil, errors.New("SECRET_KEY is not set")
	}

	// Decode base64
	decoded, err := base64.StdEncoding.DecodeString(secretKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode SECRET_KEY: %w", err)
	}

	// Parse JSON
	var payload NodePayload
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return nil, fmt.Errorf("SECRET_KEY contains invalid JSON: %w", err)
	}

	// Validate required fields
	if err := payload.Validate(); err != nil {
		return nil, fmt.Errorf("invalid SECRET_KEY payload: %w", err)
	}

	return &payload, nil
}

// Validate checks if all required fields are present
func (p *NodePayload) Validate() error {
	if p.CACertPem == "" {
		return errors.New("caCertPem is required")
	}
	if p.NodeCertPem == "" {
		return errors.New("nodeCertPem is required")
	}
	if p.NodeKeyPem == "" {
		return errors.New("nodeKeyPem is required")
	}
	if p.JWTPublicKey == "" {
		return errors.New("jwtPublicKey is required")
	}
	return nil
}
