package cluster

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/argon-chat/k3sd/utils"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	yamlserializer "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/discovery"
	memory "k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

func applyYAMLManifest(kubeconfigPath, manifestPathOrURL string, logger *utils.Logger, substitutions map[string]string) error {
	var data []byte
	var err error
	if strings.HasPrefix(manifestPathOrURL, "http://") || strings.HasPrefix(manifestPathOrURL, "https://") {
		resp, err := http.Get(manifestPathOrURL)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		data, err = io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
	} else {
		data, err = os.ReadFile(manifestPathOrURL)
		if err != nil {
			return err
		}
	}
	if substitutions != nil {
		content := string(data)
		for k, v := range substitutions {
			content = strings.ReplaceAll(content, k, v)
		}
		data = []byte(content)
	}
	var docs []string
	rawDocs := strings.Split(string(data), "\n---")
	for _, doc := range rawDocs {
		doc = strings.TrimSpace(doc)
		if doc == "" || strings.HasPrefix(doc, "#") {
			continue
		}
		docs = append(docs, doc)
	}
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return err
	}
	dyn, err := dynamic.NewForConfig(config)
	if err != nil {
		return err
	}
	decUnstructured := yamlserializer.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	disco, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return err
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(disco))
	for _, doc := range docs {
		obj := &unstructured.Unstructured{}
		_, _, err := decUnstructured.Decode([]byte(doc), nil, obj)
		if err != nil {
			logger.Log("YAML decode error: %v\n---\n%s", err, doc)
			continue
		}
		m := obj.GroupVersionKind()
		mapping, err := mapper.RESTMapping(m.GroupKind(), m.Version)
		if err != nil {
			logger.Log("RESTMapping error: %v", err)
			continue
		}
		ns := obj.GetNamespace()
		if ns == "" {
			ns = "default"
		}
		resource := dyn.Resource(mapping.Resource).Namespace(ns)
		_, err = resource.Create(context.TODO(), obj, metav1.CreateOptions{})
		if err != nil && !errors.IsAlreadyExists(err) {
			logger.Log("Apply error: %v", err)
		}
	}
	return nil
}
