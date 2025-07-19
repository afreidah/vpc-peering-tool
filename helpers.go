package main

import (
	"fmt"
	"log"
	"os"
	"regexp"

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
)

// -------------------------------------------------------------------------------------------------
// Struct Definitions
// -------------------------------------------------------------------------------------------------

// PeerCoreResources holds the core AWS resources for a peer in a VPC peering relationship.
//
// Fields:
//
//	SourceProvider - Terraform provider for the source VPC/account.
//	PeerProvider   - Terraform provider for the peer VPC/account.
//	SourceVpcData  - Data source for the source VPC.
//	PeerVpcData    - Data source for the peer VPC.
//	SourceMainRt   - Data source for the source VPC's main route table.
//	PeerMainRt     - Data source for the peer VPC's main route table.
type PeerCoreResources struct {
	SourceProvider cdktf.TerraformProvider
	PeerProvider   cdktf.TerraformProvider
	SourceVpcData  dataawsvpc.DataAwsVpc
	PeerVpcData    dataawsvpc.DataAwsVpc
	SourceMainRt   dataawsroutetable.DataAwsRouteTable
	PeerMainRt     dataawsroutetable.DataAwsRouteTable
}

// PeerConfig defines the configuration for a single VPC peering connection, including DNS and extra route flag.
//
// Fields:
//
//	SourceVpcID             - VPC ID of the source.
//	SourceRegion            - AWS region of the source.
//	SourceRoleArn           - IAM role ARN for the source.
//	PeerVpcID               - VPC ID of the peer.
//	PeerRegion              - AWS region of the peer.
//	PeerRoleArn             - IAM role ARN for the peer.
//	Name                    - Logical name for this peering.
//	EnableDNSResolution     - Whether to enable DNS resolution across the peering.
//	HasExtraPeerRouteTables - Whether to add subnet routes for the peer.
type PeerConfig struct {
	SourceVpcID             string
	SourceRegion            string
	SourceRoleArn           string
	PeerVpcID               string
	PeerRegion              string
	PeerRoleArn             string
	Name                    string
	EnableDNSResolution     bool
	HasExtraPeerRouteTables bool // Controls whether to add subnet routes
}

// YAMLPeer represents a peer entry in the YAML file.
//
// Fields:
//
//	VpcID               - VPC ID.
//	Region              - AWS region.
//	RoleArn             - IAM role ARN.
//	DNSResolution       - Enable DNS resolution.
//	HasAdditionalRoutes - Enable additional subnet routes.
type YAMLPeer struct {
	VpcID               string `yaml:"vpc_id"`
	Region              string `yaml:"region"`
	RoleArn             string `yaml:"role_arn"`
	DNSResolution       bool   `yaml:"dns_resolution"`
	HasAdditionalRoutes bool   `yaml:"has_additional_routes"`
}

// YAMLConfig holds the structure of the YAML configuration file, including DNS and extra route flag.
//
// Fields:
//
//	Peers            - Map of peer names to YAMLPeer definitions.
//	PeeringMatrix    - Map of source peer names to lists of target peer names.
//	DNSResolution    - Optional map of peer names to DNS resolution flags.
//	AdditionalRoutes - Optional map of peer names to additional route lists.
type YAMLConfig struct {
	Peers            map[string]YAMLPeer `yaml:"peers"`
	PeeringMatrix    map[string][]string `yaml:"peering_matrix"`
	DNSResolution    map[string]bool     `yaml:"dns_resolution,omitempty"`
	AdditionalRoutes map[string][]string `yaml:"additional_routes,omitempty"`
}

// PeeringResources holds the resources related to a single VPC peering connection.
//
// Fields:
//
//	Peering   - The VPC peering connection resource.
//	Accepter  - The accepter resource (if cross-account/region).
//	Options   - The peering options resource.
//	DependsOn - List of dependencies for downstream resources.
type PeeringResources struct {
	Peering   vpcpeeringconnection.VpcPeeringConnection
	Accepter  cdktf.TerraformResource
	Options   cdktf.TerraformResource
	DependsOn []cdktf.ITerraformDependable
}

// -------------------------------------------------------------------------------------------------
// Helper: Extract account ID from role ARN
// -------------------------------------------------------------------------------------------------

