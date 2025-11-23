package pkg

import (
	"context"
	"fmt"
	"net"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

// K8sClient represents a Kubernetes client with context
type K8sClient struct {
	Clientset  kubernetes.Interface
	Config     *api.Config
	Namespaces []string
}

// LoadKubeConfig loads kubeconfig from the specified path
func LoadKubeConfig(kubeconfigPath string) (*api.Config, error) {
	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}
	return config, nil
}

// GetContexts returns all contexts from kubeconfig
func GetContexts(config *api.Config) []string {
	contexts := make([]string, 0, len(config.Contexts))
	for name := range config.Contexts {
		contexts = append(contexts, name)
	}
	return contexts
}

// NewK8sClient creates a new Kubernetes client from kubeconfig path and context
func NewK8sClient(kubeconfigPath string, contextName string, namespaces []string) (*K8sClient, error) {
	config, err := LoadKubeConfig(kubeconfigPath)
	if err != nil {
		return nil, err
	}

	// If context is not specified, use current context
	if contextName == "" {
		contextName = config.CurrentContext
	}

	// Build client config
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
		&clientcmd.ConfigOverrides{CurrentContext: contextName},
	)

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create rest config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return &K8sClient{
		Clientset:  clientset,
		Config:     config,
		Namespaces: namespaces,
	}, nil
}

// PodInfo represents pod information
type PodInfo struct {
	Name        string
	Namespace   string
	PodIP       string
	HostIP      string
	OwnerKind   string
	OwnerName   string
	Labels      map[string]string
	Annotations map[string]string
}

// ServiceInfo represents service information
type ServiceInfo struct {
	Name        string
	Namespace   string
	ClusterIP   string
	ExternalIPs []string
	Type        string
	Ports       []corev1.ServicePort
	Selector    map[string]string
}

// SearchByIP searches for resources by IP address (pod IP, service IP, or LoadBalancer IP)
func (c *K8sClient) SearchByIP(ctx context.Context, ip string) ([]PodInfo, []ServiceInfo, error) {
	pods := []PodInfo{}
	services := []ServiceInfo{}

	// Search in all specified namespaces
	for _, namespace := range c.Namespaces {
		// Search pods by IP
		podList, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			// Skip silently if permission denied
			if isPermissionError(err) {
				continue
			}
			return nil, nil, fmt.Errorf("failed to list pods in namespace %s: %w", namespace, err)
		}

		for _, pod := range podList.Items {
			if pod.Status.PodIP == ip || pod.Status.HostIP == ip {
				ownerKind, ownerName := getOwnerInfo(&pod)
				pods = append(pods, PodInfo{
					Name:        pod.Name,
					Namespace:   pod.Namespace,
					PodIP:       pod.Status.PodIP,
					HostIP:      pod.Status.HostIP,
					OwnerKind:   ownerKind,
					OwnerName:   ownerName,
					Labels:      pod.Labels,
					Annotations: pod.Annotations,
				})
			}
		}

		// Search services by ClusterIP or LoadBalancer IP
		svcList, err := c.Clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			// Skip silently if permission denied
			if isPermissionError(err) {
				continue
			}
			return nil, nil, fmt.Errorf("failed to list services in namespace %s: %w", namespace, err)
		}

		for _, svc := range svcList.Items {
			matched := false

			// Check ClusterIP
			if svc.Spec.ClusterIP == ip {
				matched = true
			}

			// Check ExternalIPs
			for _, externalIP := range svc.Spec.ExternalIPs {
				if externalIP == ip {
					matched = true
					break
				}
			}

			// Check LoadBalancer IPs
			if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
				for _, ingress := range svc.Status.LoadBalancer.Ingress {
					if ingress.IP == ip {
						matched = true
						break
					}
				}
			}

			if matched {
				services = append(services, ServiceInfo{
					Name:        svc.Name,
					Namespace:   svc.Namespace,
					ClusterIP:   svc.Spec.ClusterIP,
					ExternalIPs: svc.Spec.ExternalIPs,
					Type:        string(svc.Spec.Type),
					Ports:       svc.Spec.Ports,
					Selector:    svc.Spec.Selector,
				})
			}
		}
	}

	return pods, services, nil
}

// SearchByName searches for pods by name (supports partial match)
func (c *K8sClient) SearchByName(ctx context.Context, name string) ([]PodInfo, error) {
	pods := []PodInfo{}

	// Search in all specified namespaces
	for _, namespace := range c.Namespaces {
		podList, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			// Skip silently if permission denied
			if isPermissionError(err) {
				continue
			}
			return nil, fmt.Errorf("failed to list pods in namespace %s: %w", namespace, err)
		}

		for _, pod := range podList.Items {
			if strings.Contains(pod.Name, name) {
				ownerKind, ownerName := getOwnerInfo(&pod)
				pods = append(pods, PodInfo{
					Name:        pod.Name,
					Namespace:   pod.Namespace,
					PodIP:       pod.Status.PodIP,
					HostIP:      pod.Status.HostIP,
					OwnerKind:   ownerKind,
					OwnerName:   ownerName,
					Labels:      pod.Labels,
					Annotations: pod.Annotations,
				})
			}
		}
	}

	return pods, nil
}

