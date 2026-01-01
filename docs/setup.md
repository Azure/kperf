# Cluster Setup

This document provides instructions for setting up Kubernetes clusters for kperf testing.

## Azure Kubernetes Service (AKS)

### Prerequisites

1. Install Azure CLI:
   ```bash
   curl -sL https://aka.ms/InstallAzureCLIDeb | sudo bash
   ```

2. Login to Azure:
   ```bash
   az login
   ```

3. Set your subscription:
   ```bash
   az account set --subscription <subscription-id>
   ```

### Create AKS Cluster

1. Create a resource group:
   ```bash
   az group create --name kperf --location eastus
   ```

2. Create the AKS cluster with system node pool:
   ```bash
   az aks create \
     --resource-group kperf \
     --name kperf \
     --node-count 3 \
     --node-vm-size Standard_D8s_v3 \
     --enable-cluster-autoscaler \
     --min-count 1 \
     --max-count 10
   ```

3. Add user node pool for kperf workloads:
   ```bash
   az aks nodepool add \
     --resource-group kperf \
     --cluster-name kperf \
     --name userpool \
     --node-count 3 \
     --node-vm-size Standard_D16s_v3 \
     --enable-cluster-autoscaler \
     --min-count 1 \
     --max-count 10
   ```

### Getting KubeConfig

To configure kubectl to connect to your AKS cluster:

```bash
az aks get-credentials --resource-group kperf --name kperf
```

Verify the connection:
```bash
kubectl get nodes
kubectl cluster-info
```

### Cleanup

To delete the AKS cluster and resources:
```bash
az group delete --name kperf --yes --no-wait
```


## Elastic Kubernetes Service (EKS)

### Getting KubeConfig
To configure kubectl to connect to your EKS cluster:
```bash
eksctl utils write-kubeconfig --cluster=<CLUSTER_NAME> --region=<REGION>
```

## Google Kubernetes Engine (GKE)

### Getting KubeConfig
To configure kubectl to connect to your GKE cluster:
```bash
gcloud container clusters get-credentials <CLUSTER_NAME> --region <REGION>
```
