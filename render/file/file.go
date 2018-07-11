package file

import (
	"flag"
	"os"
	"path/filepath"
	"strings"

	"github.com/brancz/locutus/render/types"
	rolloutTypes "github.com/brancz/locutus/rollout/types"
	"github.com/go-kit/kit/log"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

type FileProvider struct {
	directory   string
	rolloutFile string
}

func NewProvider() types.Provider {
	return &FileProvider{}
}

func (p *FileProvider) RegisterFlags(s *flag.FlagSet) {
	s.StringVar(&p.directory, "renderer.file.dir", "manifests/", "Directory to read files from.")
	s.StringVar(&p.rolloutFile, "renderer.file.rollout", "rollout.yaml", "Plain rollout spec to read.")
}

func (p *FileProvider) NewRenderer(logger log.Logger) types.Renderer {
	return &FileRenderer{
		logger:      logger,
		directory:   p.directory,
		rolloutFile: p.rolloutFile,
	}
}

func (p *FileProvider) Name() string {
	return "file"
}

type FileRenderer struct {
	logger      log.Logger
	directory   string
	rolloutFile string
}

func (r *FileRenderer) Render(_ []byte) (*types.Result, error) {
	objects := map[string]*unstructured.Unstructured{}
	dir := r.directory

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
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

	return &types.Result{
		Objects: objects,
		Rollout: &rollout,
	}, nil
}
