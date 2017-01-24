package main

import (
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/davecgh/go-spew/spew"
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
	onetime            bool
	syncInterval       time.Duration
)

func main() {
	rootCmd.PersistentFlags().StringVarP(&selector, "selector", "s", "", "label selector")
	rootCmd.PersistentFlags().StringVarP(&endpoint, "endpoint", "e", "http://127.0.0.1:8001", "kubernetes endpoint")
	rootCmd.PersistentFlags().StringArrayVarP(&namespaces, "namespace", "n", nil, "namespace to query. can be used multiple times. default is all namespaces")
	rootCmd.PersistentFlags().BoolVarP(&onetime, "onetime", "o", false, "run one time and exit.")
	rootCmd.PersistentFlags().DurationVarP(&syncInterval, "sync-interval", "i", (60 * time.Second), "the time duration between template processing.")

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func runAggregator(cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		log.Fatal("namespace and name of target configmap is required")
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

	log.Println("Starting configmap-aggregator...")

	if err := c.client.waitForKubernetes(); err != nil {
		log.Fatal(err)
	}

	if onetime {
		if err := c.process(); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}

	var wg sync.WaitGroup
	done := make(chan struct{})

	go func() {
		wg.Add(1)
		for {
			if err := c.process(); err != nil {
				log.Printf("failed to process config maps: %v", err)
			}
			// TODO: info level?
			//else {
			//	log.Printf("configmap aggregation complete. Next sync in %v seconds.", syncInterval.Seconds())
			//}
			select {
			case <-time.After(syncInterval):
			case <-done:
				wg.Done()
				return
			}
		}
	}()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	<-signalChan
	log.Printf("Shutdown signal received, exiting...")
	close(done)
	wg.Wait()
	os.Exit(0)
}

func hashConfigMap(cm *ConfigMap) string {
	h := fnv.New64()
	printer := spew.ConfigState{
		Indent:         " ",
		SortKeys:       true,
		DisableMethods: true,
		SpewKeys:       true,
	}

	// we only hash the data for now
	printer.Fprintf(h, "%#v", cm.Data)
	return hex.EncodeToString(h.Sum(nil))
}

// true if they are the same
func compareConfigMaps(a, b *ConfigMap) bool {
	return hashConfigMap(a) == hashConfigMap(b)
}

func (c *controller) process() error {
	cm, err := c.createConfigMap()
	if err != nil {
		return err
	}
	return c.upsertConfigMap(cm)
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

	// XXX: unset fields on existing that will cause to not match
	// currently we don't unmarshal any

	if compareConfigMaps(existing, cm) {
		return nil
	}
	return c.client.updateConfigMap(cm)
}
