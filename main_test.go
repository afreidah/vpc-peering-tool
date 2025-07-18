package main

// -------------------------------------------------------------------------------------------------
// Unit tests for GetAccountIDFromRoleArn
// -------------------------------------------------------------------------------------------------

import (
	"os"
	"testing"

	"github.com/aws/jsii-runtime-go"
	"github.com/hashicorp/terraform-cdk-go/cdktf"
)

// -------------------------------------------------------------------------------------------------
// Unit test: GetAccountIDFromRoleArn (table-driven)
// -------------------------------------------------------------------------------------------------
/*
TestGetAccountIDFromRoleArn verifies extraction of the AWS account ID from various IAM role ARN strings,
including valid, invalid, empty, and malformed cases.
*/

func TestGetAccountIDFromRoleArn(t *testing.T) {
	tests := []struct {
		name     string
		arn      string
		expected string
	}{
		{
			name:     "Valid ARN",
			arn:      "arn:aws:iam::123456789012:role/MyRole",
			expected: "123456789012",
		},
		{
			name:     "Invalid ARN - too short",
			arn:      "arn:aws:iam::role/MyRole",
			expected: "",
		},
		{
			name:     "Empty string",
			arn:      "",
			expected: "",
		},
		{
			name:     "Malformed ARN",
			arn:      "arn:aws:iam:123456789012",
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := GetAccountIDFromRoleArn(tc.arn)
			if got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

// -------------------------------------------------------------------------------------------------
// Unit test: LoadConfig (positive case)
// -------------------------------------------------------------------------------------------------
/*
TestLoadConfig_Positive verifies that LoadConfig correctly loads and parses a valid YAML config file.

Creates a temporary YAML file, writes a minimal valid config, loads it, and checks the result.
*/
func TestLoadConfig_Positive(t *testing.T) {
	yamlContent := `
peers:
  test-peer:
    vpc_id: vpc-123
    region: us-west-2
    role_arn: arn:aws:iam::123456789012:role/TestRole
    dns_resolution: true
    has_additional_routes: false
peering_matrix:
  test-peer: []
`
	tmpfile, err := os.CreateTemp("", "peering-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(yamlContent)); err != nil {
		t.Fatalf("failed to write yaml: %v", err)
	}
	tmpfile.Close()

	cfg := LoadConfig(tmpfile.Name())
	if len(cfg.Peers) != 1 {
		t.Errorf("expected 1 peer, got %d", len(cfg.Peers))
	}
	if _, ok := cfg.Peers["test-peer"]; !ok {
		t.Errorf("expected peer 'test-peer' in config")
	}
}

// -------------------------------------------------------------------------------------------------
// Unit test: ConvertToPeerConfigs (positive case)
// -------------------------------------------------------------------------------------------------
/*
TestConvertToPeerConfigs_Positive verifies that ConvertToPeerConfigs correctly converts a valid YAMLConfig
to a slice of PeerConfig structs, including correct field mapping and source filtering.

This test constructs a minimal YAMLConfig with two peers and a peering matrix, then checks that the
resulting PeerConfig slice has the expected length and values.
*/

func TestConvertToPeerConfigs_Positive(t *testing.T) {
	cfg := YAMLConfig{
		Peers: map[string]YAMLPeer{
			"source-peer": {
				VpcId:               "vpc-111",
				Region:              "us-west-2",
				RoleArn:             "arn:aws:iam::111111111111:role/SourceRole",
				DnsResolution:       true,
				HasAdditionalRoutes: false,
			},
			"target-peer": {
				VpcId:               "vpc-222",
				Region:              "us-east-1",
				RoleArn:             "arn:aws:iam::222222222222:role/TargetRole",
				DnsResolution:       false,
				HasAdditionalRoutes: true,
			},
		},
		PeeringMatrix: map[string][]string{
			"source-peer": {"target-peer"},
		},
	}

	peerConfigs := ConvertToPeerConfigs(cfg, "source-peer")
	if len(peerConfigs) != 1 {
		t.Fatalf("expected 1 peer config, got %d", len(peerConfigs))
	}
	pc := peerConfigs[0]
	if pc.SourceVpcId != "vpc-111" || pc.PeerVpcId != "vpc-222" {
		t.Errorf("unexpected VPC IDs: got %q and %q", pc.SourceVpcId, pc.PeerVpcId)
	}
	if pc.EnableDnsResolution != false || pc.HasExtraPeerRouteTables != true {
		t.Errorf("unexpected DNS or route table flags: got %v and %v", pc.EnableDnsResolution, pc.HasExtraPeerRouteTables)
	}
}

// -------------------------------------------------------------------------------------------------
// Unit test: CreateAwsProvider (positive case)
// -------------------------------------------------------------------------------------------------
/*
TestCreateAwsProvider_Positive verifies that CreateAwsProvider creates an AWS provider resource
with the expected configuration fields set.

This test creates a minimal CDKTF stack, invokes CreateAwsProvider, and checks the provider's
region, alias, and assume role ARN attributes.
*/

func TestCreateAwsProvider_Positive(t *testing.T) {
	app := cdktf.NewApp(nil)
	stack := cdktf.NewTerraformStack(app, jsii.String("test-stack"))

	provider := CreateAwsProvider(stack, "TestProvider", "test-alias", "us-west-2", "arn:aws:iam::123456789012:role/TestRole")

	if provider.Region() == nil || *provider.Region() != "us-west-2" {
		t.Errorf("expected region 'us-west-2', got %v", provider.Region())
	}
	if provider.Alias() == nil || *provider.Alias() != "test-alias" {
		t.Errorf("expected alias 'test-alias', got %v", provider.Alias())
	}
	if provider.AssumeRole() == nil || len(*provider.AssumeRole()) == 0 || (*provider.AssumeRole())[0].RoleArn == nil || *(*provider.AssumeRole())[0].RoleArn != "arn:aws:iam::123456789012:role/TestRole" {
		t.Errorf("expected assume role ARN 'arn:aws:iam::123456789012:role/TestRole', got %v", provider.AssumeRole())
	}
}
