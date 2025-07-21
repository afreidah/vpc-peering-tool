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
type PeerCoreResources struct {
	SourceProvider cdktf.TerraformProvider
	PeerProvider   cdktf.TerraformProvider
	SourceVpcData  dataawsvpc.DataAwsVpc
	PeerVpcData    dataawsvpc.DataAwsVpc
	SourceMainRt   dataawsroutetable.DataAwsRouteTable
	PeerMainRt     dataawsroutetable.DataAwsRouteTable
}

// PeerConfig defines the configuration for a single VPC peering connection.
type PeerConfig struct {
	SourceVpcID             string // VPC ID of the source.
	SourceRegion            string // AWS region of the source.
	SourceRoleArn           string // IAM role ARN for the source.
	PeerVpcID               string // VPC ID of the peer.
	PeerRegion              string // AWS region of the peer.
	PeerRoleArn             string // IAM role ARN for the peer.
	Name                    string // Logical name for this peering.
	EnableDNSResolution     bool   // Enables DNS resolution across the peering.
	HasExtraPeerRouteTables bool   // Adds subnet routes for the peer.
}

// YAMLPeer represents a peer entry in the YAML file.
type YAMLPeer struct {
	VpcID               string `yaml:"vpc_id"`                // VPC ID.
	Region              string `yaml:"region"`                // AWS region.
	RoleArn             string `yaml:"role_arn"`              // IAM role ARN.
	DNSResolution       bool   `yaml:"dns_resolution"`        // Enables DNS resolution.
	HasAdditionalRoutes bool   `yaml:"has_additional_routes"` // Enables additional subnet routes.
}

// YAMLConfig holds the structure of the YAML configuration file.
type YAMLConfig struct {
	Peers            map[string]YAMLPeer `yaml:"peers"`                       // Map of peer names to YAMLPeer definitions.
	PeeringMatrix    map[string][]string `yaml:"peering_matrix"`              // Map of source peer names to lists of target peer names.
	DNSResolution    map[string]bool     `yaml:"dns_resolution,omitempty"`    // Optional map of peer names to DNS resolution flags.
	AdditionalRoutes map[string][]string `yaml:"additional_routes,omitempty"` // Optional map of peer names to additional route lists.
}

// PeeringResources holds the resources related to a single VPC peering connection.
type PeeringResources struct {
	Peering   vpcpeeringconnection.VpcPeeringConnection // The VPC peering connection resource.
	Accepter  cdktf.TerraformResource                   // The accepter resource (if cross-account/region).
	Options   cdktf.TerraformResource                   // The peering options resource.
	DependsOn []cdktf.ITerraformDependable              // List of dependencies for downstream resources.
}

// -------------------------------------------------------------------------------------------------
// Interfaces for Resource Creation (for testability)
// -------------------------------------------------------------------------------------------------

// AwsProviderFactory defines an interface for creating AWS providers.
type AwsProviderFactory interface {
	Create(stack constructs.Construct, name, alias, region, roleArn string) awsprovider.AwsProvider
}

// DataAwsVpcFactory defines an interface for creating AWS VPC data sources.
type DataAwsVpcFactory interface {
	Create(stack constructs.Construct, name, vpcID string, provider awsprovider.AwsProvider) dataawsvpc.DataAwsVpc
}

// DataAwsRouteTableFactory defines an interface for creating main route table data sources.
type DataAwsRouteTableFactory interface {
	Create(stack constructs.Construct, name, vpcID string, provider awsprovider.AwsProvider) dataawsroutetable.DataAwsRouteTable
}

// RealAwsProviderFactory is the production implementation of AwsProviderFactory.
type RealAwsProviderFactory struct{}

// Create creates a new AWS provider resource.
func (f *RealAwsProviderFactory) Create(stack constructs.Construct, name, alias, region, roleArn string) awsprovider.AwsProvider {
	return awsprovider.NewAwsProvider(stack, jsii.String(name), &awsprovider.AwsProviderConfig{
		Region: jsii.String(region),
		Alias:  jsii.String(alias),
		AssumeRole: &[]*awsprovider.AwsProviderAssumeRole{{
			RoleArn: jsii.String(roleArn),
		}},
	})
}

// RealDataAwsVpcFactory is the production implementation of DataAwsVpcFactory.
type RealDataAwsVpcFactory struct{}

// Create creates a new AWS VPC data source.
func (f *RealDataAwsVpcFactory) Create(stack constructs.Construct, name, vpcID string, provider awsprovider.AwsProvider) dataawsvpc.DataAwsVpc {
	return dataawsvpc.NewDataAwsVpc(stack, jsii.String(name), &dataawsvpc.DataAwsVpcConfig{
		Id:       jsii.String(vpcID),
		Provider: provider,
	})
}

// RealDataAwsRouteTableFactory is the production implementation of DataAwsRouteTableFactory.
type RealDataAwsRouteTableFactory struct{}

