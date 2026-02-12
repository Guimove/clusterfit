package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/guimove/clusterfit/internal/model"
)

const (
	// pricingAPIBase is the public EC2 pricing API (no auth required).
	pricingAPIBase = "https://go.runs-on.com/api/instances"

	// pricingHTTPTimeout is the timeout for each pricing HTTP request.
	pricingHTTPTimeout = 10 * time.Second
)

// instancePricing holds the resolved on-demand and spot prices for one instance type.
type instancePricing struct {
	OnDemandPrice float64
	SpotPrice     float64
}

// pricingAPIResult maps the runs-on API response fields we need.
type pricingAPIResult struct {
	InstanceType  string  `json:"instanceType"`
	OnDemandPrice float64 `json:"onDemandPrice"`
	SpotPrice     float64 `json:"spotPrice"`
}

type pricingAPIResponse struct {
	Results []pricingAPIResult `json:"results"`
}

// EnrichWithPricing adds on-demand and spot prices to a slice of NodeTemplates
// using the public runs-on.com pricing API (no AWS credentials required).
// Returns the number of templates that got on-demand pricing.
func (p *AWSProvider) EnrichWithPricing(ctx context.Context, templates []model.NodeTemplate) (int, error) {
	client := &http.Client{Timeout: pricingHTTPTimeout}
	priced := 0

	for i := range templates {
		pricing, err := fetchInstancePrice(ctx, client, templates[i].InstanceType, p.region)
		if err != nil {
			continue
		}
		if pricing.OnDemandPrice > 0 {
			templates[i].OnDemandPricePerHour = pricing.OnDemandPrice
			priced++
		}
		if pricing.SpotPrice > 0 {
			templates[i].SpotPricePerHour = pricing.SpotPrice
		}
	}

	return priced, nil
}

// fetchInstancePrice queries the public pricing API for a single instance type.
// Returns both on-demand and lowest spot price across AZs.
func fetchInstancePrice(ctx context.Context, client *http.Client, instanceType, region string) (*instancePricing, error) {
	url := fmt.Sprintf("%s/%s?region=%s&platform=Linux/UNIX", pricingAPIBase, instanceType, region)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pricing API returned %d for %s", resp.StatusCode, instanceType)
	}

	var pr pricingAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, err
	}

	if len(pr.Results) == 0 {
		return nil, fmt.Errorf("no pricing data for %s in %s", instanceType, region)
	}

	// On-demand is the same across AZs; for spot, pick the lowest
	result := &instancePricing{
		OnDemandPrice: pr.Results[0].OnDemandPrice,
		SpotPrice:     pr.Results[0].SpotPrice,
	}
	for _, r := range pr.Results[1:] {
		if r.SpotPrice > 0 && (result.SpotPrice == 0 || r.SpotPrice < result.SpotPrice) {
			result.SpotPrice = r.SpotPrice
		}
	}

	return result, nil
}
