package file

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/brancz/locutus/render"
	rolloutTypes "github.com/brancz/locutus/rollout/types"
	"github.com/go-kit/kit/log"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

type Renderer struct {
	logger      log.Logger
	directory   string
	rolloutFile string
}

func NewRenderer(logger log.Logger, directory, rolloutFile string) *Renderer {
	return &Renderer{
		logger:      logger,
		directory:   directory,
		rolloutFile: rolloutFile,
	}
}

func (r *Renderer) Render(_ []byte) (*render.Result, error) {
	objects := map[string]*unstructured.Unstructured{}
	dir := r.directory

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info == nil || info.IsDir() {
			// skip directories
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		res := map[string]interface{}{}
		err = yaml.NewYAMLOrJSONDecoder(f, 100).Decode(&res)
		if err != nil {
			return err
		}

		objects[strings.TrimPrefix(path, dir)] = &unstructured.Unstructured{Object: res}

		return nil
	})
	if err != nil {
		return nil, err
	}

	f, err := os.Open(r.rolloutFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var rollout rolloutTypes.Rollout
	err = yaml.NewYAMLOrJSONDecoder(f, 100).Decode(&rollout)
	if err != nil {
		return nil, err
	}

	return &render.Result{
		Objects: objects,
		Rollout: &rollout,
	}, nil
}
