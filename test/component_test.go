package test

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/efs"
	"github.com/cloudposse/test-helpers/pkg/atmos"
	helper "github.com/cloudposse/test-helpers/pkg/atmos/aws-component-helper"
	"github.com/gruntwork-io/terratest/modules/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComponent(t *testing.T) {
	// Define the AWS region to use for the tests
	awsRegion := "us-east-2"

	// Initialize the test fixture
	fixture := helper.NewFixture(t, "../", awsRegion, "test/fixtures")

	// Ensure teardown is executed after the test
	defer fixture.TearDown()
	fixture.SetUp(&atmos.Options{})

	// Define the test suite
	fixture.Suite("default", func(t *testing.T, suite *helper.Suite) {
		suite.AddDependency("vpc", "default-test")

		// Setup phase: Create DNS zones for testing
		suite.Setup(t, func(t *testing.T, atm *helper.Atmos) {
			// Deploy the delegated DNS zone
			inputs := map[string]interface{}{
				"zone_config": []map[string]interface{}{
					{
						"subdomain": suite.GetRandomIdentifier(),
						"zone_name": "components.cptest.test-automation.app",
					},
				},
			}
			atm.GetAndDeploy("dns-delegated", "default-test", inputs)
		})

		// Teardown phase: Destroy the DNS zones created during setup
		suite.TearDown(t, func(t *testing.T, atm *helper.Atmos) {
			// Deploy the delegated DNS zone
			inputs := map[string]interface{}{
				"zone_config": []map[string]interface{}{
					{
						"subdomain": suite.GetRandomIdentifier(),
						"zone_name": "components.cptest.test-automation.app",
					},
				},
			}
			atm.GetAndDestroy("dns-delegated", "default-test", inputs)
		})

		// Test phase: Validate the functionality of the ALB component
		suite.Test(t, "basic", func(t *testing.T, atm *helper.Atmos) {
			defer atm.GetAndDestroy("efs/basic", "default-test", map[string]interface{}{})
			component := atm.GetAndDeploy("efs/basic", "default-test", map[string]interface{}{})
			assert.NotNil(t, component)

			arn := atm.Output(component, "efs_arn")
			assert.NotEmpty(t, arn)

			id := atm.Output(component, "efs_id")
			assert.True(t, strings.HasPrefix(id, "fs-"))

			dns_name := atm.Output(component, "efs_dns_name")
			assert.Equal(t, fmt.Sprintf("%s.efs.%s.amazonaws.com", id, awsRegion), dns_name)

			dnsDelegatedComponent := helper.NewAtmosComponent("dns-delegated", "default-test", nil)
			delegatedDomain := atm.Output(dnsDelegatedComponent, "default_domain_name")
			// delegatedZoneId := atm.Output(dnsDelegatedComponent, "default_domain_name")
			host := atm.Output(component, "efs_host")
			assert.Equal(t, fmt.Sprintf("ue2.test.default.%s", delegatedDomain), host)

			target_dns_names := atm.OutputList(component, "efs_mount_target_dns_names")
			assert.Equal(t, 2, len(target_dns_names))
			vpcComponent := helper.NewAtmosComponent("vpc", "default-test", nil)
			availability_zones := atm.OutputList(vpcComponent, "availability_zones")

			for _, az := range availability_zones {
				assert.Contains(t, target_dns_names, fmt.Sprintf("%s.%s.efs.%s.amazonaws.com", az, id, awsRegion))
			}

			target_ids := atm.OutputList(component, "efs_mount_target_ids")
			for _, target_id := range target_ids {
				assert.True(t, strings.HasPrefix(target_id, "fsmt-"))
			}

			target_ips := atm.OutputList(component, "efs_mount_target_ips")
			for _, target_ip := range target_ips {
				assert.NotNil(t, net.ParseIP(target_ip))
			}

			network_interface_ids := atm.OutputList(component, "efs_network_interface_ids")
			for _, network_interface_id := range network_interface_ids {
				assert.True(t, strings.HasPrefix(network_interface_id, "eni-"))
			}

			security_group_id := atm.Output(component, "security_group_id")
			assert.True(t, strings.HasPrefix(security_group_id, "sg-"))

			security_group_arn := atm.Output(component, "security_group_arn")
			assert.True(t, strings.HasSuffix(security_group_arn, security_group_id))

			security_group_name := atm.Output(component, "security_group_name")
			assert.NotEmpty(t, security_group_name)

			client := NewEFSClient(t, awsRegion)
			efsList, err := client.DescribeFileSystems(context.Background(), &efs.DescribeFileSystemsInput{
				FileSystemId: &id,
			})
			assert.NoError(t, err)

			efs := efsList.FileSystems[0]
			assert.Equal(t, id, *efs.FileSystemId)

			assert.EqualValues(t, "generalPurpose", efs.PerformanceMode)

			// Nil because we run EFS across multiple AZs
			assert.Nil(t, efs.AvailabilityZoneId)
			assert.Nil(t, efs.AvailabilityZoneName)

			assert.True(t, *efs.Encrypted)
			assert.Equal(t, arn, *efs.FileSystemArn)
			assert.Equal(t, id, *efs.FileSystemId)
			assert.EqualValues(t, "ENABLED", efs.FileSystemProtection.ReplicationOverwriteProtection)
			assert.EqualValues(t, "available", efs.LifeCycleState)
			assert.EqualValues(t, 2, efs.NumberOfMountTargets)

			assert.EqualValues(t, "bursting", efs.ThroughputMode)
			// Nil because we run EFS in bursting mode
			assert.Nil(t, efs.ProvisionedThroughputInMibps)
		})
	})
}

func NewEFSClient(t *testing.T, region string) *efs.Client {
	client, err := NewEFSClientE(t, region)
	require.NoError(t, err)

	return client
}

func NewEFSClientE(t *testing.T, region string) (*efs.Client, error) {
	sess, err := aws.NewAuthenticatedSession(region)
	if err != nil {
		return nil, err
	}
	return efs.NewFromConfig(*sess), nil
}
