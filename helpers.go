package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	dataawsroutetable "cdk.tf/go/stack/generated/hashicorp/aws/dataawsroutetable"
	dataawssubnets "cdk.tf/go/stack/generated/hashicorp/aws/dataawssubnets"
	dataawsvpc "cdk.tf/go/stack/generated/hashicorp/aws/dataawsvpc"
	awsprovider "cdk.tf/go/stack/generated/hashicorp/aws/provider"
	awsroute "cdk.tf/go/stack/generated/hashicorp/aws/route"
	vpcpeeringconnection "cdk.tf/go/stack/generated/hashicorp/aws/vpcpeeringconnection"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
	"github.com/hashicorp/terraform-cdk-go/cdktf"
	"gopkg.in/yaml.v2"

	vpcpeeringconnectionaccepter "cdk.tf/go/stack/generated/hashicorp/aws/vpcpeeringconnectionaccepter"
)

// -------------------------------------------------------------------------------------------------
// Helper: Extract account ID from role ARN
// -------------------------------------------------------------------------------------------------

func GetAccountIDFromRoleArn(roleArn string) string {
	parts := strings.Split(roleArn, ":")
	if len(parts) >= 5 {
		return parts[4]
	}
	return ""
}

// -------------------------------------------------------------------------------------------------
// YAML Config Loading and Conversion
// -------------------------------------------------------------------------------------------------

func LoadConfig(path string) YAMLConfig {
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

// --------------------------------------------------------------------------------------------------
// Convert YAML Config to PeerConfig
// --------------------------------------------------------------------------------------------------

func ConvertToPeerConfigs(cfg YAMLConfig, sourceFilter string) []PeerConfig {
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
				SourceVpcId:             sourcePeer.VpcId,
				SourceRegion:            sourcePeer.Region,
				SourceRoleArn:           sourcePeer.RoleArn,
				PeerVpcId:               peerPeer.VpcId,
				PeerRegion:              peerPeer.Region,
				PeerRoleArn:             peerPeer.RoleArn,
				PeerVpcCidr:             peerPeer.CidrBlock, // <-- Add this line
				Name:                    target,
				EnableDnsResolution:     sourcePeer.DnsResolution,
				HasExtraPeerRouteTables: sourcePeer.HasAdditionalRoutes,
			})
		}
	}

	log.Printf("[convert] Returning %d peer configs", len(peerConfigs))
	return peerConfigs
}

// -------------------------------------------------------------------------------------------------
// Output Helper
// -------------------------------------------------------------------------------------------------
// Outputs peering connection IDs, main route tables, and peer subnet IDs for use in downstream consumers.

func AddOutputs(
	stack cdktf.TerraformStack,
	peers []PeerConfig,
	vpcs []vpcpeeringconnection.VpcPeeringConnection,
	sourceTables []dataawsroutetable.DataAwsRouteTable,
	peerTables []dataawsroutetable.DataAwsRouteTable,
	peerSubnetIds [][]*string,
) {
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
		cdktf.NewTerraformOutput(stack, jsii.String(fmt.Sprintf("PeerSubnetIds_%d", i)), &cdktf.TerraformOutputConfig{
			Value: peerSubnetIds[i],
		})
	}
}

// -------------------------------------------------------------------------------------------------
// Create AWS Provider
// -------------------------------------------------------------------------------------------------

func CreateAwsProvider(stack constructs.Construct, name, alias, region, roleArn string) awsprovider.AwsProvider {
	return awsprovider.NewAwsProvider(stack, jsii.String(name), &awsprovider.AwsProviderConfig{
		Region: jsii.String(region),
		Alias:  jsii.String(alias),
		AssumeRole: &[]*awsprovider.AwsProviderAssumeRole{{
			RoleArn: jsii.String(roleArn),
		}},
	})
}

// -------------------------------------------------------------------------------------------------
// Create Data Sources for VPC
// -------------------------------------------------------------------------------------------------

func CreateDataAwsVpc(stack constructs.Construct, name, vpcId string, provider awsprovider.AwsProvider) dataawsvpc.DataAwsVpc {
	return dataawsvpc.NewDataAwsVpc(stack, jsii.String(name), &dataawsvpc.DataAwsVpcConfig{
		Id:       jsii.String(vpcId),
		Provider: provider,
	})
}

// -------------------------------------------------------------------------------------------------
// Create Data Source for Main Route Table
// -------------------------------------------------------------------------------------------------

func CreateMainRouteTable(stack constructs.Construct, name, vpcId string, provider awsprovider.AwsProvider) dataawsroutetable.DataAwsRouteTable {
	return dataawsroutetable.NewDataAwsRouteTable(stack, jsii.String(name), &dataawsroutetable.DataAwsRouteTableConfig{
		VpcId:    jsii.String(vpcId),
		Provider: provider,
		Filter: &[]*dataawsroutetable.DataAwsRouteTableFilter{{
			Name:   jsii.String("association.main"),
			Values: jsii.Strings("true"),
		}},
	})
}

// -------------------------------------------------------------------------------------------------
// Create VPC Peering Accepter
// -------------------------------------------------------------------------------------------------

