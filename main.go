package main

import (
	"fmt"
	"os"
	"strings"

	cmdk8s "k8sx/cmd"

	"github.com/spf13/cobra"
)

var (
	kubeconfigPath string
	namespaces     []string
	contextName    string
)

var rootCmd = &cobra.Command{
	Use:   "k8sx [query]",
	Short: "Kubernetes resource search tool",
	Long: `A tool to search Kubernetes resources by IP or name.
Supports searching pods, services, and their relationships.

If you provide a query without a subcommand, it will automatically search:
- By IP if the query is a valid IP address
- By name if the query is not an IP`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// If no args, show help
		if len(args) == 0 {
			return cmd.Help()
		}

		query := args[0]

		// Auto-detect if it's an IP or name
		if cmdk8s.ValidateIP(query) {
			// It's an IP address
			fmt.Println("Detected IP address, searching by IP...")
			return cmdk8s.SearchK8sByIPAllContexts(kubeconfigPath, query, namespaces)
		} else {
			// It's a name
			fmt.Println("Detected name pattern, searching by name...")
			return cmdk8s.SearchK8sByNameAllContexts(kubeconfigPath, query, namespaces)
		}
	},
}

var listContextsCmd = &cobra.Command{
	Use:   "ctx",
	Short: "List all contexts from kubeconfig",
	Long:  "List all available contexts from the specified kubeconfig file",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmdk8s.ListK8sContexts(kubeconfigPath)
	},
}

var listNamespacesCmd = &cobra.Command{
	Use:   "ns",
	Short: "List all namespaces you have permission to access",
	Long: `List all namespaces from the current (or specified) context.
Shows which namespaces you have permission to list pods in.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmdk8s.ListK8sNamespaces(kubeconfigPath, contextName)
	},
}

var searchCmd = &cobra.Command{
	Use:   "s [query]",
	Short: "Search for Kubernetes resources (auto-detects IP or name)",
	Long: `Search for Kubernetes resources by IP address or name across ALL contexts and ALL namespaces.

The search automatically detects whether your query is an IP address or a name:
- If it's a valid IP (IPv4/IPv6): searches for pods and services by IP
- Otherwise: searches for pods by name (partial match)

This is a comprehensive search that will:
- Search in every context from kubeconfig
- Search in every namespace in each context (or only specified namespaces with --namespaces flag)
- Return all matching pods and services

Note: This may take a while as it searches everywhere.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := args[0]

		// Auto-detect if it's an IP or name
		if cmdk8s.ValidateIP(query) {
			// It's an IP address
			fmt.Println("Detected IP address, searching by IP...")
			return cmdk8s.SearchK8sByIPAllContexts(kubeconfigPath, query, namespaces)
		} else {
			// It's a name
			fmt.Println("Detected name pattern, searching by name...")
			return cmdk8s.SearchK8sByNameAllContexts(kubeconfigPath, query, namespaces)
		}
	},
}

func init() {
	// Get default kubeconfig path from environment or default location
	defaultKubeconfig := os.Getenv("KUBECONFIG")
	if defaultKubeconfig == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			// Fallback to /root/.kube/config on Unix-like systems
			// This is a reasonable default for containerized environments
			defaultKubeconfig = "/root/.kube/config"
		} else {
			defaultKubeconfig = homeDir + "/.kube/config"
		}
	}

	// Get default namespaces from environment or use empty (auto-discover)
	var defaultNamespaces []string
	if envNamespaces := os.Getenv("K8S_SEARCH_NAMESPACES"); envNamespaces != "" {
		defaultNamespaces = strings.Split(envNamespaces, ",")
		// Trim whitespace from each namespace
		for i := range defaultNamespaces {
			defaultNamespaces[i] = strings.TrimSpace(defaultNamespaces[i])
		}
	}

	// Get default context from environment
	defaultContext := os.Getenv("K8S_SEARCH_CONTEXT")

	// Persistent flags for all commands
	rootCmd.PersistentFlags().StringVar(&kubeconfigPath, "kubeconfig", defaultKubeconfig, "Path to kubeconfig file (env: KUBECONFIG)")
	rootCmd.PersistentFlags().StringSliceVar(&namespaces, "namespaces", defaultNamespaces, "Namespaces to search (comma-separated, empty = auto-discover accessible namespaces) (env: K8S_SEARCH_NAMESPACES)")
	rootCmd.PersistentFlags().StringVar(&contextName, "context", defaultContext, "Context to use (empty = current context) (env: K8S_SEARCH_CONTEXT)")

	// Add subcommands
	rootCmd.AddCommand(listContextsCmd)
	rootCmd.AddCommand(listNamespacesCmd)
	rootCmd.AddCommand(searchCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
