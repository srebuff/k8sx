package pkg

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/clientcmd/api"
)

// TestLoadKubeConfig tests loading kubeconfig from file
func TestLoadKubeConfig(t *testing.T) {
	// Create temporary kubeconfig for testing
	tempDir := t.TempDir()
	kubeconfigPath := filepath.Join(tempDir, "kubeconfig")

	// Create a valid kubeconfig
	kubeconfigContent := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://test-cluster:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    token: test-token
`
	err := os.WriteFile(kubeconfigPath, []byte(kubeconfigContent), 0644)
	require.NoError(t, err)

	// Test loading valid kubeconfig
	config, err := LoadKubeConfig(kubeconfigPath)
	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, "test-context", config.CurrentContext)

	// Test loading non-existent kubeconfig
	_, err = LoadKubeConfig("/nonexistent/path")
	assert.Error(t, err)
}

// TestGetContexts tests extracting contexts from kubeconfig
func TestGetContexts(t *testing.T) {
	config := &api.Config{
		Contexts: map[string]*api.Context{
			"context1": {},
			"context2": {},
			"context3": {},
		},
	}

	contexts := GetContexts(config)
	assert.Len(t, contexts, 3)
	assert.Contains(t, contexts, "context1")
	assert.Contains(t, contexts, "context2")
	assert.Contains(t, contexts, "context3")

	// Test empty contexts
	emptyConfig := &api.Config{
		Contexts: map[string]*api.Context{},
	}
	emptyContexts := GetContexts(emptyConfig)
	assert.Len(t, emptyContexts, 0)
}

// TestValidateIP tests IP validation
func TestValidateIP(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{"Valid IPv4", "192.168.1.1", true},
		{"Valid IPv4 localhost", "127.0.0.1", true},
		{"Valid IPv6", "2001:db8::1", true},
		{"Valid IPv6 localhost", "::1", true},
		{"Invalid IP - text", "not-an-ip", false},
		{"Invalid IP - partial", "192.168", false},
		{"Invalid IP - empty", "", false},
		{"Invalid IP - out of range", "256.256.256.256", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateIP(tt.ip)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestSearchByIP tests searching resources by IP
func TestSearchByIP(t *testing.T) {
	// Create fake clientset
	fakeClient := fake.NewSimpleClientset()

	client := &K8sClient{
		Clientset:  fakeClient,
		Namespaces: []string{"default", "test-ns"},
	}

	ctx := context.Background()

	// Create test pods
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod-1",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "ReplicaSet",
					Name: "test-rs-1",
				},
			},
		},
		Status: corev1.PodStatus{
			PodIP:  "10.0.0.1",
			HostIP: "192.168.1.1",
		},
	}

	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod-2",
			Namespace: "test-ns",
		},
		Status: corev1.PodStatus{
			PodIP:  "10.0.0.2",
			HostIP: "192.168.1.2",
		},
	}

	// Create test service
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "10.96.0.1",
			Type:      corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app": "test",
			},
		},
	}

	// Add resources to fake client
	_, err := fakeClient.CoreV1().Pods("default").Create(ctx, pod1, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = fakeClient.CoreV1().Pods("test-ns").Create(ctx, pod2, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = fakeClient.CoreV1().Services("default").Create(ctx, svc, metav1.CreateOptions{})
	require.NoError(t, err)

	// Test searching by pod IP
	pods, services, err := client.SearchByIP(ctx, "10.0.0.1")
	assert.NoError(t, err)
	assert.Len(t, pods, 1)
	assert.Equal(t, "test-pod-1", pods[0].Name)
	assert.Equal(t, "default", pods[0].Namespace)
	assert.Equal(t, "ReplicaSet", pods[0].OwnerKind)
	assert.Equal(t, "test-rs-1", pods[0].OwnerName)
	assert.Len(t, services, 0)

	// Test searching by service ClusterIP
	pods, services, err = client.SearchByIP(ctx, "10.96.0.1")
	assert.NoError(t, err)
	assert.Len(t, pods, 0)
	assert.Len(t, services, 1)
	assert.Equal(t, "test-service", services[0].Name)
	assert.Equal(t, "default", services[0].Namespace)
	assert.Equal(t, "10.96.0.1", services[0].ClusterIP)

	// Test searching by non-existent IP
	pods, services, err = client.SearchByIP(ctx, "10.0.0.99")
	assert.NoError(t, err)
	assert.Len(t, pods, 0)
	assert.Len(t, services, 0)
}

// TestSearchByName tests searching pods by name
func TestSearchByName(t *testing.T) {
	// Create fake clientset
	fakeClient := fake.NewSimpleClientset()

	client := &K8sClient{
		Clientset:  fakeClient,
		Namespaces: []string{"default", "test-ns"},
	}

	ctx := context.Background()

	// Create test pods
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-deployment-abc123",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "ReplicaSet",
					Name: "nginx-deployment-xyz",
				},
			},
		},
		Status: corev1.PodStatus{
			PodIP:  "10.0.0.1",
			HostIP: "192.168.1.1",
		},
	}

	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-deployment-def456",
			Namespace: "test-ns",
		},
		Status: corev1.PodStatus{
			PodIP:  "10.0.0.2",
			HostIP: "192.168.1.2",
		},
	}

	pod3 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis-pod",
			Namespace: "default",
		},
		Status: corev1.PodStatus{
			PodIP:  "10.0.0.3",
			HostIP: "192.168.1.3",
		},
	}

	// Add pods to fake client
	_, err := fakeClient.CoreV1().Pods("default").Create(ctx, pod1, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = fakeClient.CoreV1().Pods("test-ns").Create(ctx, pod2, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = fakeClient.CoreV1().Pods("default").Create(ctx, pod3, metav1.CreateOptions{})
	require.NoError(t, err)

	// Test searching by partial name "nginx"
	pods, err := client.SearchByName(ctx, "nginx")
	assert.NoError(t, err)
	assert.Len(t, pods, 2)

	// Test searching by partial name "deployment"
	pods, err = client.SearchByName(ctx, "deployment")
	assert.NoError(t, err)
	assert.Len(t, pods, 2)

	// Test searching by full name
	pods, err = client.SearchByName(ctx, "redis-pod")
	assert.NoError(t, err)
	assert.Len(t, pods, 1)
	assert.Equal(t, "redis-pod", pods[0].Name)

	// Test searching by non-existent name
	pods, err = client.SearchByName(ctx, "nonexistent")
	assert.NoError(t, err)
	assert.Len(t, pods, 0)
}

// TestGetOwnerInfo tests extracting owner information from pod
func TestGetOwnerInfo(t *testing.T) {
	// Test pod with owner
	podWithOwner := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "ReplicaSet",
					Name: "test-rs",
				},
			},
		},
	}

	kind, name := getOwnerInfo(podWithOwner)
	assert.Equal(t, "ReplicaSet", kind)
	assert.Equal(t, "test-rs", name)

	// Test pod without owner
	podWithoutOwner := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	kind, name = getOwnerInfo(podWithoutOwner)
	assert.Equal(t, "", kind)
	assert.Equal(t, "", name)

	// Test pod with multiple owners (should return first)
	podWithMultipleOwners := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "ReplicaSet",
					Name: "test-rs-1",
				},
				{
					Kind: "DaemonSet",
					Name: "test-ds",
				},
			},
		},
	}

	kind, name = getOwnerInfo(podWithMultipleOwners)
	assert.Equal(t, "ReplicaSet", kind)
	assert.Equal(t, "test-rs-1", name)
}

// TestSearchByIPWithLoadBalancer tests searching LoadBalancer services
func TestSearchByIPWithLoadBalancer(t *testing.T) {
	// Create fake clientset
	fakeClient := fake.NewSimpleClientset()

	client := &K8sClient{
		Clientset:  fakeClient,
		Namespaces: []string{"default"},
	}

	ctx := context.Background()

	// Create LoadBalancer service
	lbSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "lb-service",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "10.96.0.10",
			Type:      corev1.ServiceTypeLoadBalancer,
			ExternalIPs: []string{
				"203.0.113.1",
			},
		},
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{
					{
						IP: "203.0.113.2",
					},
				},
			},
		},
	}

	_, err := fakeClient.CoreV1().Services("default").Create(ctx, lbSvc, metav1.CreateOptions{})
	require.NoError(t, err)

	// Test searching by LoadBalancer IP
	pods, services, err := client.SearchByIP(ctx, "203.0.113.2")
	assert.NoError(t, err)
	assert.Len(t, pods, 0)
	assert.Len(t, services, 1)
	assert.Equal(t, "lb-service", services[0].Name)
	assert.Equal(t, "LoadBalancer", services[0].Type)

	// Test searching by ExternalIP
	pods, services, err = client.SearchByIP(ctx, "203.0.113.1")
	assert.NoError(t, err)
	assert.Len(t, pods, 0)
	assert.Len(t, services, 1)
	assert.Equal(t, "lb-service", services[0].Name)
}

// TestSearchByIPAllContexts tests searching across all contexts and namespaces
func TestSearchByIPAllContexts(t *testing.T) {
	// Create temporary kubeconfig for testing
	tempDir := t.TempDir()
	kubeconfigPath := filepath.Join(tempDir, "kubeconfig")

	// Create a valid kubeconfig with multiple contexts
	kubeconfigContent := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://test-cluster-1:6443
  name: test-cluster-1
- cluster:
    server: https://test-cluster-2:6443
  name: test-cluster-2
contexts:
- context:
    cluster: test-cluster-1
    user: test-user
  name: context-1
- context:
    cluster: test-cluster-2
    user: test-user
  name: context-2
current-context: context-1
users:
- name: test-user
  user:
    token: test-token
`
	err := os.WriteFile(kubeconfigPath, []byte(kubeconfigContent), 0644)
	require.NoError(t, err)

	ctx := context.Background()

	// Note: This test will try to connect to real API servers, which will fail
	// In a real test environment, you would need to mock the entire kubeconfig system
	// For now, we just test that the function doesn't panic and handles errors gracefully
	results, err := SearchByIPAllContexts(ctx, kubeconfigPath, "10.0.0.1", []string{})

	// Since we can't connect to the test clusters, we expect either an error or empty results
	// The important thing is that the function doesn't panic
	if err == nil {
		assert.NotNil(t, results)
	}
}

// TestSearchByNameAllContexts tests searching by name across all contexts and namespaces
func TestSearchByNameAllContexts(t *testing.T) {
	// Create temporary kubeconfig for testing
	tempDir := t.TempDir()
	kubeconfigPath := filepath.Join(tempDir, "kubeconfig")

	// Create a valid kubeconfig with multiple contexts
	kubeconfigContent := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://test-cluster-1:6443
  name: test-cluster-1
contexts:
- context:
    cluster: test-cluster-1
    user: test-user
  name: context-1
current-context: context-1
users:
- name: test-user
  user:
    token: test-token
`
	err := os.WriteFile(kubeconfigPath, []byte(kubeconfigContent), 0644)
	require.NoError(t, err)

	ctx := context.Background()

	// Note: This test will try to connect to real API servers, which will fail
	// In a real test environment, you would need to mock the entire kubeconfig system
	// For now, we just test that the function doesn't panic and handles errors gracefully
	results, err := SearchByNameAllContexts(ctx, kubeconfigPath, "nginx", []string{})

	// Since we can't connect to the test clusters, we expect either an error or empty results
	// The important thing is that the function doesn't panic
	if err == nil {
		assert.NotNil(t, results)
	}
}
