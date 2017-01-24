# configmap-aggregator

This is a simple utility for Kubernetes for aggregating several [config maps](https://kubernetes.io/docs/user-guide/configmap/) into a single one.

A pattern that has emerged is to store configuration files in a config map and mount
them into a Pod.  Often times, applications can include all files in a directory.  
A config map can be mounted as a directory, but you can't natively combine multiple
config maps into a single directory.  As I want to manage some config files - for proemthues for example -
with the application (in [helm](https://github.com/kubernetes/helm) charts, perhaps), I needed
a utility to do this.  Hence, `configmap-aggregator`

# Usage

./configmap-aggregator <target-namespace> <target-name>

where target is the config map that will hold the aggregated data.  The keys in
the resulting config map will be in the form `<namespace>-<name>-<key>`.

You may limit the namespaces searched by passing in the `--namespace=<namespace>` flag.
This can be used multiple times. By default, all namespaces are search.

You may also specify a label query, by passing the `--selector=<key=value>` flag.

Generally, run an instance of `configmap-aggregator` for each targeted config map. In the future,
this may be driven by a [third party resource](https://kubernetes.io/docs/user-guide/thirdpartyresources/).

Note: we assume you are running `kubectl` in proxy mode to handle authentication with
Kubernetes.

# Status

It works, but is not well tested.

# License

See [LICENSE](./LICENSE)
