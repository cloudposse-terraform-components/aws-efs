package test

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/efs"
	"github.com/cloudposse/test-helpers/pkg/atmos"
	helper "github.com/cloudposse/test-helpers/pkg/atmos/component-helper"
	awshelper "github.com/cloudposse/test-helpers/pkg/aws"
	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/stretchr/testify/assert"
)

type ComponentSuite struct {
	helper.TestSuite
}

func (s *ComponentSuite) TestBasic() {
	const component = "efs/basic"
	const stack = "default-test"
	const awsRegion = "us-east-2"

	hostnamePrefix := strings.ToLower(random.UniqueId())
	inputs := map[string]interface{}{
		"hostname_template": hostnamePrefix + "-%[3]v.%[2]v.%[1]v",
	}

	defer s.DestroyAtmosComponent(s.T(), component, stack, &inputs)
	options, _ := s.DeployAtmosComponent(s.T(), component, stack, &inputs)
	assert.NotNil(s.T(), options)

	arn := atmos.Output(s.T(), options, "efs_arn")
	assert.NotEmpty(s.T(), arn)

	id := atmos.Output(s.T(), options, "efs_id")
	assert.True(s.T(), strings.HasPrefix(id, "fs-"))

	dns_name := atmos.Output(s.T(), options, "efs_dns_name")
	assert.Equal(s.T(), fmt.Sprintf("%s.efs.%s.amazonaws.com", id, awsRegion), dns_name)

	dnsDelegatedOptions := s.GetAtmosOptions("dns-delegated", "default-test", nil)
	delegatedDomain := atmos.Output(s.T(), dnsDelegatedOptions, "default_domain_name")
	host := atmos.Output(s.T(), options, "efs_host")
	assert.Equal(s.T(), fmt.Sprintf("%s-ue2.test.default.%s", hostnamePrefix, delegatedDomain), host)

	target_dns_names := atmos.OutputList(s.T(), options, "efs_mount_target_dns_names")
	assert.Equal(s.T(), 2, len(target_dns_names))
	vpcOptions := s.GetAtmosOptions("vpc", "default-test", nil)
	availability_zones := atmos.OutputList(s.T(), vpcOptions, "availability_zones")

	for _, az := range availability_zones {
		assert.Contains(s.T(), target_dns_names, fmt.Sprintf("%s.%s.efs.%s.amazonaws.com", az, id, awsRegion))
	}

	target_ids := atmos.OutputList(s.T(), options, "efs_mount_target_ids")
	for _, target_id := range target_ids {
		assert.True(s.T(), strings.HasPrefix(target_id, "fsmt-"))
	}

	target_ips := atmos.OutputList(s.T(), options, "efs_mount_target_ips")
	for _, target_ip := range target_ips {
		assert.NotNil(s.T(), net.ParseIP(target_ip))
	}

	network_interface_ids := atmos.OutputList(s.T(), options, "efs_network_interface_ids")
	for _, network_interface_id := range network_interface_ids {
		assert.True(s.T(), strings.HasPrefix(network_interface_id, "eni-"))
	}

	security_group_id := atmos.Output(s.T(), options, "security_group_id")
	assert.True(s.T(), strings.HasPrefix(security_group_id, "sg-"))

	security_group_arn := atmos.Output(s.T(), options, "security_group_arn")
	assert.True(s.T(), strings.HasSuffix(security_group_arn, security_group_id))

	security_group_name := atmos.Output(s.T(), options, "security_group_name")
	assert.NotEmpty(s.T(), security_group_name)

	client := awshelper.NewEFSClient(s.T(), awsRegion)
	efsList, err := client.DescribeFileSystems(context.Background(), &efs.DescribeFileSystemsInput{
		FileSystemId: &id,
	})
	assert.NoError(s.T(), err)

	efs := efsList.FileSystems[0]
	assert.Equal(s.T(), id, *efs.FileSystemId)

	assert.EqualValues(s.T(), "generalPurpose", efs.PerformanceMode)

	assert.Nil(s.T(), efs.AvailabilityZoneId)
	assert.Nil(s.T(), efs.AvailabilityZoneName)

	assert.True(s.T(), *efs.Encrypted)
	assert.Equal(s.T(), arn, *efs.FileSystemArn)
	assert.Equal(s.T(), id, *efs.FileSystemId)
	assert.EqualValues(s.T(), "ENABLED", efs.FileSystemProtection.ReplicationOverwriteProtection)
	assert.EqualValues(s.T(), "available", efs.LifeCycleState)
	assert.EqualValues(s.T(), 2, efs.NumberOfMountTargets)

	assert.EqualValues(s.T(), "bursting", efs.ThroughputMode)
	assert.Nil(s.T(), efs.ProvisionedThroughputInMibps)

	s.DriftTest(component, stack, &inputs)
}

func (s *ComponentSuite) TestEnabledFlag() {
	const component = "efs/disabled"
	const stack = "default-test"
	const awsRegion = "us-east-2"

	s.VerifyEnabledFlag(component, stack, nil)
}


func TestRunSuite(t *testing.T) {
	suite := new(ComponentSuite)

	suite.AddDependency(t, "vpc", "default-test", nil)

	subdomain := strings.ToLower(random.UniqueId())
	inputs := map[string]interface{}{
		"zone_config": []map[string]interface{}{
			{
				"subdomain": subdomain,
				"zone_name": "components.cptest.test-automation.app",
			},
		},
	}
	suite.AddDependency(t, "dns-delegated", "default-test", &inputs)
	helper.Run(t, suite)
}
