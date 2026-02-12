package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	awspkg "github.com/guimove/clusterfit/internal/aws"
	"github.com/guimove/clusterfit/internal/model"
)

var pricingCmd = &cobra.Command{
	Use:   "pricing",
	Short: "List EC2 instance pricing",
	Long:  `Query and display EC2 instance type specifications and pricing.`,
	RunE:  runPricing,
}

func init() {
	f := pricingCmd.Flags()
	f.StringSlice("families", nil, "instance families to show")
	f.String("sort-by", "price", "sort by: price, vcpu, memory, type")
	f.Bool("include-spot", false, "include spot prices")
	f.StringSlice("architectures", nil, "filter by architecture (amd64, arm64)")

	rootCmd.AddCommand(pricingCmd)
}

func runPricing(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	provider, err := awspkg.NewAWSProvider(ctx, cfg.Cluster.Region, "")
	if err != nil {
		return err
	}

	families := cfg.Instances.Families
	if fam, _ := cmd.Flags().GetStringSlice("families"); len(fam) > 0 {
		families = fam
	}

	archs := make([]model.Architecture, len(cfg.Instances.Architectures))
	for i, a := range cfg.Instances.Architectures {
		archs[i] = model.Architecture(a)
	}
	if a, _ := cmd.Flags().GetStringSlice("architectures"); len(a) > 0 {
		archs = make([]model.Architecture, len(a))
		for i, v := range a {
			archs[i] = model.Architecture(v)
		}
	}

	filter := awspkg.InstanceFilter{
		Families:              families,
		Architectures:         archs,
		CurrentGenerationOnly: true,
		ExcludeBareMetal:      true,
		ExcludeBurstable:      true,
		MinVCPUs:              cfg.Instances.MinVCPUs,
		MaxVCPUs:              cfg.Instances.MaxVCPUs,
	}

	templates, err := provider.GetInstanceTypes(ctx, filter)
	if err != nil {
		return err
	}

	// Enrich with pricing
	if err := provider.EnrichWithPricing(ctx, templates); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not fetch all pricing data: %v\n", err)
	}

	// Sort
	sortBy, _ := cmd.Flags().GetString("sort-by")
	sortTemplates(templates, sortBy)

	// Display
	includeSpot, _ := cmd.Flags().GetBool("include-spot")

	header := "%-20s %5s %8s %7s %6s %10s"
	args2 := []interface{}{"INSTANCE TYPE", "vCPU", "MEM(GiB)", "MAXPOD", "ARCH", "$/HOUR(OD)"}
	if includeSpot {
		header += " %10s"
		args2 = append(args2, "$/HOUR(SP)")
	}
	header += "\n"
	fmt.Fprintf(os.Stdout, header, args2...)
	fmt.Fprintf(os.Stdout, "%s\n", strings.Repeat("-", 80))

	for _, t := range templates {
		row := fmt.Sprintf("%-20s %5d %8.1f %7d %6s %10.4f",
			t.InstanceType,
			t.VCPUs,
			float64(t.MemoryMiB)/1024.0,
			t.MaxPods,
			string(t.Architecture),
			t.OnDemandPricePerHour,
		)
		if includeSpot {
			if t.SpotPricePerHour > 0 {
				row += fmt.Sprintf(" %10.4f", t.SpotPricePerHour)
			} else {
				row += fmt.Sprintf(" %10s", "N/A")
			}
		}
		fmt.Println(row)
	}

	fmt.Fprintf(os.Stdout, "\n%d instance types in %s\n", len(templates), cfg.Cluster.Region)
	return nil
}

func sortTemplates(templates []model.NodeTemplate, by string) {
	switch by {
	case "vcpu":
		sort.Slice(templates, func(i, j int) bool {
			return templates[i].VCPUs < templates[j].VCPUs
		})
	case "memory":
		sort.Slice(templates, func(i, j int) bool {
			return templates[i].MemoryMiB < templates[j].MemoryMiB
		})
	case "type":
		sort.Slice(templates, func(i, j int) bool {
			return templates[i].InstanceType < templates[j].InstanceType
		})
	default: // price
		sort.Slice(templates, func(i, j int) bool {
			return templates[i].OnDemandPricePerHour < templates[j].OnDemandPricePerHour
		})
	}
}
