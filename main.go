package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func createClientSet() *kubernetes.Clientset {
	kubeconfig := filepath.Join(
		os.Getenv("HOME"), ".kube", "config",
	)
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.DefaultClientConfig = &clientcmd.DefaultClientConfig
	loadingRules.ExplicitPath = kubeconfig
	configOverrides := &clientcmd.ConfigOverrides{
		ClusterDefaults: clientcmd.ClusterDefaults,
		CurrentContext:  "",
	}
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides).ClientConfig()
	if err != nil {
		log.Fatal(err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal(err)
	}
	return clientset
}

func main() {
	clientset := createClientSet()
	// Store node name and availability zone in to a map
	nodeAzMap := populateNodesToAzMapping()
	// Get all namespaces in the cluster
	namespaces := getNamespaces()

	for _, ns := range namespaces {
		DeploymentsClient := clientset.AppsV1().Deployments(ns)
		deploymentObject, err := DeploymentsClient.List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			log.Fatal(err)
		}
		if len(deploymentObject.Items) == 0 {
			fmt.Printf("No Deployments found in %s namespace\n", ns)
			continue
		}
		for _, deployment := range deploymentObject.Items {
			deploymentName := deployment.ObjectMeta.Name
			fmt.Printf("%v:\n\n", deploymentName)
			// Get the labels the deployment uses to manage pods
			deploymentMatchLabels := getDeploymentMatchLabels(deploymentName, ns)
			// Get all the pods with the labels returned from getDeploymentMatchLabels()
			listOfPods := getPodsAndNode(deploymentMatchLabels)

			// Populate podsPerAZ with all of the az's available in the cluster, based on nodes in nodeAzMap
			podsPerAZ := map[string]int{}
			for _, az := range nodeAzMap {
				podsPerAZ[az] = 0
			}
			// Loop through the list of pods and assign the pod to an AZ
			for _, podNode := range listOfPods {
				podAZ := nodeAzMap[podNode]
				podsPerAZ[podAZ] = podsPerAZ[podAZ] + 1
			}

			// Sort output by AZ alphabetically
			keys := make([]string, 0, len(podsPerAZ))
			for az := range podsPerAZ {
				keys = append(keys, az)
			}
			sort.Strings(keys)

			/* Print the topology of each deployment, example:
			eu-west-2a: 3
			eu-west-2b: 7
			eu-west-2c: 11
			*/
			for _, az := range keys {
				fmt.Printf("%v: %v\n", az, podsPerAZ[az])
			}

			// Calculate the max skew, which is the highest number minus the lowest number
			skew := calculateTopologySkew(podsPerAZ)
			// Submit the skew to Datadog as a metric so that it can be used foir dashboards and monitors
			// submitSkewMetrics(deploymentName, ns, skew)
			fmt.Printf("Skew: %v\n\n", skew)
		}
	}
}

func submitSkewMetrics(deployment string, namespace string, skew int) {
	body := datadogV2.MetricPayload{
		Series: []datadogV2.MetricSeries{
			{
				Metric: "topology.skew",
				Type:   datadogV2.METRICINTAKETYPE_UNSPECIFIED.Ptr(),
				Points: []datadogV2.MetricPoint{
					{
						Timestamp: datadog.PtrInt64(time.Now().Unix()),
						Value:     datadog.PtrFloat64(float64(skew)),
					},
				},
				Resources: []datadogV2.MetricResource{
					{
						Type: datadog.PtrString("kube_deployment"),
						Name: datadog.PtrString(deployment),
					},
					{
						Type: datadog.PtrString("kube_namespace"),
						Name: datadog.PtrString(namespace),
					},
				},
			},
		},
	}
	ctx := datadog.NewDefaultContext(context.Background())
	configuration := datadog.NewConfiguration()
	apiClient := datadog.NewAPIClient(configuration)
	api := datadogV2.NewMetricsApi(apiClient)
	resp, r, err := api.SubmitMetrics(ctx, body, *datadogV2.NewSubmitMetricsOptionalParameters())

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `MetricsApi.SubmitMetrics`: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}

	responseContent, _ := json.MarshalIndent(resp, "", "  ")
	fmt.Fprintf(os.Stdout, "Response from `MetricsApi.SubmitMetrics`:\n%s\n", responseContent)
}

func calculateTopologySkew(topologyMap map[string]int) int {
	var numberOfPods []int
	// Take the value of pods from the map and assign it to a slice ready for sorting
	for _, numPods := range topologyMap {
		numberOfPods = append(numberOfPods, numPods)
	}
	// Sort the slice in ascending order
	sort.Ints(numberOfPods)
	// Return the result of the highest number minus the lowest number
	return numberOfPods[len(numberOfPods)-1] - numberOfPods[0]
}

func getNamespaces() []string {
	clientset := createClientSet()
	coreClient := clientset.CoreV1()

	namespaces, err := coreClient.Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Fatal(err)
	}
	var namespaceList []string
	for _, ns := range namespaces.Items {
		namespaceList = append(namespaceList, ns.ObjectMeta.Name)
	}
	return namespaceList
}

func getDeploymentMatchLabels(deployment string, namespace string) map[string]string {
	clientset := createClientSet()
	DeploymentsClient := clientset.AppsV1().Deployments(namespace)
	deploymentObject, err := DeploymentsClient.Get(context.TODO(), deployment, metav1.GetOptions{})
	if err != nil {
		log.Fatal(err)
	}
	return deploymentObject.Spec.Selector.MatchLabels
}

func getPodsAndNode(matchLabels map[string]string) map[string]string {
	clientset := createClientSet()
	coreClient := clientset.CoreV1()
	labelSelector := metav1.LabelSelector{MatchLabels: matchLabels}
	listOptions := metav1.ListOptions{
		LabelSelector: labels.Set(labelSelector.MatchLabels).String(),
	}
	podsToNode := make(map[string]string)
	pods, err := coreClient.Pods("").List(context.TODO(), listOptions)
	if err != nil {
		log.Fatal(err)
	}
	for _, pod := range pods.Items {
		podName := pod.ObjectMeta.Name
		podNode := pod.Spec.NodeName
		if podNode == "" {
			fmt.Printf("Pod %v isn't attached to a node\n", podName)
			continue
		}
		podsToNode[podName] = podNode
	}
	return podsToNode
}

func populateNodesToAzMapping() (nodeAzMap map[string]string) {
	clientset := createClientSet()
	coreClient := clientset.CoreV1()
	nodes, err := coreClient.Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Fatal(err)
	}
	if len(nodes.Items) == 0 {
		log.Fatal("There are 0 nodes in the cluster")
	}
	nodesToAZ := make(map[string]string)
	for _, node := range nodes.Items {
		nodeName := node.ObjectMeta.Name
		nodeAZ := node.ObjectMeta.Labels["topology.kubernetes.io/zone"]
		if nodeAZ == "" {
			log.Fatalf("Label 'topology.kubernetes.io/zone' doesn't exist on node %v", nodeName)
		}
		nodesToAZ[nodeName] = nodeAZ
	}
	return nodesToAZ
}