// Create creates a new main route table data source.
func (f *RealDataAwsRouteTableFactory) Create(stack constructs.Construct, name, vpcID string, provider awsprovider.AwsProvider) dataawsroutetable.DataAwsRouteTable {
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
// YAML Config Loading and Conversion
// -------------------------------------------------------------------------------------------------

// LoadConfig loads and parses the YAML configuration file at the given path. It panics if the file cannot be read or parsed.
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

// ConvertToPeerConfigs converts a YAMLConfig and optional source filter into a slice of PeerConfig structs.
// It panics if required peer config entries are missing.
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
// ARN and Account Helpers
// -------------------------------------------------------------------------------------------------

// GetAccountIDFromRoleArn extracts the AWS account ID from a role ARN string.
// It returns the account ID as a string, or an empty string if not found.
func GetAccountIDFromRoleArn(roleArn string) string {
	re := regexp.MustCompile(`^arn:aws:iam::(\d+):`)
	matches := re.FindStringSubmatch(roleArn)
	if len(matches) == 2 {
		return matches[1]
	}
	return ""
}

// -------------------------------------------------------------------------------------------------
// AWS Provider and Data Source Creation (via interfaces)
// -------------------------------------------------------------------------------------------------

// SetupPeerCoreResources creates all core AWS provider and data source resources for a peer.
// Uses factories for testability.
func SetupPeerCoreResources(
	providerFactory AwsProviderFactory,
	vpcFactory DataAwsVpcFactory,
	rtFactory DataAwsRouteTableFactory,
	stack cdktf.TerraformStack,
	i int,
	peer PeerConfig,
	sourceRegion, peerRegion string,
) PeerCoreResources {
	sourceProviderName := fmt.Sprintf("SourceAWS%d", i)
	sourceProviderAlias := fmt.Sprintf("source%d", i)
	peerProviderName := fmt.Sprintf("PeerAWS%d", i)
	peerProviderAlias := fmt.Sprintf("peer%d", i)
	sourceProvider := providerFactory.Create(stack, sourceProviderName, sourceProviderAlias, sourceRegion, peer.SourceRoleArn)
	peerProvider := providerFactory.Create(stack, peerProviderName, peerProviderAlias, peerRegion, peer.PeerRoleArn)

	sourceVpcName := fmt.Sprintf("SourceVpcData%d", i)
	peerVpcName := fmt.Sprintf("PeerVpcData%d", i)
	sourceVpcData := vpcFactory.Create(stack, sourceVpcName, peer.SourceVpcID, sourceProvider)
	peerVpcData := vpcFactory.Create(stack, peerVpcName, peer.PeerVpcID, peerProvider)

	sourceMainRtName := fmt.Sprintf("SourceMainRouteTable%d", i)
	peerMainRtName := fmt.Sprintf("PeerMainRouteTable%d", i)
	sourceMainRt := rtFactory.Create(stack, sourceMainRtName, peer.SourceVpcID, sourceProvider)
	peerMainRt := rtFactory.Create(stack, peerMainRtName, peer.PeerVpcID, peerProvider)

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
// Output and Route Helpers
// -------------------------------------------------------------------------------------------------

// AddOutputs creates Terraform outputs for peering connection, main route table IDs, peering connection status, and DNS resolution settings.
func AddOutputs(
	stack cdktf.TerraformStack,
	peers []PeerConfig,
	vpcs []vpcpeeringconnection.VpcPeeringConnection,
	sourceTables []dataawsroutetable.DataAwsRouteTable,
	peerTables []dataawsroutetable.DataAwsRouteTable,
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
		cdktf.NewTerraformOutput(stack, jsii.String(fmt.Sprintf("DnsResolutionEnabled_%d", i)), &cdktf.TerraformOutputConfig{
			Value: peers[i].EnableDNSResolution,
		})
	}
}

// CreateSubnetRoutes creates routes for each subnet in a VPC using a TerraformIterator escape hatch.
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

// CreateFilteredSubnetRoutes creates subnet routes for subnets matching a tag filter.
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
// Core Resource and Peering Logic
// -------------------------------------------------------------------------------------------------

// CreatePeeringResources creates the VPC peering connection, conditional accepter, and options resources.
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

	peering := vpcpeeringconnection.NewVpcPeeringConnection(
		stack,
		jsii.String(fmt.Sprintf("VpcPeering%d", i)),
		peeringConfig,
	)

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

// CreateBiDirectionalSubnetRoutes creates all main and subnet route table entries required for bi-directional routing between two VPCs in a peering relationship.
func CreateBiDirectionalSubnetRoutes(
	stack cdktf.TerraformStack,
	peer PeerConfig,
	core PeerCoreResources,
	peeringRes PeeringResources,
	name string,
	i int,
) {
	CreateRoute(
		stack,
		fmt.Sprintf("SourceToPeerMainRoute%d", i),
		core.SourceMainRt.Id(),
		core.PeerVpcData.CidrBlock(),
		peeringRes.Peering.Id(),
		core.SourceProvider,
		peeringRes.DependsOn,
	)

	CreateRoute(
		stack,
		fmt.Sprintf("PeerToPeerMainRoute%d", i),
		core.PeerMainRt.Id(),
		core.SourceVpcData.CidrBlock(),
		peeringRes.Peering.Id(),
		core.PeerProvider,
		peeringRes.DependsOn,
	)

	if peer.HasExtraPeerRouteTables {
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
