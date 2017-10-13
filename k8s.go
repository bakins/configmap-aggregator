package aggregator

import (
	"github.com/pkg/errors"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// ConfigMapLister gets a list of config maps
type ConfigMapLister interface {
	List(namespace string, selector string) (*v1.ConfigMapList, error)
}

// K8s uses a real k8s client to list config maps
type K8s struct {
	client *kubernetes.Clientset
}

// NewK8s creates a new Kubernetes client.
// if kubeconfig is blank, an include client is used.
func NewK8s(kubeconfig string) (*K8s, error) {
	var config *rest.Config
	var err error
	if kubeconfig == "" {
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, errors.Wrap(err, "failed to create in cluster config")
		}
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create config from %s", kubeconfig)
		}
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create kubernetes client")
	}
	return &K8s{client: clientset}, nil
}

// List uses a Kubernetes client to list config maps
func (k *K8s) List(namespace string, selector string) (*v1.ConfigMapList, error) {
	list, err := k.client.CoreV1().ConfigMaps(namespace).List(metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list config maps for %s", namespace)
	}
	return list, nil
}
