package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	k8s "k8sx/pkg"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// K8sSearchConfig represents the configuration for K8s search
type K8sSearchConfig struct {
	KubeconfigPath string
	Namespaces     []string
	ContextName    string
}

// ValidateIP is a wrapper for k8s.ValidateIP for use in CLI
func ValidateIP(ip string) bool {
	return k8s.ValidateIP(ip)
}

// formatTargetPort properly formats a target port, handling both integer and string (named) ports
func formatTargetPort(targetPort intstr.IntOrString) string {
	if targetPort.Type == intstr.String {
		return targetPort.StrVal
	}
	return fmt.Sprintf("%d", targetPort.IntVal)
}

// ListK8sContexts lists all contexts in kubeconfig
func ListK8sContexts(kubeconfigPath string) error {
	config, err := k8s.LoadKubeConfig(kubeconfigPath)
	if err != nil {
		fmt.Println(text.FgRed.Sprintf("Failed to load kubeconfig: %v", err))
		return err
	}

	contexts := k8s.GetContexts(config)
	if len(contexts) == 0 {
		fmt.Println(text.FgYellow.Sprintf("No contexts found in kubeconfig"))
		return nil
	}

	tablex := table.Table{}
	tablex.SetStyle(table.StyleLight)
	tablex.AppendRow(table.Row{"Context Name", "Current"})

	for _, contextName := range contexts {
		isCurrent := ""
		if contextName == config.CurrentContext {
			isCurrent = "*"
		}
		tablex.AppendRow(table.Row{contextName, isCurrent})
	}

	fmt.Println(tablex.Render())
	return nil
}

