package jsonnet

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/brancz/locutus/render"
	rolloutTypes "github.com/brancz/locutus/rollout/types"
	"github.com/go-kit/kit/log"
	"github.com/google/go-jsonnet"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Renderer struct {
	logger     log.Logger
	entrypoint string
}

func NewRenderer(logger log.Logger, entrypoint string) *Renderer {
	return &Renderer{logger: logger, entrypoint: entrypoint}
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

	vm := jsonnet.MakeVM()
	vm.Importer(&jsonnetImporter{
		fileImporter:      &jsonnet.FileImporter{JPaths: jpaths},
		configContent:     config,
		virtualConfigPath: "generic-operator/config",
	})
	rawJson, err := vm.EvaluateSnippet(jsonnetMain, string(jsonnetMainContent))
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate: %v", err)
	}

	var res result
	err = json.Unmarshal([]byte(rawJson), &res)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal generated json: %v", err)
	}

	objects := map[string]*unstructured.Unstructured{}
	for k, v := range res.Objects {
		objects[k] = &unstructured.Unstructured{Object: v}
	}

	return &render.Result{
		Objects: objects,
		Rollout: res.Rollout,
	}, nil
}

type jsonnetImporter struct {
	fileImporter      *jsonnet.FileImporter
	configContent     []byte
	virtualConfigPath string
}

func (i *jsonnetImporter) Import(dir, importedPath string) (contents jsonnet.Contents, foundAt string, err error) {
	if importedPath == i.virtualConfigPath {
		return jsonnet.MakeContents(string(i.configContent)), i.virtualConfigPath, nil
	}

	return i.fileImporter.Import(dir, importedPath)
}
