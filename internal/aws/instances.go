package aws

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/guimove/clusterfit/internal/model"
)

// GetInstanceTypes retrieves EC2 instance types matching the filter.
func (p *AWSProvider) GetInstanceTypes(ctx context.Context, filter InstanceFilter) ([]model.NodeTemplate, error) {
	var filters []ec2types.Filter

	if filter.CurrentGenerationOnly {
		filters = append(filters, ec2types.Filter{
			Name:   aws.String("current-generation"),
			Values: []string{"true"},
		})
	}

	if filter.ExcludeBareMetal {
		filters = append(filters, ec2types.Filter{
			Name:   aws.String("bare-metal"),
			Values: []string{"false"},
		})
	}

	if filter.ExcludeBurstable {
		filters = append(filters, ec2types.Filter{
			Name:   aws.String("burstable-performance-supported"),
			Values: []string{"false"},
		})
	}

	var allTypes []ec2types.InstanceTypeInfo
	var nextToken *string

	for {
		input := &ec2.DescribeInstanceTypesInput{
			Filters:   filters,
			NextToken: nextToken,
			MaxResults: aws.Int32(100),
		}

		output, err := p.ec2Client.DescribeInstanceTypes(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("describing instance types: %w", err)
		}

		allTypes = append(allTypes, output.InstanceTypes...)

		if output.NextToken == nil {
			break
		}
		nextToken = output.NextToken
	}

	// Apply client-side filters and convert to model
	var templates []model.NodeTemplate
	familySet := toSet(filter.Families)
	archSet := toArchSet(filter.Architectures)

	for _, it := range allTypes {
		tmpl := convertInstanceType(it, p.region)

		// Family filter
		if len(familySet) > 0 && !familySet[tmpl.InstanceFamily] {
			continue
		}

		// Architecture filter
		if len(archSet) > 0 && !archSet[tmpl.Architecture] {
			continue
		}

		// vCPU range filter
		if filter.MinVCPUs > 0 && tmpl.VCPUs < filter.MinVCPUs {
			continue
		}
		if filter.MaxVCPUs > 0 && tmpl.VCPUs > filter.MaxVCPUs {
			continue
		}

		templates = append(templates, tmpl)
	}

	if len(templates) == 0 {
		return nil, ErrNoInstanceTypes
	}

	return templates, nil
}

// convertInstanceType maps an EC2 InstanceTypeInfo to our NodeTemplate.
func convertInstanceType(it ec2types.InstanceTypeInfo, region string) model.NodeTemplate {
	tmpl := model.NodeTemplate{
		InstanceType: string(it.InstanceType),
		Region:       region,
		CapacityType: model.CapacityOnDemand,
	}

	// Parse instance family, generation, size from the type name
	tmpl.InstanceFamily, tmpl.Generation, tmpl.Size = parseInstanceType(string(it.InstanceType))

	// CPU
	if it.VCpuInfo != nil && it.VCpuInfo.DefaultVCpus != nil {
		tmpl.VCPUs = *it.VCpuInfo.DefaultVCpus
	}

	// Memory
	if it.MemoryInfo != nil && it.MemoryInfo.SizeInMiB != nil {
		tmpl.MemoryMiB = *it.MemoryInfo.SizeInMiB
	}

	// Networking
	if it.NetworkInfo != nil {
		if it.NetworkInfo.MaximumNetworkInterfaces != nil {
			tmpl.MaxENIs = *it.NetworkInfo.MaximumNetworkInterfaces
		}
		if it.NetworkInfo.Ipv4AddressesPerInterface != nil {
			tmpl.IPv4PerENI = *it.NetworkInfo.Ipv4AddressesPerInterface
		}
	}

	// Architecture
	if it.ProcessorInfo != nil {
		for _, arch := range it.ProcessorInfo.SupportedArchitectures {
			switch arch {
			case ec2types.ArchitectureTypeX8664:
				tmpl.Architecture = model.ArchAMD64
			case ec2types.ArchitectureTypeArm64:
				tmpl.Architecture = model.ArchARM64
			}
		}
	}

	// Current generation
	if it.CurrentGeneration != nil {
		tmpl.CurrentGeneration = *it.CurrentGeneration
	}

	// Compute max pods (standard mode: (MaxENIs * IPv4PerENI) - 1)
	tmpl.MaxPods = ComputeMaxPods(tmpl.MaxENIs, tmpl.IPv4PerENI)

	// Compute allocatable resources
	tmpl.AllocatableCPUMillis = computeAllocatableCPU(tmpl.VCPUs)
	tmpl.AllocatableMemoryBytes = computeAllocatableMemory(tmpl.MemoryMiB)

	return tmpl
}

