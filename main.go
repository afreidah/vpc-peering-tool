// -------------------------------------------------------------------------------------------------
// CDKTF VPC Peering Stack with Bi-Directional Routing, DNS, and Automatic Subnet Route Management
// Handles cross-account/region peering with explicit accepter resource.
// -------------------------------------------------------------------------------------------------
package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
	"github.com/hashicorp/terraform-cdk-go/cdktf"
	"gopkg.in/yaml.v2"

	dataawsroutetable "cdk.tf/go/stack/generated/hashicorp/aws/dataawsroutetable"
	dataawssubnets "cdk.tf/go/stack/generated/hashicorp/aws/dataawssubnets"
	dataawsvpc "cdk.tf/go/stack/generated/hashicorp/aws/dataawsvpc"
	awsprovider "cdk.tf/go/stack/generated/hashicorp/aws/provider"
	awsroute "cdk.tf/go/stack/generated/hashicorp/aws/route"
	vpcpeeringconnection "cdk.tf/go/stack/generated/hashicorp/aws/vpcpeeringconnection"
)

// -------------------------------------------------------------------------------------------------
// Struct Definitions
// -------------------------------------------------------------------------------------------------

// PeerConfig defines the configuration for a single VPC peering connection, including DNS and extra route flag.
type PeerConfig struct {
	SourceVpcId             string
	SourceRegion            string
	SourceRoleArn           string
	PeerVpcId               string
	PeerRegion              string
	PeerRoleArn             string
	Name                    string
	EnableDnsResolution     bool
	HasExtraPeerRouteTables bool // Controls whether to add subnet routes
}

// YAMLPeer represents a peer entry in the YAML file.
type YAMLPeer struct {
	VpcId   string `yaml:"vpc_id"`
	Region  string `yaml:"region"`
	RoleArn string `yaml:"role_arn"`
}

// YAMLConfig holds the structure of the YAML configuration file, including DNS and extra route flag.
type YAMLConfig struct {
	Peers            map[string]YAMLPeer `yaml:"peers"`
	PeeringMatrix    map[string][]string `yaml:"peering_matrix"`
	DnsResolution    map[string]bool     `yaml:"dns_resolution,omitempty"`
	AdditionalRoutes map[string][]string `yaml:"additional_routes,omitempty"`
}

// -------------------------------------------------------------------------------------------------
// Helper: Extract account ID from role ARN
// -------------------------------------------------------------------------------------------------
func getAccountIdFromRoleArn(roleArn string) string {
	parts := strings.Split(roleArn, ":")
	if len(parts) >= 5 {
		return parts[4]
	}
	return ""
}

// -------------------------------------------------------------------------------------------------
// Stack Construction
// -------------------------------------------------------------------------------------------------

