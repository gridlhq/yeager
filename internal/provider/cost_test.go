package provider

import (
	"testing"

	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/stretchr/testify/assert"
)

func TestCostPerHour(t *testing.T) {
	tests := []struct {
		name         string
		size         string
		expectedCost float64
	}{
		{"small", "small", 0.0168},
		{"medium", "medium", 0.0336},
		{"large", "large", 0.0672},
		{"xlarge", "xlarge", 0.1344},
		{"unknown returns zero", "unknown", 0.0},
		{"empty returns zero", "", 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost := CostPerHour(tt.size)
			assert.Equal(t, tt.expectedCost, cost, "expected cost for size %q to be $%.4f/hr", tt.size, tt.expectedCost)
		})
	}
}

func TestFormatCost(t *testing.T) {
	tests := []struct {
		name       string
		hourlyRate float64
		expected   string
	}{
		{"small", 0.0168, "~$0.017/hr"},
		{"medium", 0.0336, "~$0.034/hr"},
		{"large", 0.0672, "~$0.067/hr"},
		{"xlarge", 0.1344, "~$0.134/hr"},
		{"zero returns empty", 0.0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatCost(tt.hourlyRate)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInstanceSpecs(t *testing.T) {
	tests := []struct {
		name         string
		instanceType ec2types.InstanceType
		expectedVCPU string
		expectedMem  string
	}{
		{"t4g.small", ec2types.InstanceTypeT4gSmall, "2 vCPU", "2 GB"},
		{"t4g.medium", ec2types.InstanceTypeT4gMedium, "2 vCPU", "4 GB"},
		{"t4g.large", ec2types.InstanceTypeT4gLarge, "2 vCPU", "8 GB"},
		{"t4g.xlarge", ec2types.InstanceTypeT4gXlarge, "4 vCPU", "16 GB"},
		{"unknown returns empty", ec2types.InstanceType("t2.micro"), "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vcpu, mem := InstanceSpecs(tt.instanceType)
			assert.Equal(t, tt.expectedVCPU, vcpu, "expected vCPU for %s", tt.instanceType)
			assert.Equal(t, tt.expectedMem, mem, "expected memory for %s", tt.instanceType)
		})
	}
}

func TestCostPerHourMatchesInstanceSizeMap(t *testing.T) {
	// Verify that all instance types in instanceSizeMap have cost data.
	for sizeName, instanceType := range instanceSizeMap {
		cost, exists := costPerHour[instanceType]
		assert.True(t, exists, "instance type %s (%s) missing from costPerHour map", instanceType, sizeName)
		assert.Greater(t, cost, 0.0, "cost for %s should be > 0", sizeName)
	}
}

func TestInstanceSpecsMatchesInstanceSizeMap(t *testing.T) {
	// Verify that all instance types in instanceSizeMap have spec data.
	for sizeName, instanceType := range instanceSizeMap {
		vcpu, mem := InstanceSpecs(instanceType)
		assert.NotEmpty(t, vcpu, "instance type %s (%s) missing vCPU spec", instanceType, sizeName)
		assert.NotEmpty(t, mem, "instance type %s (%s) missing memory spec", instanceType, sizeName)
	}
}
