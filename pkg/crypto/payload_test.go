package crypto

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestParseNodePayload(t *testing.T) {
	// Create a test payload
	testPayload := &NodePayload{
		CACertPem:    "-----BEGIN CERTIFICATE-----\ntest-ca-cert\n-----END CERTIFICATE-----",
		NodeCertPem:  "-----BEGIN CERTIFICATE-----\ntest-node-cert\n-----END CERTIFICATE-----",
		NodeKeyPem:   "-----BEGIN RSA PRIVATE KEY-----\ntest-node-key\n-----END RSA PRIVATE KEY-----",
		JWTPublicKey: "-----BEGIN PUBLIC KEY-----\ntest-jwt-key\n-----END PUBLIC KEY-----",
	}

	// Encode to JSON then base64
	jsonBytes, err := json.Marshal(testPayload)
	if err != nil {
		t.Fatalf("Failed to marshal test payload: %v", err)
	}
	encoded := base64.StdEncoding.EncodeToString(jsonBytes)

	// Test parsing
	parsed, err := ParseNodePayload(encoded)
	if err != nil {
		t.Fatalf("ParseNodePayload failed: %v", err)
	}

	// Verify fields
	if parsed.CACertPem != testPayload.CACertPem {
		t.Errorf("CACertPem mismatch: got %q, want %q", parsed.CACertPem, testPayload.CACertPem)
	}
	if parsed.NodeCertPem != testPayload.NodeCertPem {
		t.Errorf("NodeCertPem mismatch: got %q, want %q", parsed.NodeCertPem, testPayload.NodeCertPem)
	}
	if parsed.NodeKeyPem != testPayload.NodeKeyPem {
		t.Errorf("NodeKeyPem mismatch: got %q, want %q", parsed.NodeKeyPem, testPayload.NodeKeyPem)
	}
	if parsed.JWTPublicKey != testPayload.JWTPublicKey {
		t.Errorf("JWTPublicKey mismatch: got %q, want %q", parsed.JWTPublicKey, testPayload.JWTPublicKey)
	}
}

func TestParseNodePayload_InvalidBase64(t *testing.T) {
	_, err := ParseNodePayload("not-valid-base64!!!")
	if err == nil {
		t.Error("Expected error for invalid base64, got nil")
	}
}

func TestParseNodePayload_InvalidJSON(t *testing.T) {
	// Valid base64 but invalid JSON
	encoded := base64.StdEncoding.EncodeToString([]byte("not json"))
	_, err := ParseNodePayload(encoded)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestParseNodePayload_MissingFields(t *testing.T) {
	tests := []struct {
		name    string
		payload *NodePayload
	}{
		{
			name: "missing CACertPem",
			payload: &NodePayload{
				NodeCertPem:  "cert",
				NodeKeyPem:   "key",
				JWTPublicKey: "jwt",
			},
		},
		{
			name: "missing NodeCertPem",
			payload: &NodePayload{
				CACertPem:    "ca",
				NodeKeyPem:   "key",
				JWTPublicKey: "jwt",
			},
		},
		{
			name: "missing NodeKeyPem",
			payload: &NodePayload{
				CACertPem:    "ca",
				NodeCertPem:  "cert",
				JWTPublicKey: "jwt",
			},
		},
		{
			name: "missing JWTPublicKey",
			payload: &NodePayload{
				CACertPem:   "ca",
				NodeCertPem: "cert",
				NodeKeyPem:  "key",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBytes, _ := json.Marshal(tt.payload)
			encoded := base64.StdEncoding.EncodeToString(jsonBytes)
			_, err := ParseNodePayload(encoded)
			if err == nil {
				t.Error("Expected error for missing field, got nil")
			}
		})
	}
}
