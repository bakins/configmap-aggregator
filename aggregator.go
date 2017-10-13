package aggregator

import (
	"net/http"
	"net/url"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/spf13/afero"
	"go.uber.org/zap"
)

// Aggregator reads config maps, writes to a directory,
// and, optionally, calls a webhook
type Aggregator struct {
	namespaces    []string
	selector      string
	lister        ConfigMapLister
	webhook       string
	webhookMethod string
	outputDir     string
	logger        *zap.Logger
	fs            afero.Fs
	dryRun        bool
}

// OptionsFunc are used when creating a new Aggregator
type OptionsFunc func(*Aggregator) error

// New creates a new Aggregator
func New(options ...OptionsFunc) (*Aggregator, error) {
	a := &Aggregator{
		webhookMethod: "POST",
		outputDir:     ".",
	}

	for _, f := range options {
		if err := f(a); err != nil {
			return nil, errors.Wrap(err, "failed to run options function")
		}
	}

	if a.logger == nil {
		l, err := NewLogger()
		if err != nil {
			return nil, errors.Wrap(err, "failed to create logger")
		}
		a.logger = l
	}

	if a.lister == nil {
		return nil, errors.New("no config map lister was set")
	}

	if len(a.namespaces) == 0 {
		// default to all namespaces
		a.namespaces = []string{""}
	}

	if a.fs == nil {
		a.fs = afero.NewOsFs()
	}

	return a, nil
}

// SetNamespaces sets the namespaces to query.
// By default, all namespaces are queried.
// Generally only used when creating a new Aggregator.
func SetNamespaces(namespaces []string) OptionsFunc {
	return func(a *Aggregator) error {
		a.namespaces = namespaces
		return nil
	}
}

// SetLabelSelector sets the labels that config maps must match
// By default, all config maps are matched, which is usually not what you want..
// Generally only used when creating a new Aggregator.
func SetLabelSelector(selector string) OptionsFunc {
	return func(a *Aggregator) error {
		// TODO: ensure the selector is valid
		a.selector = selector
		return nil
	}
}

// SetConfigMapLister sets the lister to use to get configmaps
// Generally only used when creating a new Aggregator.
func SetConfigMapLister(l ConfigMapLister) OptionsFunc {
	return func(a *Aggregator) error {
		a.lister = l
		return nil
	}
}

// SetLogger creates a function that will set the logger.
// Generally only used when creating a new Aggregator.
func SetLogger(l *zap.Logger) OptionsFunc {
	return func(a *Aggregator) error {
		a.logger = l
		return nil
	}
}

// SetWebHook creates a function that will set the webhook url.
// Generally only used when creating a new Aggregator.
func SetWebHook(webhook string) OptionsFunc {
	return func(a *Aggregator) error {
		_, err := url.Parse(webhook)
		if err != nil {
			return errors.Wrap(err, "failed to parse webhook")
		}
		a.webhook = webhook
		return nil
	}
}

// SetOutputDir creates a function that will set the output directory.
// Generally only used when creating a new Aggregator.
func SetOutputDir(dir string) OptionsFunc {
	return func(a *Aggregator) error {
		a.outputDir = dir
		return nil
	}
}

// SetFS creates a function that will set the Fs.
// Generally only used when testing and creating a new Aggregator.
func SetFS(fs afero.Fs) OptionsFunc {
	return func(a *Aggregator) error {
		a.fs = fs
		return nil
	}
}

// Once runs the loop once
func (a *Aggregator) Once() error {
	//get existing files
	files, err := afero.ReadDir(a.fs, a.outputDir)
	if err != nil {
		return errors.Wrapf(err, "failed to list files in %s", a.outputDir)
	}

	existingFiles := map[string]bool{}
	for _, f := range files {
		if f.IsDir() {
			// just skip dirs. we probably should error here as we expect output
			// dir to be empty
			continue
		}
		existingFiles[f.Name()] = true
	}

	changed := false

	for _, n := range a.namespaces {
		list, err := a.lister.List(n, a.selector)
		if err != nil {
			return errors.Wrapf(err, "failed to list config maps for namespace %s", "")
		}
		for _, item := range list.Items {
		DATA:
			for key, val := range item.Data {
				name := filepath.Join(a.outputDir, item.ObjectMeta.Namespace+"_"+item.ObjectMeta.Name+"_"+key)

				_, ok := existingFiles[name]
				if ok {
					// delete file from , so we can cleanup non-managed files later
					delete(existingFiles, name)

					contents, err := afero.ReadFile(a.fs, name)
					if err != nil {
						return errors.Wrapf(err, "failed to read file %s", name)
					}
					if string(contents) == val {
						// skip unchanged file
						a.logger.Debug("contents match", zap.String("file", name))
						continue DATA
					}

				}

				// if we made it to here, then need to write file
				changed = true
				if err := afero.WriteFile(a.fs, name, []byte(val), 0644); err != nil {
					return errors.Wrapf(err, "failed to write file %s", name)
				}
			}
		}
	}

	// delete any files left in map as we do not know about them
	for k, _ := range existingFiles {
		if err := a.fs.Remove(k); err != nil {
			return errors.Wrapf(err, "failed to remove file %s", k)
		}
		changed = true
	}

	if changed && a.webhook != "" {
		req, err := http.NewRequest(a.webhookMethod, a.webhook, nil)
		if err != nil {
			return errors.Wrap(err, "failed to create http request for webhook")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return errors.Wrap(err, "webhook request failed")
		}
		if resp.StatusCode >= 400 {
			return errors.Wrapf(err, "webhook request receive unexpected status %d", resp.StatusCode)
		}
	}
	return nil
}

// Run starts server mode
func (a *Aggregator) Run() error {
	return nil
}
