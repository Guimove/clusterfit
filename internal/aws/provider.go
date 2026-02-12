package aws

import (
	"context"
	"errors"
	"fmt"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/pricing"

	"github.com/guimove/clusterfit/internal/model"
)

const credentialCheckTimeout = 3 * time.Second

var (
	ErrAWSCredentials  = errors.New("AWS credentials not found; set AWS_PROFILE, run 'aws sso login', or configure ~/.aws/credentials")
	ErrNoInstanceTypes = errors.New("no instance types match the specified filters")
)

// PricingProvider abstracts the retrieval of EC2 instance types and pricing.
type PricingProvider interface {
	GetInstanceTypes(ctx context.Context, filter InstanceFilter) ([]model.NodeTemplate, error)
	GetSpotPrices(ctx context.Context, instanceTypes []string) (map[string]float64, error)
	Region() string
}

// InstanceFilter constrains which instance types to consider.
type InstanceFilter struct {
	Families              []string
	MinVCPUs              int32
	MaxVCPUs              int32
	Architectures         []model.Architecture
	CurrentGenerationOnly bool
	ExcludeBareMetal      bool
	ExcludeBurstable      bool
}

// ec2API is a minimal interface for the EC2 calls we need.
type ec2API interface {
	DescribeInstanceTypes(ctx context.Context, params *ec2.DescribeInstanceTypesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstanceTypesOutput, error)
	DescribeSpotPriceHistory(ctx context.Context, params *ec2.DescribeSpotPriceHistoryInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error)
}

// pricingAPI is a minimal interface for the Pricing API calls we need.
type pricingAPI interface {
	GetProducts(ctx context.Context, params *pricing.GetProductsInput, optFns ...func(*pricing.Options)) (*pricing.GetProductsOutput, error)
}

// AWSProvider implements PricingProvider using the AWS SDK.
type AWSProvider struct {
	ec2Client     ec2API
	pricingClient pricingAPI
	region        string
	cache         *FileCache
}

// NewAWSProvider creates a provider using the default AWS SDK config chain.
// IMDS (EC2 metadata) is disabled to avoid long timeouts when running locally.
// On EC2, use environment variables or instance profile via AWS_PROFILE.
func NewAWSProvider(ctx context.Context, region string, cacheDir string) (*AWSProvider, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithEC2IMDSClientEnableState(imds.ClientDisabled),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAWSCredentials, err)
	}

	// Verify credentials are available before making any API calls
	credCtx, cancel := context.WithTimeout(ctx, credentialCheckTimeout)
	defer cancel()
	if _, err := cfg.Credentials.Retrieve(credCtx); err != nil {
		return nil, ErrAWSCredentials
	}

	ec2Client := ec2.NewFromConfig(cfg)

	// Pricing API is only available in us-east-1
	pricingCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithEC2IMDSClientEnableState(imds.ClientDisabled),
	)
	if err != nil {
		return nil, fmt.Errorf("loading pricing config: %w", err)
	}
	pricingClient := pricing.NewFromConfig(pricingCfg)

	var cache *FileCache
	if cacheDir != "" {
		cache = NewFileCache(cacheDir)
	}

	return &AWSProvider{
		ec2Client:     ec2Client,
		pricingClient: pricingClient,
		region:        region,
		cache:         cache,
	}, nil
}

// Region returns the AWS region.
func (p *AWSProvider) Region() string {
	return p.region
}
