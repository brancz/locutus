package jsonnet

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"sort"
	"strings"

	"github.com/brancz/locutus/render"
	rolloutTypes "github.com/brancz/locutus/rollout/types"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/google/go-jsonnet"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Renderer struct {
	logger     log.Logger
	entrypoint string
	sources    map[string]func() ([]byte, error)
}

func NewRenderer(
	logger log.Logger,
	entrypoint string,
	sources map[string]func() ([]byte, error),
) *Renderer {
	return &Renderer{
		logger:     logger,
		entrypoint: entrypoint,
		sources:    sources,
	}
}

type result struct {
	Objects map[string]map[string]interface{} `json:"objects"`
	Rollout *rolloutTypes.Rollout             `json:"rollout"`
}

func (r *Renderer) Render(config []byte) (*render.Result, error) {
	jsonnetMain := r.entrypoint
	jpaths := []string{"vendor"}
	jsonnetMainContent, err := ioutil.ReadFile(jsonnetMain)
	if err != nil {
		return nil, fmt.Errorf("could not read main jsonnet file: %s", jsonnetMain)
	}

	level.Debug(r.logger).Log("msg", "start evaluating jsonnet")

	vm := jsonnet.MakeVM()
	vm.Importer(&jsonnetImporter{
		logger:            r.logger,
		sources:           r.sources,
		fileImporter:      &jsonnet.FileImporter{JPaths: jpaths},
		configContent:     config,
		virtualConfigPath: "generic-operator/config",
	})
	rawJson, err := vm.EvaluateAnonymousSnippet(jsonnetMain, string(jsonnetMainContent))
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate: %v", err)
	}

	level.Debug(r.logger).Log("msg", "finished evaluating jsonnet")

	var res result
	err = json.Unmarshal([]byte(rawJson), &res)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal generated json: %v", err)
	}

	objects := map[string]*unstructured.Unstructured{}
	for k, v := range res.Objects {
		u := &unstructured.Unstructured{Object: v}
		b, err := u.MarshalJSON()
		if err != nil {
			return nil, fmt.Errorf("marshal previously unmarshaled json: %w", err)
		}
		level.Debug(r.logger).Log("msg", "finished evaluating jsonnet", "object", k, "content", string(b))
		objects[k] = u
	}

	return &render.Result{
		Objects: objects,
		Rollout: res.Rollout,
	}, nil
}

type jsonnetImporter struct {
	logger            log.Logger
	fileImporter      *jsonnet.FileImporter
	configContent     []byte
	virtualConfigPath string
	sources           map[string]func() ([]byte, error)
}

func (i *jsonnetImporter) Import(dir, importedPath string) (contents jsonnet.Contents, foundAt string, err error) {
	if importedPath == i.virtualConfigPath {
		return jsonnet.MakeContents(string(i.configContent)), i.virtualConfigPath, nil
	}

	sourceNames := []string{}
	for k := range i.sources {
		sourceNames = append(sourceNames, k)
	}
	sort.Strings(sourceNames)
	level.Debug(i.logger).Log("msg", "available dynamic sources", "sources", strings.Join(sourceNames, ","))

	if strings.HasPrefix(importedPath, "locutus-runtime/") {
		p := strings.TrimPrefix(importedPath, "locutus-runtime/")
		f, found := i.sources[p]
		if found {
			b, err := f()
			if err != nil {
				return jsonnet.Contents{}, "", err
			}

			key := "locutus-runtime/" + p
			level.Debug(i.logger).Log("msg", "rendering dynamic import", "import-path", key, "content", string(b))

			return jsonnet.MakeContents(string(b)), key, nil
		}
	}

	return i.fileImporter.Import(dir, importedPath)
}