// NewMyStack creates the Terraform stack for VPC peering, DNS, and route management.
func NewMyStack(scope constructs.Construct, id string, sourceID string, peers []PeerConfig) cdktf.TerraformStack {
	stack := cdktf.NewTerraformStack(scope, &id)

	// Define the 'source_id' variable for filtering (not used in Terraform output, just for context)
	cdktf.NewTerraformVariable(stack, jsii.String("source_id"), &cdktf.TerraformVariableConfig{
		Type:        jsii.String("string"),
		Description: jsii.String("The source identifier for this resource"),
		Default:     jsii.String("default-source"),
	})

	var vpcPeeringConnections []vpcpeeringconnection.VpcPeeringConnection
	var sourceMainRouteTables []dataawsroutetable.DataAwsRouteTable
	var peerMainRouteTables []dataawsroutetable.DataAwsRouteTable

	// -------------------------------------------------------------------------
	// Iterate over each peering configuration and create resources
	// -------------------------------------------------------------------------
	for i, peer := range peers {
		// Set default regions if not provided
		sourceRegion := peer.SourceRegion
		if sourceRegion == "" {
			sourceRegion = "us-west-2"
		}
		peerRegion := peer.PeerRegion
		if peerRegion == "" {
			peerRegion = "us-west-2"
		}

		// AWS Providers for source and peer, with role assumption
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

		// ---------------------------------------------------------------------
		// Data sources for VPCs (used to get main CIDR block)
		// ---------------------------------------------------------------------
		sourceVpcData := dataawsvpc.NewDataAwsVpc(stack, jsii.String(fmt.Sprintf("SourceVpcData%d", i)), &dataawsvpc.DataAwsVpcConfig{
			Id:       jsii.String(peer.SourceVpcId),
			Provider: sourceProvider,
		})
		peerVpcData := dataawsvpc.NewDataAwsVpc(stack, jsii.String(fmt.Sprintf("PeerVpcData%d", i)), &dataawsvpc.DataAwsVpcConfig{
			Id:       jsii.String(peer.PeerVpcId),
			Provider: peerProvider,
		})

		// Main route tables for source and peer VPCs
		sourceMainRt := dataawsroutetable.NewDataAwsRouteTable(stack, jsii.String(fmt.Sprintf("SourceMainRouteTable%d", i)), &dataawsroutetable.DataAwsRouteTableConfig{
			VpcId:    jsii.String(peer.SourceVpcId),
			Provider: sourceProvider,
			Filter: &[]*dataawsroutetable.DataAwsRouteTableFilter{{
				Name:   jsii.String("association.main"),
				Values: jsii.Strings("true"),
			}},
		})
		sourceMainRouteTables = append(sourceMainRouteTables, sourceMainRt)

		peerMainRt := dataawsroutetable.NewDataAwsRouteTable(stack, jsii.String(fmt.Sprintf("PeerMainRouteTable%d", i)), &dataawsroutetable.DataAwsRouteTableConfig{
			VpcId:    jsii.String(peer.PeerVpcId),
			Provider: peerProvider,
			Filter: &[]*dataawsroutetable.DataAwsRouteTableFilter{{
				Name:   jsii.String("association.main"),
				Values: jsii.Strings("true"),
			}},
		})
		peerMainRouteTables = append(peerMainRouteTables, peerMainRt)

		// ---------------------------------------------------------------------
		// VPC Peering Connection with DNS resolution option
		// ---------------------------------------------------------------------
		name := peer.Name
		if name == "" {
			name = peer.PeerVpcId
		}

		// Determine if auto_accept can be true (only if regions are the same)
		autoAccept := sourceRegion == peerRegion

		// Set PeerOwnerId dynamically from peer's role ARN
		peerOwnerId := getAccountIdFromRoleArn(peer.PeerRoleArn)

		// Build the config struct for the peering connection
		peeringConfig := &vpcpeeringconnection.VpcPeeringConnectionConfig{
			VpcId:       jsii.String(peer.SourceVpcId),
			PeerVpcId:   jsii.String(peer.PeerVpcId),
			PeerOwnerId: jsii.String(peerOwnerId),
			Provider:    sourceProvider, // Always use the source/requester provider!
			AutoAccept:  jsii.Bool(autoAccept),
			Requester: &vpcpeeringconnection.VpcPeeringConnectionRequester{
				AllowRemoteVpcDnsResolution: jsii.Bool(peer.EnableDnsResolution),
			},
			Tags: &map[string]*string{
				"Name":        jsii.String(fmt.Sprintf("Connection to %s", name)),
				"Environment": jsii.String("production"),
				"ManagedBy":   jsii.String("cdktf"),
				"SourceVpcId": jsii.String(peer.SourceVpcId),
				"PeerVpcId":   jsii.String(peer.PeerVpcId),
			},
		}
		// Only set Accepter block if autoAccept is true
		if autoAccept {
			peeringConfig.Accepter = &vpcpeeringconnection.VpcPeeringConnectionAccepter{
				AllowRemoteVpcDnsResolution: jsii.Bool(peer.EnableDnsResolution),
			}
		}

		// Only set Accepter block if autoAccept is true
		if autoAccept {
			peeringConfig.Accepter = &vpcpeeringconnection.VpcPeeringConnectionAccepter{
				AllowRemoteVpcDnsResolution: jsii.Bool(peer.EnableDnsResolution),
			}
		}
		if sourceRegion != peerRegion {
			peeringConfig.PeerRegion = jsii.String(peerRegion)
		}

		peering := vpcpeeringconnection.NewVpcPeeringConnection(
			stack,
			jsii.String(fmt.Sprintf("VpcPeering%d", i)),
			peeringConfig,
		)
		vpcPeeringConnections = append(vpcPeeringConnections, peering)

		// ---------------------------------------------------------------------
		// If auto_accept is false, add an accepter resource in the peer account/region
		// ---------------------------------------------------------------------

		var accepter cdktf.TerraformResource
		if !autoAccept {
			accepter = cdktf.NewTerraformResource(stack, jsii.String(fmt.Sprintf("VpcPeeringAccepter%d", i)), &cdktf.TerraformResourceConfig{
				TerraformResourceType: jsii.String("aws_vpc_peering_connection_accepter"),
				Provider:              peerProvider,
				DependsOn:             &[]cdktf.ITerraformDependable{peering},
			})
			accepter.AddOverride(jsii.String("vpc_peering_connection_id"), peering.Id())
			accepter.AddOverride(jsii.String("auto_accept"), true)
			accepter.AddOverride(jsii.String("tags"), map[string]interface{}{
				"Name":        fmt.Sprintf("Connection to %s", name),
				"Environment": "production",
				"ManagedBy":   "cdktf",
				"SourceVpcId": peer.SourceVpcId,
				"PeerVpcId":   peer.PeerVpcId,
			})
		}

		// ---------------------------------------------------------------------
		// Bi-Directional Main Route Table Entries for Peering
		// ---------------------------------------------------------------------

		// Build dependsOn slice for resources that require an active peering
		var dependsOn []cdktf.ITerraformDependable
		dependsOn = append(dependsOn, peering)
		if !autoAccept && accepter != nil {
			dependsOn = append(dependsOn, accepter)
		}

		// Source → Peer: route to peer's main CIDR
		awsroute.NewRoute(stack, jsii.String(fmt.Sprintf("SourceToPeerMainRoute%d", i)), &awsroute.RouteConfig{
			RouteTableId:           sourceMainRt.Id(),
			DestinationCidrBlock:   peerVpcData.CidrBlock(),
			VpcPeeringConnectionId: peering.Id(),
			Provider:               sourceProvider,
			DependsOn:              &dependsOn,
		})

		// Peer → Source: route to source's main CIDR
		awsroute.NewRoute(stack, jsii.String(fmt.Sprintf("PeerToSourceMainRoute%d", i)), &awsroute.RouteConfig{
			RouteTableId:           peerMainRt.Id(),
			DestinationCidrBlock:   sourceVpcData.CidrBlock(),
			VpcPeeringConnectionId: peering.Id(),
			Provider:               peerProvider,
			DependsOn:              &dependsOn,
		})

		// ---------------------------------------------------------------------
		// Bi-Directional Subnet Route Table Entries (if enabled)
		// ---------------------------------------------------------------------
		if peer.HasExtraPeerRouteTables {
			// --- Source → Peer: Add routes in all source subnet route tables to peer's main CIDR ---
			sourceSubnets := dataawssubnets.NewDataAwsSubnets(stack, jsii.String(fmt.Sprintf("SourceSubnets%d", i)), &dataawssubnets.DataAwsSubnetsConfig{
				Provider: sourceProvider,
				Filter: &[]*dataawssubnets.DataAwsSubnetsFilter{
					{
						Name:   jsii.String("vpc-id"),
						Values: jsii.Strings(peer.SourceVpcId),
					},
				},
			})
			if sourceSubnets.Ids() != nil {
				for j, subnetId := range *sourceSubnets.Ids() {
					routeTable := dataawsroutetable.NewDataAwsRouteTable(stack, jsii.String(fmt.Sprintf("SourceSubnetRouteTable%d_%d", i, j)), &dataawsroutetable.DataAwsRouteTableConfig{
						SubnetId: subnetId,
						Provider: sourceProvider,
					})
					awsroute.NewRoute(stack, jsii.String(fmt.Sprintf("SourceSubnetToPeerRoute%d_%d", i, j)), &awsroute.RouteConfig{
						RouteTableId:           routeTable.Id(),
						DestinationCidrBlock:   peerVpcData.CidrBlock(),
						VpcPeeringConnectionId: peering.Id(),
						Provider:               sourceProvider,
						DependsOn:              &dependsOn,
					})
				}
			}

			// --- Peer → Source: Add routes in all peer subnet route tables to source's main CIDR ---
			peerSubnets := dataawssubnets.NewDataAwsSubnets(stack, jsii.String(fmt.Sprintf("PeerSubnets%d", i)), &dataawssubnets.DataAwsSubnetsConfig{
				Provider: peerProvider,
				Filter: &[]*dataawssubnets.DataAwsSubnetsFilter{
					{
						Name:   jsii.String("vpc-id"),
						Values: jsii.Strings(peer.PeerVpcId),
					},
				},
			})
			if peerSubnets.Ids() != nil {
				for j, subnetId := range *peerSubnets.Ids() {
					routeTable := dataawsroutetable.NewDataAwsRouteTable(stack, jsii.String(fmt.Sprintf("PeerSubnetRouteTable%d_%d", i, j)), &dataawsroutetable.DataAwsRouteTableConfig{
						SubnetId: subnetId,
						Provider: peerProvider,
					})
					awsroute.NewRoute(stack, jsii.String(fmt.Sprintf("PeerSubnetToSourceRoute%d_%d", i, j)), &awsroute.RouteConfig{
						RouteTableId:           routeTable.Id(),
						DestinationCidrBlock:   sourceVpcData.CidrBlock(),
						VpcPeeringConnectionId: peering.Id(),
						Provider:               peerProvider,
						DependsOn:              &dependsOn,
					})
				}
			}
		}
	}

	// -------------------------------------------------------------------------
	// Outputs for Peering Connection and Route Tables
	// -------------------------------------------------------------------------
	addOutputs(stack, peers, vpcPeeringConnections, sourceMainRouteTables, peerMainRouteTables)
	return stack
}

