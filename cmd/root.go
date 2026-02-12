package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/guimove/clusterfit/internal/config"
)

var (
	cfgFile string
	cfg     config.Config
	verbose bool
)

var rootCmd = &cobra.Command{
	Use:   "clusterfit",
	Short: "EC2 instance sizing recommender for EKS clusters",
	Long: `ClusterFit analyzes historical pod metrics from Prometheus and recommends
optimal EC2 instance families and sizes for your EKS cluster.

It runs bin-packing simulations across instance types and ranks them by
cost efficiency, resource utilization, and fragmentation.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return loadConfig()
	},
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: clusterfit.yaml)")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "enable verbose output")

	// Global flags that map to config
	rootCmd.PersistentFlags().String("region", "", "AWS region")
	rootCmd.PersistentFlags().String("prometheus-url", "", "Prometheus/Thanos endpoint URL")
	rootCmd.PersistentFlags().String("kubeconfig", "", "path to kubeconfig file")
	rootCmd.PersistentFlags().String("kube-context", "", "Kubernetes context name")
	rootCmd.PersistentFlags().BoolP("discover", "d", false, "auto-discover Prometheus endpoint from Kubernetes")
	rootCmd.PersistentFlags().String("discovery-namespace", "", "limit service discovery to a namespace")

	_ = viper.BindPFlag("cluster.region", rootCmd.PersistentFlags().Lookup("region"))
	_ = viper.BindPFlag("prometheus.url", rootCmd.PersistentFlags().Lookup("prometheus-url"))
	_ = viper.BindPFlag("kubernetes.kubeconfig", rootCmd.PersistentFlags().Lookup("kubeconfig"))
	_ = viper.BindPFlag("kubernetes.context", rootCmd.PersistentFlags().Lookup("kube-context"))
	_ = viper.BindPFlag("kubernetes.enabled", rootCmd.PersistentFlags().Lookup("discover"))
	_ = viper.BindPFlag("kubernetes.discovery_namespace", rootCmd.PersistentFlags().Lookup("discovery-namespace"))
}

func loadConfig() error {
	// Start with defaults
	cfg = config.Default()

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("clusterfit")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
		viper.AddConfigPath("$HOME/.clusterfit")
	}

	// Environment variable overrides
	viper.SetEnvPrefix("CLUSTERFIT")
	viper.AutomaticEnv()

	// Read config file (not an error if missing)
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok && cfgFile != "" {
			return fmt.Errorf("reading config file: %w", err)
		}
	}

	// Unmarshal into config struct
	if err := viper.Unmarshal(&cfg); err != nil {
		return fmt.Errorf("parsing config: %w", err)
	}

	return cfg.Validate()
}
