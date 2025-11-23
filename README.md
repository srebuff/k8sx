### Usage


```
A tool to search Kubernetes resources by IP or name.
Supports searching pods, services, and their relationships.

If you provide a query without a subcommand, it will automatically search:
- By IP if the query is a valid IP address
- By name if the query is not an IP

Usage:
  k8sx [query] [flags]
  k8sx [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  ctx         List all contexts from kubeconfig
  help        Help about any command
  ns          List all namespaces you have permission to access
  s           Search for Kubernetes resources (auto-detects IP or name)

Flags:
      --context string       Context to use (empty = current context) (env: K8S_SEARCH_CONTEXT)
  -h, --help                 help for k8sx
      --kubeconfig string    Path to kubeconfig file (env: KUBECONFIG) (default "/root/.kube/config")
      --namespaces strings   Namespaces to search (comma-separated, empty = auto-discover accessible namespaces) (env: K8S_SEARCH_NAMESPACES) (default [])

Use "k8sx [command] --help" for more information about a command.

```

#### steps

- set default kubeconfig path, if it's /root/.kube/config skip 

```
export KUBECONFIG=xxxx
```


- set namespace enviroment

> in my work scene , i have no "get ns" permission , so i need to Preset namespaces list

```
export K8S_SEARCH_NAMESPACES=test,xxx...
```

- search by ip

> k8sx will check all context and all namespace to find the pod ip or svc ip 

![](./doc/image_ip.png)


- search by name

![](./doc/image_name.png)

