package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
)

// Label to apply to all namespaces
const namespaceLabelKey = "managed-by"
const namespaceLabelValue = "namespace-labeler"

// NamespaceController watches namespaces and adds a label
type NamespaceController struct {
	clientset kubernetes.Interface
	queue     workqueue.RateLimitingInterface
	informer  cache.SharedIndexInformer
}

// NewNamespaceController creates a new NamespaceController
func NewNamespaceController(clientset kubernetes.Interface) *NamespaceController {
	informer := cache.NewSharedIndexInformer(
		cache.NewListWatchFromClient(
			clientset.CoreV1().RESTClient(),
			"namespaces",
			metav1.NamespaceAll,
			fields.Everything(),
		),
		&corev1.Namespace{},
		0,
		cache.Indexers{},
	)

	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			queue.Add(obj)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			queue.Add(newObj)
		},
		DeleteFunc: func(obj interface{}) {
			// Optional: Handle deletion logic if needed
		},
	})

	return &NamespaceController{
		clientset: clientset,
		queue:     queue,
		informer:  informer,
	}
}

// Run starts the controller
func (c *NamespaceController) Run(stopCh <-chan struct{}) {
	defer runtime.HandleCrash()
	defer c.queue.ShutDown()

	fmt.Println("Starting Namespace Controller")

	go c.informer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, c.informer.HasSynced) {
		runtime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
		return
	}

	for {
		c.processNextItem()
	}
}

// processNextItem processes the next item in the queue
func (c *NamespaceController) processNextItem() {
	obj, shutdown := c.queue.Get()

	if shutdown {
		return
	}

	defer c.queue.Done(obj)

	err := c.syncNamespace(obj.(*corev1.Namespace))
	if err != nil {
		runtime.HandleError(fmt.Errorf("error syncing namespace: %v", err))
		c.queue.AddRateLimited(obj)
		return
	}

	c.queue.Forget(obj)
}

func (c *NamespaceController) syncNamespace(namespace *corev1.Namespace) error {
	labels := namespace.Labels
	if labels == nil {
		labels = make(map[string]string)
	}

	// Check if the label is already present
	if val, exists := labels[namespaceLabelKey]; exists && val == namespaceLabelValue {
		return nil
	}

	// Add or update the label
	labels[namespaceLabelKey] = namespaceLabelValue
	namespace.SetLabels(labels)

	_, err := c.clientset.CoreV1().Namespaces().Update(context.TODO(), namespace, metav1.UpdateOptions{})
	return err
}

func main() {
	kubeconfig := flag.String("kubeconfig", "", "Path to a kubeconfig file")
	flag.Parse()

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		fmt.Printf("Error building kubeconfig: %v\n", err)
		os.Exit(1)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Printf("Error building Kubernetes clientset: %v\n", err)
		os.Exit(1)
	}

	controller := NewNamespaceController(clientset)

	stopCh := make(chan struct{})
	defer close(stopCh)

	// Handle termination signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signalCh
		close(stopCh)
	}()

	controller.Run(stopCh)
}
