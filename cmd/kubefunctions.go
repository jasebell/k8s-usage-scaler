package cmd

import (
	"context"
	"fmt"
	"math"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/metrics/pkg/client/clientset/versioned"
)

func collectMetrics(pods []corev1.Pod, config *rest.Config, namespace string) (map[string]float64, error) {
	metricsClient, err := versioned.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error creating metrics client: %v", err)
	}

	podMetrics := make(map[string]float64)

	for _, pod := range pods {
		podName := pod.Name
		podMetricsList, err := metricsClient.MetricsV1beta1().PodMetricses(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("error getting pod metrics for %s: %v", podName, err)
		}

		// Calculate the total CPU usage and memory usage for the pod
		totalCPU := int64(0)
		totalMemory := int64(0)
		for _, container := range podMetricsList.Containers {
			cpuQuantity := container.Usage.Cpu().MilliValue()
			memoryQuantity := container.Usage.Memory().Value()

			totalCPU += cpuQuantity
			totalMemory += memoryQuantity
		}

		// Calculate the average CPU and memory usage percentages for the pod
		cpuLimit := pod.Spec.Containers[0].Resources.Limits.Cpu().MilliValue()
		memoryLimit := pod.Spec.Containers[0].Resources.Limits.Memory().Value()

		avgCPUPercentage := float64(totalCPU) / float64(cpuLimit) * 100
		avgMemoryPercentage := float64(totalMemory) / float64(memoryLimit) * 100

		// Store the average CPU and memory usage percentages in the podMetrics map
		podMetrics[podName+"_cpu"] = avgCPUPercentage
		podMetrics[podName+"_memory"] = avgMemoryPercentage
	}

	return podMetrics, nil
}

func makeScalingDecision(metrics map[string]float64, minReplicas, maxReplicas int32, scaleUpThreshold, scaleDownThreshold int) (int, error) {
	if len(metrics) == 0 {
		return 0, fmt.Errorf("no metrics data available")
	}

	// Calculate the average CPU and memory usage across all target pods
	totalCPUPercentage := 0.0
	totalMemoryPercentage := 0.0
	podCount := 0

	for metricName, value := range metrics {
		if strings.HasSuffix(metricName, "_cpu") {
			totalCPUPercentage += value
			podCount++
		} else if strings.HasSuffix(metricName, "_memory") {
			totalMemoryPercentage += value
		}
	}

	avgCPUPercentage := totalCPUPercentage / float64(podCount)
	avgMemoryPercentage := totalMemoryPercentage / float64(podCount)

	// Determine if scaling is needed
	if avgCPUPercentage >= float64(scaleUpThreshold) || avgMemoryPercentage >= float64(scaleUpThreshold) {
		return int(math.Min(float64(maxReplicas), float64(podCount+1))), nil
	} else if avgCPUPercentage <= float64(scaleDownThreshold) && avgMemoryPercentage <= float64(scaleDownThreshold) {
		return int(math.Max(float64(minReplicas), float64(podCount-1))), nil
	}

	return 0, nil
}

func scalePods(clientset *kubernetes.Clientset, namespace, labelSelector string, scaleDecision int) error {
	// Retrieve and scale the matching deployments
	deployments, err := clientset.AppsV1().Deployments(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return fmt.Errorf("error listing deployments: %v", err)
	}

	for _, deployment := range deployments.Items {
		deployment.Spec.Replicas = int32Ptr(int32(scaleDecision))
		_, err = clientset.AppsV1().Deployments(namespace).Update(context.TODO(), &deployment, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("error updating deployment %s: %v", deployment.Name, err)
		}
	}

	// Retrieve and scale the matching ReplicaSets
	replicaSets, err := clientset.AppsV1().ReplicaSets(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return fmt.Errorf("error listing ReplicaSets: %v", err)
	}

	for _, replicaSet := range replicaSets.Items {
		replicaSet.Spec.Replicas = int32Ptr(int32(scaleDecision))
		_, err = clientset.AppsV1().ReplicaSets(namespace).Update(context.TODO(), &replicaSet, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("error updating ReplicaSet %s: %v", replicaSet.Name, err)
		}
	}

	return nil
}

func int32Ptr(i int32) *int32 {
	return &i
}
