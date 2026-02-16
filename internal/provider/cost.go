package provider

import (
	"fmt"

	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// costPerHour maps EC2 instance types to their approximate hourly cost in USD.
// Prices are for on-demand Linux instances in us-east-1 as of 2025.
// Source: https://aws.amazon.com/ec2/pricing/on-demand/
//
// These are approximate and vary by region. Actual costs may differ.
var costPerHour = map[ec2types.InstanceType]float64{
	ec2types.InstanceTypeT4gSmall:  0.0168, // 2 vCPU, 2 GB
	ec2types.InstanceTypeT4gMedium: 0.0336, // 2 vCPU, 4 GB
	ec2types.InstanceTypeT4gLarge:  0.0672, // 2 vCPU, 8 GB
	ec2types.InstanceTypeT4gXlarge: 0.1344, // 4 vCPU, 16 GB
}

// CostPerHour returns the approximate hourly cost in USD for a yeager VM size.
// Returns 0.0 if the size is unknown or if pricing data is unavailable.
func CostPerHour(size string) float64 {
	instanceType, err := InstanceTypeForSize(size)
	if err != nil {
		return 0.0
	}
	return costPerHour[instanceType]
}

// FormatCost returns a human-readable cost string like "~$0.034/hr".
// If the cost is zero, returns an empty string.
func FormatCost(hourlyRate float64) string {
	if hourlyRate == 0.0 {
		return ""
	}
	return fmt.Sprintf("~$%.3f/hr", hourlyRate)
}

// InstanceSpecs returns human-readable specs for an instance type.
// Returns empty strings if the instance type is not recognized.
func InstanceSpecs(instanceType ec2types.InstanceType) (vcpu, memory string) {
	switch instanceType {
	case ec2types.InstanceTypeT4gSmall:
		return "2 vCPU", "2 GB"
	case ec2types.InstanceTypeT4gMedium:
		return "2 vCPU", "4 GB"
	case ec2types.InstanceTypeT4gLarge:
		return "2 vCPU", "8 GB"
	case ec2types.InstanceTypeT4gXlarge:
		return "4 vCPU", "16 GB"
	default:
		return "", ""
	}
}
