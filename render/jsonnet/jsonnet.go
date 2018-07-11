package jsonnet

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"

	"github.com/brancz/locutus/render/types"
	rolloutTypes "github.com/brancz/locutus/rollout/types"
	"github.com/go-kit/kit/log"
	jsonnet "github.com/google/go-jsonnet"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type JsonnetProvider struct {
	entrypoint string
}

func NewProvider() types.Provider {
	return &JsonnetProvider{}
}

func (p *JsonnetProvider) RegisterFlags(s *flag.FlagSet) {
	s.StringVar(&p.entrypoint, "renderer.jsonnet.entrypoint", "jsonnet/main.jsonnet", "Jsonnet file to execute to render.")
}

func (p *JsonnetProvider) NewRenderer(logger log.Logger) types.Renderer {
	return &JsonnetRenderer{logger: logger, entrypoint: p.entrypoint}
}

func (p *JsonnetProvider) Name() string {
	return "jsonnet"
}

type JsonnetRenderer struct {
	logger log.Logger

	entrypoint string
}

type result struct {
	Objects map[string]map[string]interface{} `json:"objects"`
	Rollout *rolloutTypes.Rollout             `json:"rollout"`
}

func (r *JsonnetRenderer) Render(config []byte) (*types.Result, error) {
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

	return &types.Result{
		Objects: objects,
		Rollout: res.Rollout,
	}, nil
}

type jsonnetImporter struct {
	fileImporter      *jsonnet.FileImporter
	configContent     []byte
	virtualConfigPath string
}

func (i *jsonnetImporter) Import(dir, importedPath string) (*jsonnet.ImportedData, error) {
	if importedPath == i.virtualConfigPath {
		return &jsonnet.ImportedData{Content: string(i.configContent), FoundHere: i.virtualConfigPath}, nil
	}

	return i.fileImporter.Import(dir, importedPath)
}