// -------------------------------------------------------------------------------------------------
// Output Helper
// -------------------------------------------------------------------------------------------------

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

// -------------------------------------------------------------------------------------------------
// YAML Config Loading and Conversion
// -------------------------------------------------------------------------------------------------

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

			enableDns := false
			if cfg.DnsResolution != nil {
				enableDns = cfg.DnsResolution[source]
			}
			hasExtraPeerRouteTables := false
			if cfg.AdditionalRoutes != nil {
				_, hasExtra := cfg.AdditionalRoutes[source]
				hasExtraPeerRouteTables = hasExtra
			}

			peerConfigs = append(peerConfigs, PeerConfig{
				SourceVpcId:             sourcePeer.VpcId,
				SourceRegion:            sourcePeer.Region,
				SourceRoleArn:           sourcePeer.RoleArn,
				PeerVpcId:               peerPeer.VpcId,
				PeerRegion:              peerPeer.Region,
				PeerRoleArn:             peerPeer.RoleArn,
				Name:                    target,
				EnableDnsResolution:     enableDns,
				HasExtraPeerRouteTables: hasExtraPeerRouteTables,
			})
		}
	}
	log.Printf("[convert] Returning %d peer configs", len(peerConfigs))
	return peerConfigs
}

// -------------------------------------------------------------------------------------------------
// Main Entrypoint
// -------------------------------------------------------------------------------------------------

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
