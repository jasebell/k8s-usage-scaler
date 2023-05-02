package cmd

import (
	"context"
	"fmt"
	"log"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func Run(labelSelector string, namespace string) {
	ctx := context.Background()
	kubeconfig := "/Users/jasonbell/.kube/config"
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Fatalf("Error building kubeconfig: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error creating clientset: %v", err)
	}

	minReplicas := int32(1)
	maxReplicas := int32(10)
	scaleUpThreshold := 80
	scaleDownThreshold := 20

	for {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: labelSelector,
		})

		fmt.Println(pods)

		if err != nil {
			log.Printf("Error listing pods: %v", err)
			continue
		}

		metrics, err := collectMetrics(pods.Items, config, namespace)
		if err != nil {
			log.Printf("Error collecting metrics: %v", err)
			continue
		}

		scaleDecision, err := makeScalingDecision(metrics, minReplicas, maxReplicas, scaleUpThreshold, scaleDownThreshold)
		if err != nil {
			log.Printf("Error making scaling decision: %v", err)
			continue
		}

		if scaleDecision != 0 {
			err := scalePods(clientset, namespace, labelSelector, scaleDecision)
			if err != nil {
				log.Printf("Error scaling pods: %v", err)
			} else {
				log.Printf("Successfully scaled pods with decision: %d", scaleDecision)
			}
		}

		time.Sleep(60 * time.Second)
	}
}
