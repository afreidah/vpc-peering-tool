// -------------------------------------------------------------------------------------------------
// CDKTF VPC Peering Stack with YAML Input and Source Filtering
// -------------------------------------------------------------------------------------------------
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
	"github.com/hashicorp/terraform-cdk-go/cdktf"
	"gopkg.in/yaml.v2"

	dataawsroutetable "cdk.tf/go/stack/generated/hashicorp/aws/dataawsroutetable"
	dataawsvpc "cdk.tf/go/stack/generated/hashicorp/aws/dataawsvpc"
	awsprovider "cdk.tf/go/stack/generated/hashicorp/aws/provider"
	vpcpeeringconnection "cdk.tf/go/stack/generated/hashicorp/aws/vpcpeeringconnection"
)

// PeerConfig defines the configuration for a single VPC peering connection.
type PeerConfig struct {
	SourceVpcId   string
	SourceRegion  string
	SourceRoleArn string
	PeerVpcId     string
	PeerRegion    string
	PeerRoleArn   string
	Name          string
}

// YAMLPeer represents a peer entry in the YAML file.
type YAMLPeer struct {
	VpcId   string `yaml:"vpc_id"`
	Region  string `yaml:"region"`
	RoleArn string `yaml:"role_arn"`
}

// YAMLConfig holds the structure of the YAML configuration file.
type YAMLConfig struct {
	Peers         map[string]YAMLPeer `yaml:"peers"`
	PeeringMatrix map[string][]string `yaml:"peering_matrix"`
}

func NewMyStack(scope constructs.Construct, id string, sourceID string, peers []PeerConfig) cdktf.TerraformStack {
	stack := cdktf.NewTerraformStack(scope, &id)

	// Define the 'source_id' variable (renamed from 'source')
	cdktf.NewTerraformVariable(stack, jsii.String("source_id"), &cdktf.TerraformVariableConfig{
		Type:        jsii.String("string"),
		Description: jsii.String("The source identifier for this resource"),
		Default:     jsii.String("default-source"), // Optional default value
	})

	var vpcPeeringConnections []vpcpeeringconnection.VpcPeeringConnection
	var sourceMainRouteTables []dataawsroutetable.DataAwsRouteTable
	var peerMainRouteTables []dataawsroutetable.DataAwsRouteTable

	for i, peer := range peers {
		sourceRegion := peer.SourceRegion
		if sourceRegion == "" {
			sourceRegion = "us-west-2"
		}
		peerRegion := peer.PeerRegion
		if peerRegion == "" {
			peerRegion = "us-west-2"
		}

		sourceProvider := awsprovider.NewAwsProvider(stack, jsii.String(fmt.Sprintf("SourceAWS%d", i)), &awsprovider.AwsProviderConfig{
			Region: jsii.String(sourceRegion),
			Alias:  jsii.String(fmt.Sprintf("source%d", i)),
			AssumeRole: &[]*awsprovider.AwsProviderAssumeRole{{
				RoleArn: jsii.String(peer.SourceRoleArn),
			}},
		})

		peerProvider := awsprovider.NewAwsProvider(stack, jsii.String(fmt.Sprintf("PeerAWS%d", i)), &awsprovider.AwsProviderConfig{
			Region: jsii.String(peerRegion),
			Alias:  jsii.String(fmt.Sprintf("peer%d", i)),
			AssumeRole: &[]*awsprovider.AwsProviderAssumeRole{{
				RoleArn: jsii.String(peer.PeerRoleArn),
			}},
		})

		_ = dataawsvpc.NewDataAwsVpc(stack, jsii.String(fmt.Sprintf("PeerVpcData%d", i)), &dataawsvpc.DataAwsVpcConfig{
			Id:       jsii.String(peer.PeerVpcId),
			Provider: peerProvider,
		})
		_ = dataawsvpc.NewDataAwsVpc(stack, jsii.String(fmt.Sprintf("SourceVpcData%d", i)), &dataawsvpc.DataAwsVpcConfig{
			Id:       jsii.String(peer.SourceVpcId),
			Provider: sourceProvider,
		})

		peerMainRt := dataawsroutetable.NewDataAwsRouteTable(stack, jsii.String(fmt.Sprintf("PeerMainRouteTable%d", i)), &dataawsroutetable.DataAwsRouteTableConfig{
			VpcId:    jsii.String(peer.PeerVpcId),
			Provider: peerProvider,
			Filter: &[]*dataawsroutetable.DataAwsRouteTableFilter{{
				Name:   jsii.String("association.main"),
				Values: jsii.Strings("true"),
			}},
		})
		peerMainRouteTables = append(peerMainRouteTables, peerMainRt)

		sourceMainRt := dataawsroutetable.NewDataAwsRouteTable(stack, jsii.String(fmt.Sprintf("SourceMainRouteTable%d", i)), &dataawsroutetable.DataAwsRouteTableConfig{
			VpcId:    jsii.String(peer.SourceVpcId),
			Provider: sourceProvider,
			Filter: &[]*dataawsroutetable.DataAwsRouteTableFilter{{
				Name:   jsii.String("association.main"),
				Values: jsii.Strings("true"),
			}},
		})
		sourceMainRouteTables = append(sourceMainRouteTables, sourceMainRt)

		name := peer.Name
		if name == "" {
			name = peer.PeerVpcId
		}

		peering := vpcpeeringconnection.NewVpcPeeringConnection(stack, jsii.String(fmt.Sprintf("VpcPeering%d", i)), &vpcpeeringconnection.VpcPeeringConnectionConfig{
			VpcId:       jsii.String(peer.SourceVpcId),
			PeerVpcId:   jsii.String(peer.PeerVpcId),
			PeerRegion:  jsii.String(peerRegion),
			PeerOwnerId: jsii.String("302210007521"),
			Provider:    peerProvider,
			AutoAccept:  jsii.Bool(true),
			Tags: &map[string]*string{
				"Name":        jsii.String(fmt.Sprintf("Connection to %s", name)),
				"Environment": jsii.String("production"),
				"ManagedBy":   jsii.String("cdktf"),
				"SourceVpcId": jsii.String(peer.SourceVpcId),
				"PeerVpcId":   jsii.String(peer.PeerVpcId),
			},
		})
		vpcPeeringConnections = append(vpcPeeringConnections, peering)
	}

	addOutputs(stack, peers, vpcPeeringConnections, sourceMainRouteTables, peerMainRouteTables)
	return stack
}

