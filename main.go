// -------------------------------------------------------------------------------------------------
// CDKTF VPC Peering Stack with Bi-Directional Routing, DNS, and Automatic Subnet Route Management
// Handles cross-account/region peering with explicit accepter resource.
// -------------------------------------------------------------------------------------------------
package main

import (
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

/*
NewMyStack constructs the CDKTF stack for VPC peering, bi-directional routing, and DNS management.

Parameters:

	scope     - The CDKTF construct scope.
	id        - Logical stack identifier.
	sourceID  - The source identifier for this resource.
	peers     - Slice of PeerConfig describing all peering relationships.

Returns:

	cdktf.TerraformStack with all resources and outputs defined.
*/
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

		// --- Get core info on each peer ---
		core := SetupPeerCoreResources(stack, i, peer, sourceRegion, peerRegion)
		sourceMainRouteTables = append(sourceMainRouteTables, core.SourceMainRt)
		peerMainRouteTables = append(peerMainRouteTables, core.PeerMainRt)

		// --- Prepare peering connection and related resources ---
		peerOwnerID := GetAccountIDFromRoleArn(peer.PeerRoleArn)
		name := peer.Name
		if name == "" {
			name = peer.PeerVpcID
		}
		autoAccept := sourceRegion == peerRegion

		peeringRes := CreatePeeringResources(
			stack,
			i,
			peer,
			core,
			name,
			peerOwnerID,
			autoAccept,
			peerRegion,
		)
		vpcPeeringConnections = append(vpcPeeringConnections, peeringRes.Peering)

		// --- Create all main and subnet routes for this peer ---
		CreateBiDirectionalSubnetRoutes(
			stack,
			peer,
			core,
			peeringRes,
			name,
			i,
		)
	}

	AddOutputs(stack, peers, vpcPeeringConnections, sourceMainRouteTables, peerMainRouteTables)
	return stack
}

// -------------------------------------------------------------------------------------------------
// Main Entrypoint
// -------------------------------------------------------------------------------------------------

/*
main is the entrypoint for the CDKTF VPC peering stack application.

- Loads configuration from peering.yaml.
- Determines the source ID from environment or default.
- Converts config to PeerConfig slice.
- Fails if no peers match.
- Synthesizes the CDKTF app.
*/
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
