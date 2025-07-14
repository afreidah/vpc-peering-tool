// -------------------------------------------------------------------------------------------------
// CDKTF VPC Peering Stack with Bi-Directional Routing, DNS, and Automatic Subnet Route Management
// Handles cross-account/region peering with explicit accepter resource.
// -------------------------------------------------------------------------------------------------
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
	"github.com/hashicorp/terraform-cdk-go/cdktf"

	dataawsroutetable "cdk.tf/go/stack/generated/hashicorp/aws/dataawsroutetable"
	vpcpeeringconnection "cdk.tf/go/stack/generated/hashicorp/aws/vpcpeeringconnection"
)

// -------------------------------------------------------------------------------------------------
// Stack Construction
// -------------------------------------------------------------------------------------------------

func NewMyStack(scope constructs.Construct, id string, sourceID string, peers []PeerConfig) cdktf.TerraformStack {
	stack := cdktf.NewTerraformStack(scope, &id)

	cdktf.NewTerraformVariable(stack, jsii.String("source_id"), &cdktf.TerraformVariableConfig{
		Type:        jsii.String("string"),
		Description: jsii.String("The source identifier for this resource"),
		Default:     jsii.String("default-source"),
	})

	var vpcPeeringConnections []vpcpeeringconnection.VpcPeeringConnection
	var sourceMainRouteTables []dataawsroutetable.DataAwsRouteTable
	var peerMainRouteTables []dataawsroutetable.DataAwsRouteTable

	for i, peer := range peers {
		// --- Validate peer configuration or set defaults ---
		sourceRegion := peer.SourceRegion
		if sourceRegion == "" {
			sourceRegion = "us-west-2"
		}
		peerRegion := peer.PeerRegion
		if peerRegion == "" {
			peerRegion = "us-west-2"
		}

		// --- Setup providers ---
		sourceProviderName := fmt.Sprintf("SourceAWS%d", i)
		sourceProviderAlias := fmt.Sprintf("source%d", i)
		peerProviderName := fmt.Sprintf("PeerAWS%d", i)
		peerProviderAlias := fmt.Sprintf("peer%d", i)
		sourceProvider := CreateAwsProvider(stack, sourceProviderName, sourceProviderAlias, sourceRegion, peer.SourceRoleArn)
		peerProvider := CreateAwsProvider(stack, peerProviderName, peerProviderAlias, peerRegion, peer.PeerRoleArn)

		// --- Setup VPC data sources ---
		sourceVpcName := fmt.Sprintf("SourceVpcData%d", i)
		peerVpcName := fmt.Sprintf("PeerVpcData%d", i)
		sourceVpcData := CreateDataAwsVpc(stack, sourceVpcName, peer.SourceVpcId, sourceProvider)
		peerVpcData := CreateDataAwsVpc(stack, peerVpcName, peer.PeerVpcId, peerProvider)

		// --- Prepare main route table arguments and tags ---
		sourceMainRtName := fmt.Sprintf("SourceMainRouteTable%d", i)
		peerMainRtName := fmt.Sprintf("PeerMainRouteTable%d", i)

		// --- Create Main Route Tables ---
		sourceMainRt := CreateMainRouteTable(stack, sourceMainRtName, peer.SourceVpcId, sourceProvider)
		peerMainRt := CreateMainRouteTable(stack, peerMainRtName, peer.PeerVpcId, peerProvider)
		sourceMainRouteTables = append(sourceMainRouteTables, sourceMainRt)
		peerMainRouteTables = append(peerMainRouteTables, peerMainRt)

		// ---------------------------------------------------------------------
		// Create VPC Peering Connection with Options and Routes
		// ---------------------------------------------------------------------

		peerOwnerId := GetAccountIDFromRoleArn(peer.PeerRoleArn)
		name := peer.Name
		if name == "" {
			name = peer.PeerVpcId
		}

		// --- Only set options in aws_vpc_peering_connection_options, not here ---
		autoAccept := sourceRegion == peerRegion
		peeringConfig := &vpcpeeringconnection.VpcPeeringConnectionConfig{
			VpcId:       jsii.String(peer.SourceVpcId),
			PeerVpcId:   jsii.String(peer.PeerVpcId),
			PeerOwnerId: jsii.String(peerOwnerId),
			Provider:    sourceProvider,
			AutoAccept:  jsii.Bool(autoAccept),
			Tags: &map[string]*string{
				"Name":        jsii.String(fmt.Sprintf("Connection to %s", name)),
				"ManagedBy":   jsii.String("cdktf"),
				"SourceVpcId": jsii.String(peer.SourceVpcId),
				"PeerVpcId":   jsii.String(peer.PeerVpcId),
			},
		}

		// --- Only add PeerRegion if different from SourceRegion ---
		if sourceRegion != peerRegion {
			peeringConfig.PeerRegion = jsii.String(peerRegion)
		}

		// --- Create the VPC Peering Connection ---
		peering := vpcpeeringconnection.NewVpcPeeringConnection(
			stack,
			jsii.String(fmt.Sprintf("VpcPeering%d", i)),
			peeringConfig,
		)
		vpcPeeringConnections = append(vpcPeeringConnections, peering)

		// --- If auto_accept is false, add an accepter resource in the peer account/region ---
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

		// --- Peering Connection Options (for DNS, etc.) ---
		var optionsDependsOn []cdktf.ITerraformDependable
		optionsDependsOn = append(optionsDependsOn, peering)
		if accepter != nil {
			optionsDependsOn = append(optionsDependsOn, accepter)
		}

		// --- Create VPC Peering Options Resource ---
		opts := cdktf.NewTerraformResource(stack, jsii.String(fmt.Sprintf("VpcPeeringOptions%d", i)), &cdktf.TerraformResourceConfig{
			TerraformResourceType: jsii.String("aws_vpc_peering_connection_options"),
			Provider:              sourceProvider,
			DependsOn:             &optionsDependsOn,
		})
		opts.AddOverride(jsii.String("vpc_peering_connection_id"), peering.Id())
		opts.AddOverride(jsii.String("requester.allow_remote_vpc_dns_resolution"), peer.EnableDnsResolution)

		// --- If auto_accept is false, set accepter options as well ---
		var dependsOn []cdktf.ITerraformDependable
		dependsOn = append(dependsOn, peering)
		if !autoAccept && accepter != nil {
			dependsOn = append(dependsOn, accepter)
		}

		// --- Create Route Table Entries ---
		CreateRoute(
			stack,
			fmt.Sprintf("SourceToPeerMainRoute%d", i),
			sourceMainRt.Id(),
			peerVpcData.CidrBlock(),
			peering.Id(),
			sourceProvider,
			dependsOn,
		)

		// --- Craete Reverse Route Table Entries ---
		CreateRoute(
			stack,
			fmt.Sprintf("PeerToPeerMainRoute%d", i),
			jsii.String(*peerMainRt.Id()),
			jsii.String(*peering.Id()),
			peering.Id(),
			sourceProvider,
			dependsOn,
		)

		// --- Bi-Directional Subnet Route Table Entries.  Will handle extra peer route tables if specified ---
		if peer.HasExtraPeerRouteTables {
			// --- Create DataAwsRouteTable for Source Subnets ---
			CreateFilteredSubnetRoutes(
				stack,
				fmt.Sprintf("SourceSubnetToPeerRoute_%s_eachkey_%d", name, i),
				fmt.Sprintf("SourceSubnets%d", i),
				peer.SourceVpcId,
				sourceProvider,
				"tag:cdktf-source-main-rt",
				"",
				fmt.Sprintf("SourceSubnetRouteTable%d", i),
				peerVpcData.CidrBlock(),
				peering.Id(),
				dependsOn,
			)

			// --- Create DataAwsRouteTable for Peer Subnets ---
			CreateFilteredSubnetRoutes(
				stack,
				fmt.Sprintf("PeerSubnetToSourceRoute_%s_eachkey_%d", name, i),
				fmt.Sprintf("PeerSubnets%d", i),
				peer.PeerVpcId,
				peerProvider,
				"tag:cdktf-peer-main-rt",
				"",
				fmt.Sprintf("PeerSubnetRouteTable%d", i),
				sourceVpcData.CidrBlock(),
				peering.Id(),
				dependsOn,
			)
		}
	}

	AddOutputs(stack, peers, vpcPeeringConnections, sourceMainRouteTables, peerMainRouteTables)
	return stack
}

// -------------------------------------------------------------------------------------------------
// Main Entrypoint
// -------------------------------------------------------------------------------------------------

func main() {
	// --- Initialize logging ---
	log.SetFlags(0)
	log.SetOutput(os.Stdout)

	cfg := LoadConfig("peering.yaml")

	sourceID := os.Getenv("CDKTF_SOURCE")
	if sourceID == "" {
		sourceID = "default-source"
	}

	peers := ConvertToPeerConfigs(cfg, sourceID)

	if len(peers) == 0 {
		log.Fatalf("no peers matched for source: %s", sourceID)
	}

	app := cdktf.NewApp(nil)
	NewMyStack(app, "cdktf-vpc-peering-module", sourceID, peers)
	app.Synth()
}