// GetAccountIDFromRoleArn extracts the AWS account ID from a role ARN string.
//
// Parameters:
//
//	roleArn - The IAM role ARN string.
//
// Returns:
//
//	The AWS account ID as a string, or an empty string if not found.
func GetAccountIDFromRoleArn(roleArn string) string {
	re := regexp.MustCompile(`^arn:aws:iam::(\d+):`)
	matches := re.FindStringSubmatch(roleArn)
	if len(matches) == 2 {
		return matches[1]
	}
	return ""
}

// -------------------------------------------------------------------------------------------------
// YAML Config Loading and Conversion
// -------------------------------------------------------------------------------------------------

// LoadConfig loads and parses the YAML configuration file at the given path.
//
// Parameters:
//
//	path - Path to the YAML config file.
//
// Returns:
//
//	YAMLConfig struct populated from the file.
//
// Panics:
//
//	If the file cannot be read or parsed.
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

// ConvertToPeerConfigs converts a YAMLConfig and source filter into a slice of PeerConfig structs.
//
// Parameters:
//
//	cfg          - The loaded YAMLConfig.
//	sourceFilter - If non-empty, only include peers with this source.
//
// Returns:
//
//	Slice of PeerConfig structs for use in stack construction.
//
// Panics:
//
//	If required peer config entries are missing.
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
				SourceVpcID:             sourcePeer.VpcID,
				SourceRegion:            sourcePeer.Region,
				SourceRoleArn:           sourcePeer.RoleArn,
				PeerVpcID:               peerPeer.VpcID,
				PeerRegion:              peerPeer.Region,
				PeerRoleArn:             peerPeer.RoleArn,
				Name:                    target,
				EnableDNSResolution:     peerPeer.DNSResolution,
				HasExtraPeerRouteTables: peerPeer.HasAdditionalRoutes,
			})
		}
	}
	log.Printf("[convert] Returning %d peer configs", len(peerConfigs))
	return peerConfigs
}

// -------------------------------------------------------------------------------------------------
// Create AWS Provider
// -------------------------------------------------------------------------------------------------

// CreateAwsProvider creates a new AWS provider resource with the given configuration.
//
// Parameters:
//
//	stack   - The CDKTF construct stack.
//	name    - Logical name for the provider resource.
//	alias   - Provider alias.
//	region  - AWS region.
//	roleArn - IAM role ARN for assume role.
//
// Returns:
//
//	awsprovider.AwsProvider resource.
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

// CreateDataAwsVpc creates a data source for an AWS VPC.
//
// Parameters:
//
//	stack   - The CDKTF construct stack.
//	name    - Logical name for the data source.
//	vpcID   - VPC ID to look up.
//	provider- AWS provider to use.
//
// Returns:
//
//	dataawsvpc.DataAwsVpc resource.
func CreateDataAwsVpc(stack constructs.Construct, name, vpcID string, provider awsprovider.AwsProvider) dataawsvpc.DataAwsVpc {
	return dataawsvpc.NewDataAwsVpc(stack, jsii.String(name), &dataawsvpc.DataAwsVpcConfig{
		Id:       jsii.String(vpcID),
		Provider: provider,
	})
}

// -------------------------------------------------------------------------------------------------
// Create Data Source for Main Route Table
// -------------------------------------------------------------------------------------------------

// CreateMainRouteTable creates a data source for the main route table of a VPC.
//
// Parameters:
//
//	stack    - The CDKTF construct stack.
//	name     - Logical name for the data source.
//	vpcID    - VPC ID to look up.
//	provider - AWS provider to use.
//
// Returns:
//
//	dataawsroutetable.DataAwsRouteTable resource.
func CreateMainRouteTable(stack constructs.Construct, name, vpcID string, provider awsprovider.AwsProvider) dataawsroutetable.DataAwsRouteTable {
	return dataawsroutetable.NewDataAwsRouteTable(stack, jsii.String(name), &dataawsroutetable.DataAwsRouteTableConfig{
		VpcId:    jsii.String(vpcID),
		Provider: provider,
		Filter: &[]*dataawsroutetable.DataAwsRouteTableFilter{{
			Name:   jsii.String("association.main"),
			Values: jsii.Strings("true"),
		}},
	})
}

// -------------------------------------------------------------------------------------------------
// Output Helper
// -------------------------------------------------------------------------------------------------

