package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	pricingtypes "github.com/aws/aws-sdk-go-v2/service/pricing/types"

	"github.com/guimove/clusterfit/internal/model"
)

// regionNameMap maps AWS region codes to their display names used in the Pricing API.
var regionNameMap = map[string]string{
	"us-east-1":      "US East (N. Virginia)",
	"us-east-2":      "US East (Ohio)",
	"us-west-1":      "US West (N. California)",
	"us-west-2":      "US West (Oregon)",
	"eu-west-1":      "EU (Ireland)",
	"eu-west-2":      "EU (London)",
	"eu-west-3":      "EU (Paris)",
	"eu-central-1":   "EU (Frankfurt)",
	"eu-north-1":     "EU (Stockholm)",
	"ap-northeast-1": "Asia Pacific (Tokyo)",
	"ap-northeast-2": "Asia Pacific (Seoul)",
	"ap-southeast-1": "Asia Pacific (Singapore)",
	"ap-southeast-2": "Asia Pacific (Sydney)",
	"ap-south-1":     "Asia Pacific (Mumbai)",
	"sa-east-1":      "South America (Sao Paulo)",
	"ca-central-1":   "Canada (Central)",
}

// FetchOnDemandPricing retrieves on-demand prices for the given instance types.
func (p *AWSProvider) FetchOnDemandPricing(ctx context.Context, instanceTypes []string) (map[string]float64, error) {
	prices := make(map[string]float64)
	regionName := regionNameMap[p.region]
	if regionName == "" {
		regionName = p.region
	}

	// Process in batches of 10 (Pricing API allows up to 10 values per filter)
	for i := 0; i < len(instanceTypes); i += 10 {
		end := i + 10
		if end > len(instanceTypes) {
			end = len(instanceTypes)
		}
		batch := instanceTypes[i:end]

		input := &pricing.GetProductsInput{
			ServiceCode: awssdk.String("AmazonEC2"),
			Filters: []pricingtypes.Filter{
				{
					Type:  pricingtypes.FilterTypeTermMatch,
					Field: awssdk.String("operatingSystem"),
					Value: awssdk.String("Linux"),
				},
				{
					Type:  pricingtypes.FilterTypeTermMatch,
					Field: awssdk.String("tenancy"),
					Value: awssdk.String("Shared"),
				},
				{
					Type:  pricingtypes.FilterTypeTermMatch,
					Field: awssdk.String("capacitystatus"),
					Value: awssdk.String("Used"),
				},
				{
					Type:  pricingtypes.FilterTypeTermMatch,
					Field: awssdk.String("preInstalledSw"),
					Value: awssdk.String("NA"),
				},
				{
					Type:  pricingtypes.FilterTypeTermMatch,
					Field: awssdk.String("location"),
					Value: awssdk.String(regionName),
				},
			},
			MaxResults: awssdk.Int32(100),
		}

		output, err := p.pricingClient.GetProducts(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("fetching on-demand pricing: %w", err)
		}

		for _, priceStr := range output.PriceList {
			instanceType, pricePerHour := parsePricingProduct(priceStr, batch)
			if instanceType != "" && pricePerHour > 0 {
				prices[instanceType] = pricePerHour
			}
		}
	}

	return prices, nil
}

// parsePricingProduct extracts instance type and price from a Pricing API JSON response.
func parsePricingProduct(jsonStr string, wantTypes []string) (string, float64) {
	var product map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &product); err != nil {
		return "", 0
	}

	// Extract instance type from product attributes
	attrs, ok := product["product"].(map[string]interface{})
	if !ok {
		return "", 0
	}
	attributes, ok := attrs["attributes"].(map[string]interface{})
	if !ok {
		return "", 0
	}
	instanceType, _ := attributes["instanceType"].(string)
	if instanceType == "" {
		return "", 0
	}

	// Check if this is one of the types we want
	found := false
	for _, wt := range wantTypes {
		if wt == instanceType {
			found = true
			break
		}
	}
	if !found && len(wantTypes) > 0 {
		return "", 0
	}

	// Extract on-demand price
	terms, ok := product["terms"].(map[string]interface{})
	if !ok {
		return instanceType, 0
	}
	onDemand, ok := terms["OnDemand"].(map[string]interface{})
	if !ok {
		return instanceType, 0
	}

	for _, term := range onDemand {
		termMap, ok := term.(map[string]interface{})
		if !ok {
			continue
		}
		priceDimensions, ok := termMap["priceDimensions"].(map[string]interface{})
		if !ok {
			continue
		}
		for _, dim := range priceDimensions {
			dimMap, ok := dim.(map[string]interface{})
			if !ok {
				continue
			}
			pricePerUnit, ok := dimMap["pricePerUnit"].(map[string]interface{})
			if !ok {
				continue
			}
			usdStr, ok := pricePerUnit["USD"].(string)
			if !ok {
				continue
			}
			price, err := strconv.ParseFloat(usdStr, 64)
			if err != nil {
				continue
			}
			return instanceType, price
		}
	}

	return instanceType, 0
}

// GetSpotPrices retrieves current spot prices for the given instance types.
func (p *AWSProvider) GetSpotPrices(ctx context.Context, instanceTypes []string) (map[string]float64, error) {
	prices := make(map[string]float64)
	now := time.Now()

	// Process in batches of 100
	for i := 0; i < len(instanceTypes); i += 100 {
		end := i + 100
		if end > len(instanceTypes) {
			end = len(instanceTypes)
		}
		batch := instanceTypes[i:end]

		types := make([]ec2types.InstanceType, len(batch))
		for j, t := range batch {
			types[j] = ec2types.InstanceType(t)
		}

		input := &ec2.DescribeSpotPriceHistoryInput{
			InstanceTypes:       types,
			ProductDescriptions: []string{"Linux/UNIX"},
			StartTime:           &now,
		}

		output, err := p.ec2Client.DescribeSpotPriceHistory(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("fetching spot prices: %w", err)
		}

		for _, sp := range output.SpotPriceHistory {
			it := string(sp.InstanceType)
			price, _ := strconv.ParseFloat(awssdk.ToString(sp.SpotPrice), 64)
			// Keep the lowest price across AZs
			if existing, ok := prices[it]; !ok || price < existing {
				prices[it] = price
			}
		}
	}

	return prices, nil
}

// EnrichWithPricing adds on-demand and spot prices to a slice of NodeTemplates.
func (p *AWSProvider) EnrichWithPricing(ctx context.Context, templates []model.NodeTemplate) error {
	typeNames := make([]string, len(templates))
	for i, t := range templates {
		typeNames[i] = t.InstanceType
	}

	onDemand, err := p.FetchOnDemandPricing(ctx, typeNames)
	if err != nil {
		// Non-fatal: continue without on-demand pricing
		onDemand = make(map[string]float64)
	}

	spot, err := p.GetSpotPrices(ctx, typeNames)
	if err != nil {
		// Non-fatal: continue without spot pricing
		spot = make(map[string]float64)
	}

	for i := range templates {
		if price, ok := onDemand[templates[i].InstanceType]; ok {
			templates[i].OnDemandPricePerHour = price
		}
		if price, ok := spot[templates[i].InstanceType]; ok {
			templates[i].SpotPricePerHour = price
		}
	}

	return nil
}