// SearchK8sByIP searches Kubernetes resources by IP address
func SearchK8sByIP(config K8sSearchConfig, ip string) error {
	// Validate IP
	if !k8s.ValidateIP(ip) {
		fmt.Println(text.FgRed.Sprintf("Invalid IP address: %s", ip))
		return fmt.Errorf("invalid IP address: %s", ip)
	}

	// Create K8s client
	client, err := k8s.NewK8sClient(config.KubeconfigPath, config.ContextName, config.Namespaces)
	if err != nil {
		fmt.Println(text.FgRed.Sprintf("Failed to create K8s client: %v", err))
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Search by IP
	pods, services, err := client.SearchByIP(ctx, ip)
	if err != nil {
		fmt.Println(text.FgRed.Sprintf("Failed to search by IP: %v", err))
		return err
	}

	// Display results
	if len(pods) == 0 && len(services) == 0 {
		fmt.Println(text.FgYellow.Sprintf("No resources found for IP: %s", ip))
		return nil
	}

	// Display pods
	if len(pods) > 0 {
		fmt.Println(text.FgGreen.Sprintf("\n=== Pods matching IP: %s ===", ip))
		podTable := table.Table{}
		podTable.SetStyle(table.StyleLight)
		podTable.AppendRow(table.Row{"Namespace", "Pod Name", "Pod IP", "Host IP", "Owner Kind", "Owner Name"})

		for _, pod := range pods {
			ownerInfo := fmt.Sprintf("%s", pod.OwnerName)
			if pod.OwnerKind == "ReplicaSet" {
				// Try to get deployment name
				deploymentName, err := client.GetDeploymentByReplicaSet(ctx, pod.Namespace, pod.OwnerName)
				if err == nil {
					ownerInfo = fmt.Sprintf("%s (Deployment: %s)", pod.OwnerName, deploymentName)
				}
			}

			podTable.AppendRow(table.Row{
				pod.Namespace,
				pod.Name,
				pod.PodIP,
				pod.HostIP,
				pod.OwnerKind,
				ownerInfo,
			})
		}
		fmt.Println(podTable.Render())
	}

	// Display services
	if len(services) > 0 {
		fmt.Println(text.FgGreen.Sprintf("\n=== Services matching IP: %s ===", ip))
		svcTable := table.Table{}
		svcTable.SetStyle(table.StyleLight)
		svcTable.AppendRow(table.Row{"Namespace", "Service Name", "Type", "Cluster IP", "External IPs", "Ports", "Selector"})

		for _, svc := range services {
			ports := []string{}
			for _, port := range svc.Ports {
				ports = append(ports, fmt.Sprintf("%d:%s/%s", port.Port, formatTargetPort(port.TargetPort), port.Protocol))
			}

			selector := []string{}
			for k, v := range svc.Selector {
				selector = append(selector, fmt.Sprintf("%s=%s", k, v))
			}

			svcTable.AppendRow(table.Row{
				svc.Namespace,
				svc.Name,
				svc.Type,
				svc.ClusterIP,
				strings.Join(svc.ExternalIPs, ", "),
				strings.Join(ports, ", "),
				strings.Join(selector, ", "),
			})
		}
		fmt.Println(svcTable.Render())
	}

	return nil
}

// SearchK8sByName searches Kubernetes pods by name
func SearchK8sByName(config K8sSearchConfig, name string) error {
	if name == "" {
		fmt.Println(text.FgRed.Sprintf("Name cannot be empty"))
		return fmt.Errorf("name cannot be empty")
	}

	// Create K8s client
	client, err := k8s.NewK8sClient(config.KubeconfigPath, config.ContextName, config.Namespaces)
	if err != nil {
		fmt.Println(text.FgRed.Sprintf("Failed to create K8s client: %v", err))
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Search by name
	pods, err := client.SearchByName(ctx, name)
	if err != nil {
		fmt.Println(text.FgRed.Sprintf("Failed to search by name: %v", err))
		return err
	}

	// Display results
	if len(pods) == 0 {
		fmt.Println(text.FgYellow.Sprintf("No pods found with name containing: %s", name))
		return nil
	}

	fmt.Println(text.FgGreen.Sprintf("\n=== Pods matching name: %s ===", name))
	podTable := table.Table{}
	podTable.SetStyle(table.StyleLight)
	podTable.AppendRow(table.Row{"Namespace", "Pod Name", "Pod IP", "Host IP", "Owner Kind", "Owner Name"})

	for _, pod := range pods {
		ownerInfo := fmt.Sprintf("%s", pod.OwnerName)
		if pod.OwnerKind == "ReplicaSet" {
			// Try to get deployment name
			deploymentName, err := client.GetDeploymentByReplicaSet(ctx, pod.Namespace, pod.OwnerName)
			if err == nil {
				ownerInfo = fmt.Sprintf("%s (Deployment: %s)", pod.OwnerName, deploymentName)
			}
		}

		podTable.AppendRow(table.Row{
			pod.Namespace,
			pod.Name,
			pod.PodIP,
			pod.HostIP,
			pod.OwnerKind,
			ownerInfo,
		})
	}
	fmt.Println(podTable.Render())

	return nil
}

// SearchK8sByIPAllContexts searches Kubernetes resources by IP across all contexts and all (or specified) namespaces
func SearchK8sByIPAllContexts(kubeconfigPath string, ip string, namespaces []string) error {
	// Validate IP
	if !k8s.ValidateIP(ip) {
		fmt.Println(text.FgRed.Sprintf("Failed to search: IP address is invalid: %s", ip))
		return fmt.Errorf("invalid IP address: %s", ip)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// If no namespaces specified, try to get accessible namespaces automatically
	if len(namespaces) == 0 {
		fmt.Println(text.FgYellow.Sprintf("No namespaces specified, attempting to discover accessible namespaces..."))
		accessible, err := GetAccessibleNamespaces(kubeconfigPath, "")
		if err == nil && len(accessible) > 0 {
			namespaces = accessible
			fmt.Println(text.FgCyan.Sprintf("Found %d accessible namespace(s): %s\n", len(namespaces), strings.Join(namespaces, ", ")))
		} else {
			fmt.Println(text.FgYellow.Sprintf("Could not discover accessible namespaces, will try all namespaces...\n"))
		}
	}

	if len(namespaces) > 0 {
		fmt.Println(text.FgCyan.Sprintf("Searching in specified namespaces for IP: %s", ip))
		fmt.Println(text.FgYellow.Sprintf("Namespaces: %s\n", strings.Join(namespaces, ", ")))
	} else {
		fmt.Println(text.FgCyan.Sprintf("Searching across all contexts and namespaces for IP: %s", ip))
		fmt.Println(text.FgYellow.Sprintf("This may take a while...\n"))
	}

	// Search across all contexts and namespaces
	results, err := k8s.SearchByIPAllContexts(ctx, kubeconfigPath, ip, namespaces)
	if err != nil {
		fmt.Println(text.FgRed.Sprintf("Failed to search: %v", err))
		return err
	}

	// Display results
	if len(results) == 0 {
		fmt.Println(text.FgYellow.Sprintf("No resources found for IP: %s across all contexts and namespaces", ip))
		return nil
	}

	totalPods := 0
	totalServices := 0

	for _, result := range results {
		totalPods += len(result.Pods)
		totalServices += len(result.Services)

		// Display pods
		if len(result.Pods) > 0 {
			fmt.Println(text.FgGreen.Sprintf("\n=== Pods in Context: %s, Namespace: %s ===", result.Context, result.Namespace))
			podTable := table.Table{}
			podTable.SetStyle(table.StyleLight)
			podTable.AppendRow(table.Row{"Pod Name", "Pod IP", "Host IP", "Owner Kind", "Owner Name"})

			for _, pod := range result.Pods {
				ownerInfo := pod.OwnerName
				if pod.OwnerKind == "ReplicaSet" {
					// Try to get deployment name
					client, err := k8s.NewK8sClient(kubeconfigPath, result.Context, []string{result.Namespace})
					if err == nil {
						deploymentName, err := client.GetDeploymentByReplicaSet(ctx, pod.Namespace, pod.OwnerName)
						if err == nil {
							ownerInfo = fmt.Sprintf("%s (Deployment: %s)", pod.OwnerName, deploymentName)
						}
					}
				}

				podTable.AppendRow(table.Row{
					pod.Name,
					pod.PodIP,
					pod.HostIP,
					pod.OwnerKind,
					ownerInfo,
				})
			}
			fmt.Println(podTable.Render())
		}

		// Display services
		if len(result.Services) > 0 {
			fmt.Println(text.FgGreen.Sprintf("\n=== Services in Context: %s, Namespace: %s ===", result.Context, result.Namespace))
			svcTable := table.Table{}
			svcTable.SetStyle(table.StyleLight)
			svcTable.AppendRow(table.Row{"Service Name", "Type", "Cluster IP", "External IPs", "Ports", "Selector"})

			for _, svc := range result.Services {
				ports := []string{}
				for _, port := range svc.Ports {
					ports = append(ports, fmt.Sprintf("%d:%s/%s", port.Port, formatTargetPort(port.TargetPort), port.Protocol))
				}

				selector := []string{}
				for k, v := range svc.Selector {
					selector = append(selector, fmt.Sprintf("%s=%s", k, v))
				}

				svcTable.AppendRow(table.Row{
					svc.Name,
					svc.Type,
					svc.ClusterIP,
					strings.Join(svc.ExternalIPs, ", "),
					strings.Join(ports, ", "),
					strings.Join(selector, ", "),
				})
			}
			fmt.Println(svcTable.Render())
		}
	}

	fmt.Println(text.FgGreen.Sprintf("\n=== Summary ==="))
	fmt.Printf("Total contexts searched: %d\n", len(results))
	fmt.Printf("Total pods found: %d\n", totalPods)
	fmt.Printf("Total services found: %d\n", totalServices)

	return nil
}

// SearchK8sByNameAllContexts searches Kubernetes pods by name across all contexts and all (or specified) namespaces
func SearchK8sByNameAllContexts(kubeconfigPath string, name string, namespaces []string) error {
	if name == "" {
		fmt.Println(text.FgRed.Sprintf("Name cannot be empty"))
		return fmt.Errorf("name cannot be empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// If no namespaces specified, try to get accessible namespaces automatically
	if len(namespaces) == 0 {
		fmt.Println(text.FgYellow.Sprintf("No namespaces specified, attempting to discover accessible namespaces..."))
		accessible, err := GetAccessibleNamespaces(kubeconfigPath, "")
		if err == nil && len(accessible) > 0 {
			namespaces = accessible
			fmt.Println(text.FgCyan.Sprintf("Found %d accessible namespace(s): %s\n", len(namespaces), strings.Join(namespaces, ", ")))
		} else {
			fmt.Println(text.FgYellow.Sprintf("Could not discover accessible namespaces, will try all namespaces...\n"))
		}
	}

	if len(namespaces) > 0 {
		fmt.Println(text.FgCyan.Sprintf("Searching in specified namespaces for name: %s", name))
		fmt.Println(text.FgYellow.Sprintf("Namespaces: %s\n", strings.Join(namespaces, ", ")))
	} else {
		fmt.Println(text.FgCyan.Sprintf("Searching across all contexts and namespaces for name: %s", name))
		fmt.Println(text.FgYellow.Sprintf("This may take a while...\n"))
	}

	// Search across all contexts and namespaces
	results, err := k8s.SearchByNameAllContexts(ctx, kubeconfigPath, name, namespaces)
	if err != nil {
		fmt.Println(text.FgRed.Sprintf("Failed to search: %v", err))
		return err
	}

	// Display results
	if len(results) == 0 {
		fmt.Println(text.FgYellow.Sprintf("No pods found with name containing: %s across all contexts and namespaces", name))
		return nil
	}

	totalPods := 0

	for _, result := range results {
		totalPods += len(result.Pods)

		fmt.Println(text.FgGreen.Sprintf("\n=== Pods in Context: %s, Namespace: %s ===", result.Context, result.Namespace))
		podTable := table.Table{}
		podTable.SetStyle(table.StyleLight)
		podTable.AppendRow(table.Row{"Pod Name", "Pod IP", "Host IP", "Owner Kind", "Owner Name"})

		for _, pod := range result.Pods {
			ownerInfo := fmt.Sprintf("%s", pod.OwnerName)
			if pod.OwnerKind == "ReplicaSet" {
				// Try to get deployment name
				client, err := k8s.NewK8sClient(kubeconfigPath, result.Context, []string{result.Namespace})
				if err == nil {
					deploymentName, err := client.GetDeploymentByReplicaSet(ctx, pod.Namespace, pod.OwnerName)
					if err == nil {
						ownerInfo = fmt.Sprintf("%s (Deployment: %s)", pod.OwnerName, deploymentName)
					}
				}
			}

			podTable.AppendRow(table.Row{
				pod.Name,
				pod.PodIP,
				pod.HostIP,
				pod.OwnerKind,
				ownerInfo,
			})
		}
		fmt.Println(podTable.Render())
	}

	fmt.Println(text.FgGreen.Sprintf("\n=== Summary ==="))
	fmt.Printf("Total contexts searched: %d\n", len(results))
	fmt.Printf("Total pods found: %d\n", totalPods)

	return nil
}

// ListK8sNamespaces lists all namespaces and shows which ones you have permission to access
func ListK8sNamespaces(kubeconfigPath string, contextName string) error {
	// Create K8s client
	client, err := k8s.NewK8sClient(kubeconfigPath, contextName, []string{})
	if err != nil {
		fmt.Println(text.FgRed.Sprintf("Failed to create K8s client: %v", err))
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get current context name
	if contextName == "" {
		config, err := k8s.LoadKubeConfig(kubeconfigPath)
		if err == nil {
			contextName = config.CurrentContext
		}
	}

	fmt.Println(text.FgCyan.Sprintf("Listing namespaces from context: %s\n", contextName))

	// Get all namespaces
	namespaceList, err := client.Clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		fmt.Println(text.FgRed.Sprintf("Failed to list namespaces: %v", err))
		return err
	}

	if len(namespaceList.Items) == 0 {
		fmt.Println(text.FgYellow.Sprintf("No namespaces found"))
		return nil
	}

	// Check permissions for each namespace
	type NamespacePermission struct {
		Name      string
		HasAccess bool
		Status    string
		Error     string
	}

	permissions := []NamespacePermission{}

	for _, ns := range namespaceList.Items {
		perm := NamespacePermission{
			Name:   ns.Name,
			Status: string(ns.Status.Phase),
		}

		// Try to list pods to check permission
		_, err := client.Clientset.CoreV1().Pods(ns.Name).List(ctx, metav1.ListOptions{Limit: 1})
		if err != nil {
			perm.HasAccess = false
			if k8s.IsPermissionError(err) {
				perm.Error = "Permission Denied"
			} else {
				perm.Error = err.Error()
			}
		} else {
			perm.HasAccess = true
		}

		permissions = append(permissions, perm)
	}

	// Display results in table
	tablex := table.Table{}
	tablex.SetStyle(table.StyleLight)
	tablex.AppendRow(table.Row{"Namespace", "Status", "Access", "Notes"})

	accessibleCount := 0
	deniedCount := 0

	for _, perm := range permissions {
		accessStatus := ""
		notes := ""

		if perm.HasAccess {
			accessStatus = text.FgGreen.Sprint("✓ Allowed")
			accessibleCount++
		} else {
			accessStatus = text.FgRed.Sprint("✗ Denied")
			notes = perm.Error
			deniedCount++
		}

		tablex.AppendRow(table.Row{
			perm.Name,
			perm.Status,
			accessStatus,
			notes,
		})
	}

	fmt.Println(tablex.Render())

	// Summary
	fmt.Println(text.FgGreen.Sprintf("\n=== Summary ==="))
	fmt.Printf("Total namespaces: %d\n", len(permissions))
	fmt.Printf("Accessible: %d\n", accessibleCount)
	fmt.Printf("Denied: %d\n", deniedCount)

	if accessibleCount > 0 {
		// Show accessible namespaces as comma-separated list
		accessible := []string{}
		for _, perm := range permissions {
			if perm.HasAccess {
				accessible = append(accessible, perm.Name)
			}
		}
		fmt.Println(text.FgCyan.Sprintf("\nAccessible namespaces (for use with --namespaces flag):"))
		fmt.Println(strings.Join(accessible, ","))
	}

	return nil
}

// GetAccessibleNamespaces returns a list of namespaces the user has permission to access
func GetAccessibleNamespaces(kubeconfigPath string, contextName string) ([]string, error) {
	// Create K8s client
	client, err := k8s.NewK8sClient(kubeconfigPath, contextName, []string{})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get all namespaces
	namespaceList, err := client.Clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	accessible := []string{}

	// Check permissions for each namespace
	for _, ns := range namespaceList.Items {
		// Try to list pods to check permission
		_, err := client.Clientset.CoreV1().Pods(ns.Name).List(ctx, metav1.ListOptions{Limit: 1})
		if err == nil {
			// Has access
			accessible = append(accessible, ns.Name)
		}
		// Skip namespaces without access (silently)
	}

	return accessible, nil
}