// ComputeMaxPods calculates the maximum pods for an instance using the EKS standard formula.
func ComputeMaxPods(maxENIs, ipv4PerENI int32) int32 {
	if maxENIs == 0 || ipv4PerENI == 0 {
		return 110 // Kubernetes default
	}
	maxPods := (maxENIs * ipv4PerENI) - 1
	if maxPods > 250 {
		maxPods = 250
	}
	if maxPods < 1 {
		maxPods = 1
	}
	return maxPods
}

// computeAllocatableCPU applies the EKS kubelet CPU reservation formula.
// Reserve: 60m for first core, 10m for next, 5m for next 2, 2.5m for rest.
func computeAllocatableCPU(vcpus int32) int64 {
	totalMillis := int64(vcpus) * 1000

	var reserved int64
	remaining := int64(vcpus)

	if remaining > 0 {
		reserved += 60
		remaining--
	}
	if remaining > 0 {
		reserved += 10
		remaining--
	}
	if remaining > 0 {
		cores := remaining
		if cores > 2 {
			cores = 2
		}
		reserved += cores * 5
		remaining -= cores
	}
	if remaining > 0 {
		reserved += remaining * 2 // 2.5m rounded down per core
	}

	return totalMillis - reserved
}

// computeAllocatableMemory applies the EKS kubelet memory reservation formula.
// 255MiB base + 25% of first 4GiB + 20% of next 4GiB + 10% of next 8GiB + 6% of next 112GiB + 2% above.
func computeAllocatableMemory(memoryMiB int64) int64 {
	totalBytes := memoryMiB * 1024 * 1024

	reserved := int64(255 * 1024 * 1024) // 255 MiB base
	remainMiB := memoryMiB

	// 25% of first 4 GiB (4096 MiB)
	chunk := min64(remainMiB, 4096)
	reserved += chunk * 1024 * 1024 * 25 / 100
	remainMiB -= chunk

	// 20% of next 4 GiB
	chunk = min64(remainMiB, 4096)
	reserved += chunk * 1024 * 1024 * 20 / 100
	remainMiB -= chunk

	// 10% of next 8 GiB
	chunk = min64(remainMiB, 8192)
	reserved += chunk * 1024 * 1024 * 10 / 100
	remainMiB -= chunk

	// 6% of next 112 GiB
	chunk = min64(remainMiB, 112*1024)
	reserved += chunk * 1024 * 1024 * 6 / 100
	remainMiB -= chunk

	// 2% of rest
	if remainMiB > 0 {
		reserved += remainMiB * 1024 * 1024 * 2 / 100
	}

	allocatable := totalBytes - reserved
	if allocatable < 0 {
		allocatable = 0
	}
	return allocatable
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// parseInstanceType extracts family, generation, and size from an instance type name.
// e.g., "m5.xlarge" → ("m5", 5, "xlarge"), "m7g.large" → ("m7g", 7, "large")
var instanceTypeRegex = regexp.MustCompile(`^([a-z]+)(\d+)([a-z]*)\.(.+)$`)

func parseInstanceType(instanceType string) (family string, generation int, size string) {
	parts := strings.SplitN(instanceType, ".", 2)
	if len(parts) != 2 {
		return instanceType, 0, ""
	}

	family = parts[0]
	size = parts[1]

	matches := instanceTypeRegex.FindStringSubmatch(instanceType)
	if len(matches) >= 5 {
		gen, _ := strconv.Atoi(matches[2])
		generation = gen
	}

	return family, generation, size
}

func toSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
}

func toArchSet(archs []model.Architecture) map[model.Architecture]bool {
	s := make(map[model.Architecture]bool, len(archs))
	for _, a := range archs {
		s[a] = true
	}
	return s
}