// AddOutputs creates enhanced Terraform outputs for peering connection, main route table IDs,
// peering connection status, and DNS resolution settings.
//
// Parameters:
//
//	stack           - The CDKTF Terraform stack.
//	peers           - Slice of PeerConfig.
//	vpcs            - Slice of VpcPeeringConnection resources.
//	sourceTables    - Slice of DataAwsRouteTable for source VPCs.
//	peerTables      - Slice of DataAwsRouteTable for peer VPCs.
//
// Side Effects:
//   - Adds Terraform outputs to the stack for IDs, status, and DNS settings.
func AddOutputs(
	stack cdktf.TerraformStack,
	peers []PeerConfig,
	vpcs []vpcpeeringconnection.VpcPeeringConnection,
	sourceTables []dataawsroutetable.DataAwsRouteTable,
	peerTables []dataawsroutetable.DataAwsRouteTable,
) {
	for i := range peers {
		// --- Output VPC Peering Connection ID ---
		cdktf.NewTerraformOutput(stack, jsii.String(fmt.Sprintf("VpcPeeringConnectionId_%d", i)), &cdktf.TerraformOutputConfig{
			Value: vpcs[i].Id(),
		})

		// --- Output Source Main Route Table ID ---
		cdktf.NewTerraformOutput(stack, jsii.String(fmt.Sprintf("SourceMainRouteTableId_%d", i)), &cdktf.TerraformOutputConfig{
			Value: sourceTables[i].Id(),
		})

		// --- Output Peer Main Route Table ID ---
		cdktf.NewTerraformOutput(stack, jsii.String(fmt.Sprintf("PeerMainRouteTableId_%d", i)), &cdktf.TerraformOutputConfig{
			Value: peerTables[i].Id(),
		})

		// --- Output DNS Resolution Setting ---
		cdktf.NewTerraformOutput(stack, jsii.String(fmt.Sprintf("DnsResolutionEnabled_%d", i)), &cdktf.TerraformOutputConfig{
			Value: peers[i].EnableDNSResolution,
		})
	}
}

// CreateSubnetRoutes creates routes for each subnet in a VPC using a TerraformIterator escape hatch.
//
// Parameters:
//
//	stack      - The CDKTF Terraform stack.
//	namePrefix - Prefix for resource names.
//	subnetIDs  - List of subnet IDs.
//	provider   - AWS provider to use.
//	destCidr   - Destination CIDR block.
//	peeringID  - VPC peering connection ID.
//	dependsOn  - List of dependencies for this resource.
//
// Side Effects:
//   - Adds aws_route and data_aws_route_table resources for each subnet.
func CreateSubnetRoutes(
	stack cdktf.TerraformStack,
	namePrefix string,
	subnetIDs *[]*string,
	provider cdktf.TerraformProvider,
	destCidr *string,
	peeringID *string,
	dependsOn []cdktf.ITerraformDependable,
) {
	iterator := cdktf.TerraformIterator_FromList(subnetIDs)
	dataawsroutetable.NewDataAwsRouteTable(stack, jsii.String(namePrefix+"RouteTable"), &dataawsroutetable.DataAwsRouteTableConfig{
		ForEach:  iterator,
		SubnetId: jsii.String("${each.value}"),
		Provider: provider,
	})
	awsroute.NewRoute(stack, jsii.String(namePrefix+"Route"), &awsroute.RouteConfig{
		ForEach:                iterator,
		RouteTableId:           jsii.String("${data.aws_route_table." + namePrefix + "RouteTable[each.key].id}"),
		DestinationCidrBlock:   destCidr,
		VpcPeeringConnectionId: peeringID,
		Provider:               provider,
		DependsOn:              &dependsOn,
	})
}

// CreateRoute creates a route in a given route table for a VPC peering connection.
//
// Parameters:
//
//	stack        - The CDKTF Terraform stack.
//	name         - Logical name for the route resource.
//	routeTableID - ID of the route table.
//	destCidr     - Destination CIDR block.
//	peeringID    - VPC peering connection ID.
//	provider     - AWS provider to use.
//	dependsOn    - List of dependencies for this resource.
//
// Side Effects:
//   - Adds an aws_route resource to the stack.
func CreateRoute(
	stack cdktf.TerraformStack,
	name string,
	routeTableID *string,
	destCidr *string,
	peeringID *string,
	provider cdktf.TerraformProvider,
	dependsOn []cdktf.ITerraformDependable,
) {
	awsroute.NewRoute(stack, jsii.String(name), &awsroute.RouteConfig{
		RouteTableId:           routeTableID,
		DestinationCidrBlock:   destCidr,
		VpcPeeringConnectionId: peeringID,
		Provider:               provider,
		DependsOn:              &dependsOn,
	})
}

