package main

import (
	"os"
	"testing"
)

// TestGetAccountIDFromRoleArn tests extraction of account ID from various ARNs.
func TestGetAccountIDFromRoleArn(t *testing.T) {
	tests := []struct {
		arn      string
		expected string
	}{
		{"arn:aws:iam::123456789012:role/MyRole", "123456789012"},
		{"arn:aws:iam::role/MyRole", ""},
		{"", ""},
		{"arn:aws:iam:123456789012", ""},
	}
	for _, tt := range tests {
		got := GetAccountIDFromRoleArn(tt.arn)
		if got != tt.expected {
			t.Errorf("GetAccountIDFromRoleArn(%q) = %q, want %q", tt.arn, got, tt.expected)
		}
	}
}

// TestLoadConfig tests loading a valid YAML config.
func TestLoadConfig(t *testing.T) {
	yaml := `
peers:
  foo:
    vpc_id: vpc-1
    region: us-west-2
    role_arn: arn:aws:iam::123:role/x
    dns_resolution: true
    has_additional_routes: false
peering_matrix:
  foo: []
`
	tmp, err := os.CreateTemp("", "peering-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write([]byte(yaml)); err != nil {
		t.Fatal(err)
	}
	tmp.Close()

	cfg := LoadConfig(tmp.Name())
	if len(cfg.Peers) != 1 {
		t.Errorf("expected 1 peer, got %d", len(cfg.Peers))
	}
	if _, ok := cfg.Peers["foo"]; !ok {
		t.Errorf("expected peer 'foo' in config")
	}
}

// TestConvertToPeerConfigs tests conversion from YAMLConfig to PeerConfig.
func TestConvertToPeerConfigs(t *testing.T) {
	cfg := YAMLConfig{
		Peers: map[string]YAMLPeer{
			"foo": {
				VpcID:               "vpc-1",
				Region:              "us-west-2",
				RoleArn:             "arn:aws:iam::123:role/x",
				DNSResolution:       true,
				HasAdditionalRoutes: false,
			},
			"bar": {
				VpcID:               "vpc-2",
				Region:              "us-east-1",
				RoleArn:             "arn:aws:iam::456:role/y",
				DNSResolution:       false,
				HasAdditionalRoutes: true,
			},
		},
		PeeringMatrix: map[string][]string{
			"foo": {"bar"},
		},
	}
	peers := ConvertToPeerConfigs(cfg, "")
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer config, got %d", len(peers))
	}
	pc := peers[0]
	if pc.SourceVpcID != "vpc-1" || pc.PeerVpcID != "vpc-2" {
		t.Errorf("unexpected VPC IDs: %q, %q", pc.SourceVpcID, pc.PeerVpcID)
	}
	if pc.EnableDNSResolution != false || pc.HasExtraPeerRouteTables != true {
		t.Errorf("unexpected DNS or route table flags: %v, %v", pc.EnableDNSResolution, pc.HasExtraPeerRouteTables)
	}
}