func CreateVpcPeeringAccepter(
	stack constructs.Construct,
	alias string,
	peering vpcpeeringconnection.VpcPeeringConnection,
	provider cdktf.TerraformProvider,
) vpcpeeringconnectionaccepter.VpcPeeringConnectionAccepterA {
	return vpcpeeringconnectionaccepter.NewVpcPeeringConnectionAccepterA(stack, jsii.String(fmt.Sprintf("VpcPeeringAccepter%s", alias)), &vpcpeeringconnectionaccepter.VpcPeeringConnectionAccepterAConfig{
		VpcPeeringConnectionId: peering.Id(),
		AutoAccept:             jsii.Bool(true),
		Provider:               provider,
	})
}

// -------------------------------------------------------------------------------------------------
// Create VPC Peering Connection
// -------------------------------------------------------------------------------------------------

func CreateVpcPeeringConnection(stack constructs.Construct, i int, peer PeerConfig, sourceProvider awsprovider.AwsProvider) vpcpeeringconnection.VpcPeeringConnection {
	autoAccept := peer.SourceRegion == peer.PeerRegion
	return vpcpeeringconnection.NewVpcPeeringConnection(stack, jsii.String(fmt.Sprintf("VpcPeering%d", i)), &vpcpeeringconnection.VpcPeeringConnectionConfig{
		VpcId:       jsii.String(peer.SourceVpcId),
		PeerVpcId:   jsii.String(peer.PeerVpcId),
		PeerOwnerId: jsii.String(GetAccountIDFromRoleArn(peer.PeerRoleArn)),
		Provider:    sourceProvider,
		AutoAccept:  jsii.Bool(autoAccept),
		PeerRegion:  jsii.String(peer.PeerRegion),
		Requester: &vpcpeeringconnection.VpcPeeringConnectionRequester{
			AllowRemoteVpcDnsResolution: jsii.Bool(peer.EnableDnsResolution),
		},
		Tags: &map[string]*string{
			"Name":        jsii.String(fmt.Sprintf("Peering-%s", peer.Name)),
			"SourceVpcId": jsii.String(peer.SourceVpcId),
			"PeerVpcId":   jsii.String(peer.PeerVpcId),
		},
	})
}

// -------------------------------------------------------------------------------------------------
// Get VPC Subnet IDs (Tokenized List)
// -------------------------------------------------------------------------------------------------
// Returns a tokenized list of subnet IDs for the given VPC.
// This is intended for use with CDKTF's TerraformIterator escape hatch for per-subnet resource creation.

func GetVpcSubnetIds(stack constructs.Construct, name string, vpcId string, provider awsprovider.AwsProvider) *[]*string {
	subnets := dataawssubnets.NewDataAwsSubnets(stack, jsii.String(name+"Subnets"), &dataawssubnets.DataAwsSubnetsConfig{
		Filter: &[]*dataawssubnets.DataAwsSubnetsFilter{{
			Name:   jsii.String("vpc-id"),
			Values: jsii.Strings(vpcId),
		}},
		Provider: provider,
	})
	return subnets.Ids()
}

// -------------------------------------------------------------------------------------------------
// Create Peering Route(s) for Additional Peer Subnets (using TerraformIterator)
// -------------------------------------------------------------------------------------------------
// If HasExtraPeerRouteTables is true for a peer, this helper will create a route in each peer subnet's route table
// using the TerraformIterator escape hatch to handle tokenized subnet lists at apply time.

func CreatePeerSubnetRoutes(
	stack cdktf.TerraformStack,
	namePrefix string,
	subnetIds *[]*string,
	routeTableId *string,
	destCidr *string,
	peeringId *string,
	provider cdktf.TerraformProvider,
	dependsOn []cdktf.ITerraformDependable,
) {
	iterator := cdktf.TerraformIterator_FromList(subnetIds)
	awsroute.NewRoute(stack, jsii.String(fmt.Sprintf("%sPeerSubnetRoute", namePrefix)), &awsroute.RouteConfig{
		ForEach:                iterator,
		RouteTableId:           routeTableId,
		DestinationCidrBlock:   destCidr,
		VpcPeeringConnectionId: peeringId,
		Provider:               provider,
		DependsOn:              &dependsOn,
	})
}

// -------------------------------------------------------------------------------------------------
// Create VPC Peering Connection Options with explicit dependency on accepter
// -------------------------------------------------------------------------------------------------
func CreatePeeringConnectionOptions(
	stack constructs.Construct,
	name string,
	peeringId *string,
	provider cdktf.TerraformProvider,
	accepter cdktf.TerraformResource,
	enableDns bool,
) {
	opts := cdktf.NewTerraformResource(stack, jsii.String(fmt.Sprintf("VpcPeeringOptions%s", name)), &cdktf.TerraformResourceConfig{
		TerraformResourceType: jsii.String("aws_vpc_peering_connection_options"),
		Provider:              provider,
		DependsOn:             &[]cdktf.ITerraformDependable{accepter},
	})
	opts.AddOverride(jsii.String("vpc_peering_connection_id"), peeringId)
	opts.AddOverride(jsii.String("requester.allow_remote_vpc_dns_resolution"), enableDns)
}