func addOutputs(stack cdktf.TerraformStack, peers []PeerConfig, vpcs []vpcpeeringconnection.VpcPeeringConnection, sourceTables []dataawsroutetable.DataAwsRouteTable, peerTables []dataawsroutetable.DataAwsRouteTable) {
	for i := range peers {
		cdktf.NewTerraformOutput(stack, jsii.String(fmt.Sprintf("VpcPeeringConnectionId_%d", i)), &cdktf.TerraformOutputConfig{
			Value: vpcs[i].Id(),
		})
		cdktf.NewTerraformOutput(stack, jsii.String(fmt.Sprintf("SourceMainRouteTableId_%d", i)), &cdktf.TerraformOutputConfig{
			Value: sourceTables[i].Id(),
		})
		cdktf.NewTerraformOutput(stack, jsii.String(fmt.Sprintf("PeerMainRouteTableId_%d", i)), &cdktf.TerraformOutputConfig{
			Value: peerTables[i].Id(),
		})
	}
}

func loadConfig(path string) YAMLConfig {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("failed to read config file: %v", err)
	}
	var cfg YAMLConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("failed to parse yaml: %v", err)
	}
	return cfg
}

func convertToPeerConfigs(cfg YAMLConfig, sourceFilter string) []PeerConfig {
	var peerConfigs []PeerConfig
	log.Printf("[convert] Applying source filter: %q", sourceFilter)

	for source, targets := range cfg.PeeringMatrix {

		if sourceFilter != "" && source != sourceFilter {
			continue
		}
		log.Printf("[convert] Considering source: %q", source)

		sourcePeer, ok := cfg.Peers[source]
		if !ok {
			log.Fatalf("missing source peer config for %q", source)
		}

		for _, target := range targets {
			peerPeer, ok := cfg.Peers[target]
			if !ok {
				log.Fatalf("missing peer config for %q", target)
			}

			peerConfigs = append(peerConfigs, PeerConfig{
				SourceVpcId:   sourcePeer.VpcId,
				SourceRegion:  sourcePeer.Region,
				SourceRoleArn: sourcePeer.RoleArn,
				PeerVpcId:     peerPeer.VpcId,
				PeerRegion:    peerPeer.Region,
				PeerRoleArn:   peerPeer.RoleArn,
				Name:          target,
			})
		}
	}
	log.Printf("[convert] Returning %d peer configs", len(peerConfigs))
	return peerConfigs
}

func main() {
	cfg := loadConfig("peering.yaml")

	sourceID := os.Getenv("CDKTF_SOURCE")
	if sourceID == "" {
		sourceID = "default-source"
	}

	peers := convertToPeerConfigs(cfg, sourceID)

	if len(peers) == 0 {
		log.Fatalf("no peers matched for source: %s", sourceID)
	}

	app := cdktf.NewApp(nil)
	NewMyStack(app, "cdktf-vpc-peering-module", sourceID, peers)
	app.Synth()
}
