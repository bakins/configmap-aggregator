package aggregator

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type mockLister struct {
}

func (m *mockLister) List(namespace string, selector string) (*v1.ConfigMapList, error) {
	return &mockConfigMaps, nil
}

var mockConfigMaps = v1.ConfigMapList{
	Items: []v1.ConfigMap{
		v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "item1",
				Namespace: "default",
			},
			Data: map[string]string{
				"foo.txt": "1234567890",
				"bar.txt": "0987654321",
			},
		},
		v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "item2",
				Namespace: "default",
			},
			Data: map[string]string{
				"baz.txt": "qwertyuiop",
				"abc.txt": "asdfghjkl",
			},
		},
	},
}

func TestNew(t *testing.T) {
	tests := []struct {
		lister      ConfigMapLister
		url         string
		expectError bool
	}{
		{
			expectError: true,
		},
		{
			lister:      &mockLister{},
			expectError: false,
		},
		{
			lister:      &mockLister{},
			expectError: false,
			url:         "https://somehost:9090/foo",
		},
		{
			lister:      &mockLister{},
			expectError: true,
			url:         "\\http:/invalid url",
		},
	}

	for i, test := range tests {
		test := test
		name := fmt.Sprintf("%d", i)
		t.Run(name, func(t *testing.T) {
			a, err := New(
				SetConfigMapLister(test.lister),
				SetWebHook(test.url),
			)
			if test.expectError {
				require.Nil(t, a)
				require.NotNil(t, err)
			} else {
				require.NotNil(t, a)
				require.Nil(t, err)
			}
		})
	}
}

func TestOnceEmptyDir(t *testing.T) {
	fs := afero.NewMemMapFs()
	a, err := New(
		SetConfigMapLister(&mockLister{}),
		SetFS(fs),
	)
	require.NotNil(t, a)
	require.Nil(t, err)

	err = a.Once()
	require.Nil(t, err)

	files := []string{}
	afero.Walk(fs, "/", func(path string, info os.FileInfo, err error) error {
		files = append(files, path)
		return nil
	})
	// 5 is number of items plus "/"
	require.Equal(t, 5, len(files))
}

func TestOnceNonEmptyDir(t *testing.T) {
	fs := afero.NewMemMapFs()
	err := afero.WriteFile(fs, "random-file.json", []byte("data"), 0755)
	require.Nil(t, err)

	a, err := New(
		SetConfigMapLister(&mockLister{}),
		SetFS(fs),
	)
	require.NotNil(t, a)
	require.Nil(t, err)

	err = a.Once()
	require.Nil(t, err)

	files := []string{}
	afero.Walk(fs, "/", func(path string, info os.FileInfo, err error) error {
		files = append(files, path)
		return nil
	})
	// 5 is number of items plus "/"
	require.Equal(t, 5, len(files))
}

func TestOnceWithOutputDir(t *testing.T) {
	fs := afero.NewMemMapFs()
	err := afero.WriteFile(fs, "random-file.json", []byte("data"), 0755)
	require.Nil(t, err)
	err = fs.Mkdir("/tmp", 0777)

	a, err := New(
		SetConfigMapLister(&mockLister{}),
		SetFS(fs),
		SetOutputDir("/tmp"),
	)
	require.NotNil(t, a)
	require.Nil(t, err)

	err = a.Once()
	require.Nil(t, err)

	files := []string{}
	afero.Walk(fs, "/tmp", func(path string, info os.FileInfo, err error) error {
		files = append(files, path)
		return nil
	})
	// 6 is number of items plus "/tmp"
	require.Equal(t, 5, len(files))
}

func TestOnceWebHook(t *testing.T) {
	changed := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "POST", r.Method)
		changed = true
		fmt.Fprintln(w, "OK")
	}))
	defer ts.Close()

	fs := afero.NewMemMapFs()
	a, err := New(
		SetConfigMapLister(&mockLister{}),
		SetFS(fs),
		SetWebHook(ts.URL),
	)
	require.NotNil(t, a)
	require.Nil(t, err)

	err = a.Once()
	require.Nil(t, err)

	files := []string{}
	afero.Walk(fs, "/", func(path string, info os.FileInfo, err error) error {
		files = append(files, path)
		return nil
	})
	// 5 is number of items plus "/"
	require.Equal(t, 5, len(files))

	require.True(t, changed)
}