// -------------------------------------------------------------------------------------------------
// Create Filtered Subnet Routes
// -------------------------------------------------------------------------------------------------

// CreateFilteredSubnetRoutes creates subnet routes for subnets matching a tag filter.
//
// Parameters:
//
//	stack                  - The CDKTF Terraform stack.
//	namePrefix             - Prefix for resource names.
//	subnetResourceName     - Logical name for the subnet data source.
//	vpcID                  - VPC ID to filter subnets.
//	provider               - AWS provider to use.
//	tagFilterName          - Tag name to filter subnets.
//	tagFilterValue         - Tag value to filter subnets.
//	routeTableResourceName - Logical name for the route table data source.
//	destCidr               - Destination CIDR block.
//	peeringID              - VPC peering connection ID.
//	dependsOn              - List of dependencies for this resource.
//
// Side Effects:
//   - Adds subnet route resources for matching subnets.
func CreateFilteredSubnetRoutes(
	stack cdktf.TerraformStack,
	namePrefix string,
	subnetResourceName string,
	vpcID string,
	provider cdktf.TerraformProvider,
	tagFilterName string,
	tagFilterValue string,
	routeTableResourceName string,
	destCidr *string,
	peeringID *string,
	dependsOn []cdktf.ITerraformDependable,
) {
	subnets := dataawssubnets.NewDataAwsSubnets(stack, jsii.String(subnetResourceName), &dataawssubnets.DataAwsSubnetsConfig{
		Provider: provider,
		Filter: &[]*dataawssubnets.DataAwsSubnetsFilter{
			{
				Name:   jsii.String("vpc-id"),
				Values: jsii.Strings(vpcID),
			},
			{
				Name:   jsii.String(tagFilterName),
				Values: jsii.Strings(tagFilterValue),
			},
		},
	})

	if subnets.Ids() != nil {
		CreateSubnetRoutes(stack, namePrefix, subnets.Ids(), provider, destCidr, peeringID, dependsOn)
	}
}

// -------------------------------------------------------------------------------------------------
// Setup core AWS resources for a peer
// -------------------------------------------------------------------------------------------------

// SetupPeerCoreResources creates all core AWS provider and data source resources for a peer.
//
// Parameters:
//
//	stack        - The CDKTF Terraform stack.
//	i            - Index of the peer (for unique naming).
//	peer         - PeerConfig for this peer.
//	sourceRegion - AWS region for the source.
//	peerRegion   - AWS region for the peer.
//
// Returns:
//
//	PeerCoreResources struct with all created resources.
func SetupPeerCoreResources(
	stack cdktf.TerraformStack,
	i int,
	peer PeerConfig,
	sourceRegion, peerRegion string,
) PeerCoreResources {
	sourceProviderName := fmt.Sprintf("SourceAWS%d", i)
	sourceProviderAlias := fmt.Sprintf("source%d", i)
	peerProviderName := fmt.Sprintf("PeerAWS%d", i)
	peerProviderAlias := fmt.Sprintf("peer%d", i)
	sourceProvider := CreateAwsProvider(stack, sourceProviderName, sourceProviderAlias, sourceRegion, peer.SourceRoleArn)
	peerProvider := CreateAwsProvider(stack, peerProviderName, peerProviderAlias, peerRegion, peer.PeerRoleArn)

	sourceVpcName := fmt.Sprintf("SourceVpcData%d", i)
	peerVpcName := fmt.Sprintf("PeerVpcData%d", i)
	sourceVpcData := CreateDataAwsVpc(stack, sourceVpcName, peer.SourceVpcID, sourceProvider)
	peerVpcData := CreateDataAwsVpc(stack, peerVpcName, peer.PeerVpcID, peerProvider)

	sourceMainRtName := fmt.Sprintf("SourceMainRouteTable%d", i)
	peerMainRtName := fmt.Sprintf("PeerMainRouteTable%d", i)
	sourceMainRt := CreateMainRouteTable(stack, sourceMainRtName, peer.SourceVpcID, sourceProvider)
	peerMainRt := CreateMainRouteTable(stack, peerMainRtName, peer.PeerVpcID, peerProvider)

	return PeerCoreResources{
		SourceProvider: sourceProvider,
		PeerProvider:   peerProvider,
		SourceVpcData:  sourceVpcData,
		PeerVpcData:    peerVpcData,
		SourceMainRt:   sourceMainRt,
		PeerMainRt:     peerMainRt,
	}
}

