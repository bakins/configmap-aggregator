package main

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

type controller struct {
	client          *k8sClient
	targetNamespace string
	targetName      string
	selector        string
	namespaces      []string
}

var rootCmd = &cobra.Command{
	Use:   "configmap-aggregator [target-namespace] [target-name]",
	Short: "aggregates multiple configmaps into a single one",
	Run:   runAggregator,
}

var (
	selector, endpoint string
	namespaces         []string
)

func main() {
	rootCmd.PersistentFlags().StringVarP(&selector, "selector", "s", "", "label selector")
	rootCmd.PersistentFlags().StringVarP(&endpoint, "endpoint", "e", "http://127.0.0.1:8001", "kubernetes endpoint")
	rootCmd.PersistentFlags().StringArrayVarP(&namespaces, "namespace", "n", nil, "namespace to query. can be used multiple times. default is all namespaces")

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
}

func runAggregator(cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		fmt.Println("namespace and name of target configmap is required")
		os.Exit(-2)
	}

	if len(namespaces) == 0 {
		namespaces = append(namespaces, "")
	}
	c := &controller{
		client:          newk8sClient(endpoint),
		selector:        selector,
		namespaces:      namespaces,
		targetNamespace: args[0],
		targetName:      args[1],
	}

	cm, err := c.createConfigMap()
	if err != nil {
		fmt.Println(err)
		os.Exit(-4)
	}

	err = c.upsertConfigMap(cm)
	if err != nil {
		fmt.Println(err)
		os.Exit(-4)
	}

}

func (c *controller) createConfigMap() (*ConfigMap, error) {
	data := make(map[string]string)

	for _, n := range c.namespaces {
		list, err := c.client.getConfigMaps(n, selector)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get config maps for %s %s", n, c.selector)
		}

	ITEMS:
		for _, cm := range list.Items {
			if cm.Metadata.Namespace == c.targetNamespace && cm.Metadata.Name == c.targetName {
				continue ITEMS
			}
			for k, v := range cm.Data {
				name := fmt.Sprintf("%s_%s_%s", cm.Metadata.Namespace, cm.Metadata.Name, k)
				data[name] = v
			}
		}
	}

	cm := newConfigMap(c.targetNamespace, c.targetName)
	cm.Data = data
	cm.Metadata.Annotations["configmap-aggregator"] = "target"

	return cm, nil
}

func (c *controller) upsertConfigMap(cm *ConfigMap) error {
	existing, err := c.client.getConfigMap(c.targetNamespace, c.targetName)
	if err == ErrNotExist {
		return c.client.createConfigMap(cm)
	}
	if err != nil {
		return errors.Wrapf(err, "failed to get config map %s/%s", c.targetNamespace, c.targetName)
	}

	//copy labels, annotations, and version
	for k, v := range existing.Metadata.Annotations {
		cm.Metadata.Annotations[k] = v
	}
	for k, v := range existing.Metadata.Labels {
		cm.Metadata.Labels[k] = v
	}
	cm.Metadata.ResourceVersion = existing.Metadata.ResourceVersion

	return c.client.updateConfigMap(cm)
}