// getOwnerInfo extracts owner information from pod
func getOwnerInfo(pod *corev1.Pod) (string, string) {
	if len(pod.OwnerReferences) == 0 {
		return "", ""
	}

	owner := pod.OwnerReferences[0]
	return owner.Kind, owner.Name
}

// ValidateIP validates if a string is a valid IP address
func ValidateIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

// IsPermissionError checks if an error is a permission/forbidden error (exported for use in cmdbutils)
func IsPermissionError(err error) bool {
	return apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err)
}

// isPermissionError is the internal version (kept for backward compatibility)
func isPermissionError(err error) bool {
	return IsPermissionError(err)
}

// GetDeploymentByReplicaSet gets deployment name from ReplicaSet
func (c *K8sClient) GetDeploymentByReplicaSet(ctx context.Context, namespace, replicaSetName string) (string, error) {
	rs, err := c.Clientset.AppsV1().ReplicaSets(namespace).Get(ctx, replicaSetName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get replicaset: %w", err)
	}

	if len(rs.OwnerReferences) == 0 {
		return "", fmt.Errorf("replicaset has no owner")
	}

	for _, owner := range rs.OwnerReferences {
		if owner.Kind == "Deployment" {
			return owner.Name, nil
		}
	}

	return "", fmt.Errorf("no deployment found for replicaset")
}

// SearchResultWithContext represents search results with context information
type SearchResultWithContext struct {
	Context   string
	Namespace string
	Pods      []PodInfo
	Services  []ServiceInfo
}

// SearchByIPAllContexts searches for resources by IP across all contexts and all (or specified) namespaces
func SearchByIPAllContexts(ctx context.Context, kubeconfigPath string, ip string, namespaces []string) ([]SearchResultWithContext, error) {
	config, err := LoadKubeConfig(kubeconfigPath)
	if err != nil {
		return nil, err
	}

	results := []SearchResultWithContext{}
	contexts := GetContexts(config)

	// Search in each context
	for _, contextName := range contexts {
		// Create client for this context
		client, err := NewK8sClient(kubeconfigPath, contextName, []string{})
		if err != nil {
			// Skip contexts that fail to initialize (might not have access)
			continue
		}

		// Determine which namespaces to search
		var namespacesToSearch []string
		if len(namespaces) > 0 {
			// Use provided namespace list
			namespacesToSearch = namespaces
		} else {
			// Get all namespaces in this context
			namespaceList, err := client.Clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
			if err != nil {
				// Skip if can't list namespaces
				continue
			}
			for _, ns := range namespaceList.Items {
				namespacesToSearch = append(namespacesToSearch, ns.Name)
			}
		}

		// Search in each namespace
		for _, nsName := range namespacesToSearch {
			client.Namespaces = []string{nsName}
			pods, services, err := client.SearchByIP(ctx, ip)
			if err != nil {
				// Continue even if one namespace fails
				// Uncomment for debugging: fmt.Printf("DEBUG: Error searching namespace %s: %v\n", nsName, err)
				continue
			}

			// Only add results if found something
			if len(pods) > 0 || len(services) > 0 {
				results = append(results, SearchResultWithContext{
					Context:   contextName,
					Namespace: nsName,
					Pods:      pods,
					Services:  services,
				})
			}
		}
	}

	return results, nil
}

// PodResultWithContext represents pod search results with context information
type PodResultWithContext struct {
	Context   string
	Namespace string
	Pods      []PodInfo
}

// SearchByNameAllContexts searches for pods by name across all contexts and all (or specified) namespaces
func SearchByNameAllContexts(ctx context.Context, kubeconfigPath string, name string, namespaces []string) ([]PodResultWithContext, error) {
	config, err := LoadKubeConfig(kubeconfigPath)
	if err != nil {
		return nil, err
	}

	results := []PodResultWithContext{}
	contexts := GetContexts(config)

	// Search in each context
	for _, contextName := range contexts {
		// Create client for this context
		client, err := NewK8sClient(kubeconfigPath, contextName, []string{})
		if err != nil {
			// Skip contexts that fail to initialize
			continue
		}

		// Determine which namespaces to search
		var namespacesToSearch []string
		if len(namespaces) > 0 {
			// Use provided namespace list
			namespacesToSearch = namespaces
		} else {
			// Get all namespaces in this context
			namespaceList, err := client.Clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
			if err != nil {
				// Skip if can't list namespaces
				continue
			}
			for _, ns := range namespaceList.Items {
				namespacesToSearch = append(namespacesToSearch, ns.Name)
			}
		}

		// Search in each namespace
		for _, nsName := range namespacesToSearch {
			client.Namespaces = []string{nsName}
			pods, err := client.SearchByName(ctx, name)
			if err != nil {
				// Continue even if one namespace fails
				continue
			}

			// Only add results if found something
			if len(pods) > 0 {
				results = append(results, PodResultWithContext{
					Context:   contextName,
					Namespace: nsName,
					Pods:      pods,
				})
			}
		}
	}

	return results, nil
}