// -------------------------------------------------------------------------------------------------
// Peering Resources Helper: Creates peering connection, accepter, and options resources
// -------------------------------------------------------------------------------------------------

// CreatePeeringResources creates the VPC peering connection, conditional accepter, and options resources.
//
// Parameters:
//
//	stack       - The CDKTF Terraform stack.
//	i           - Index of the peer (for unique naming).
//	peer        - PeerConfig for this peer.
//	core        - PeerCoreResources for this peer.
//	name        - Logical name for this peering.
//	peerOwnerID - AWS account ID of the peer.
//	autoAccept  - Whether to auto-accept the peering.
//	peerRegion  - AWS region for the peer.
//
// Returns:
//
//	PeeringResources struct with all created resources and dependencies.
func CreatePeeringResources(
	stack cdktf.TerraformStack,
	i int,
	peer PeerConfig,
	core PeerCoreResources,
	name string,
	peerOwnerID string,
	autoAccept bool,
	peerRegion string,
) PeeringResources {
	// --- Build peering connection config ---
	peeringConfig := &vpcpeeringconnection.VpcPeeringConnectionConfig{
		VpcId:       jsii.String(peer.SourceVpcID),
		PeerVpcId:   jsii.String(peer.PeerVpcID),
		PeerOwnerId: jsii.String(peerOwnerID),
		Provider:    core.SourceProvider,
		AutoAccept:  jsii.Bool(autoAccept),
		Tags: &map[string]*string{
			"Name":        jsii.String(fmt.Sprintf("Connection to %s", name)),
			"ManagedBy":   jsii.String("cdktf"),
			"SourceVpcId": jsii.String(peer.SourceVpcID),
			"PeerVpcId":   jsii.String(peer.PeerVpcID),
		},
	}
	if core.SourceProvider != core.PeerProvider {
		peeringConfig.PeerRegion = jsii.String(peerRegion)
	}

	// --- Create the VPC Peering Connection ---
	peering := vpcpeeringconnection.NewVpcPeeringConnection(
		stack,
		jsii.String(fmt.Sprintf("VpcPeering%d", i)),
		peeringConfig,
	)

	// --- If auto_accept is false, add an accepter resource in the peer account/region ---
	var accepter cdktf.TerraformResource
	if !autoAccept {
		accepter = cdktf.NewTerraformResource(stack, jsii.String(fmt.Sprintf("VpcPeeringAccepter%d", i)), &cdktf.TerraformResourceConfig{
			TerraformResourceType: jsii.String("aws_vpc_peering_connection_accepter"),
			Provider:              core.PeerProvider,
			DependsOn:             &[]cdktf.ITerraformDependable{peering},
		})
		accepter.AddOverride(jsii.String("vpc_peering_connection_id"), peering.Id())
		accepter.AddOverride(jsii.String("auto_accept"), true)
		accepter.AddOverride(jsii.String("tags"), map[string]interface{}{
			"Name":        fmt.Sprintf("Connection to %s", name),
			"Environment": "production",
			"ManagedBy":   "cdktf",
			"SourceVpcId": peer.SourceVpcID,
			"PeerVpcId":   peer.PeerVpcID,
		})
	}

	// --- Peering Connection Options (for DNS, etc.) ---
	var optionsDependsOn []cdktf.ITerraformDependable
	optionsDependsOn = append(optionsDependsOn, peering)
	if accepter != nil {
		optionsDependsOn = append(optionsDependsOn, accepter)
	}

	opts := cdktf.NewTerraformResource(stack, jsii.String(fmt.Sprintf("VpcPeeringOptions%d", i)), &cdktf.TerraformResourceConfig{
		TerraformResourceType: jsii.String("aws_vpc_peering_connection_options"),
		Provider:              core.SourceProvider,
		DependsOn:             &optionsDependsOn,
	})
	opts.AddOverride(jsii.String("vpc_peering_connection_id"), peering.Id())
	opts.AddOverride(jsii.String("requester.allow_remote_vpc_dns_resolution"), peer.EnableDNSResolution)

	// --- Prepare dependsOn for downstream resources ---
	var dependsOn []cdktf.ITerraformDependable
	dependsOn = append(dependsOn, peering)
	if !autoAccept && accepter != nil {
		dependsOn = append(dependsOn, accepter)
	}

	return PeeringResources{
		Peering:   peering,
		Accepter:  accepter,
		Options:   opts,
		DependsOn: dependsOn,
	}
}

// -------------------------------------------------------------------------------------------------
// Create bi-directional subnet routes for a peer if extra route tables are enabled
// -------------------------------------------------------------------------------------------------

// CreateBiDirectionalSubnetRoutes creates all main and subnet route table entries required for
// bi-directional routing between two VPCs in a peering relationship.
//
// This function:
//   - Adds a route from the source VPC's main route table to the peer VPC's CIDR via the peering connection.
//   - Adds a reverse route from the peer VPC's main route table to the source VPC's CIDR via the peering connection.
//   - If HasExtraPeerRouteTables is enabled, creates additional subnet route table entries for both source and peer VPCs,
//     using tag-based filtering and CDKTF's TerraformIterator escape hatch for dynamic resource creation.
//
// Parameters:
//
//	stack      - The CDKTF Terraform stack context.
//	peer       - The PeerConfig struct describing the current peering relationship.
//	core       - PeerCoreResources containing providers, VPC data, and main route tables for both sides.
//	peeringRes - PeeringResources containing the peering connection, accepter, options, and dependency chain.
//	name       - A unique name for resource naming and tagging.
//	i          - The index of the current peer in the iteration (for unique resource names).
//
// Side Effects:
//   - Creates AWS route and data source resources in the provided stack.
//   - Uses CDKTF escape hatches for dynamic subnet route creation.
//
// Example:
//
//	CreateBiDirectionalSubnetRoutes(stack, peer, core, peeringRes, "my-peer", 0)
//
// See Also:
//   - CreateRoute
//   - CreateFilteredSubnetRoutes
//   - PeerCoreResources
//   - PeeringResources
func CreateBiDirectionalSubnetRoutes(
	stack cdktf.TerraformStack,
	peer PeerConfig,
	core PeerCoreResources,
	peeringRes PeeringResources,
	name string,
	i int,
) {
	// --- Create Route Table Entries ---
	CreateRoute(
		stack,
		fmt.Sprintf("SourceToPeerMainRoute%d", i),
		core.SourceMainRt.Id(),
		core.PeerVpcData.CidrBlock(),
		peeringRes.Peering.Id(),
		core.SourceProvider,
		peeringRes.DependsOn,
	)

	// --- Create Reverse Route Table Entries ---
	CreateRoute(
		stack,
		fmt.Sprintf("PeerToPeerMainRoute%d", i),
		core.PeerMainRt.Id(),
		core.SourceVpcData.CidrBlock(),
		peeringRes.Peering.Id(),
		core.PeerProvider,
		peeringRes.DependsOn,
	)

	// --- Bi-Directional Subnet Route Table Entries.  Will handle extra peer route tables if specified ---
	if peer.HasExtraPeerRouteTables {
		// --- Create DataAwsRouteTable for Source Subnets ---
		CreateFilteredSubnetRoutes(
			stack,
			fmt.Sprintf("SourceSubnetToPeerRoute_%s_eachkey_%d", name, i),
			fmt.Sprintf("SourceSubnets%d", i),
			peer.SourceVpcID,
			core.SourceProvider,
			"tag:cdktf-source-main-rt",
			"",
			fmt.Sprintf("SourceSubnetRouteTable%d", i),
			core.PeerVpcData.CidrBlock(),
			peeringRes.Peering.Id(),
			peeringRes.DependsOn,
		)

		// --- Create DataAwsRouteTable for Peer Subnets ---
		CreateFilteredSubnetRoutes(
			stack,
			fmt.Sprintf("PeerSubnetToSourceRoute_%s_eachkey_%d", name, i),
			fmt.Sprintf("PeerSubnets%d", i),
			peer.PeerVpcID,
			core.PeerProvider,
			"tag:cdktf-peer-main-rt",
			"",
			fmt.Sprintf("PeerSubnetRouteTable%d", i),
			core.SourceVpcData.CidrBlock(),
			peeringRes.Peering.Id(),
			peeringRes.DependsOn,
		)
	}
}
