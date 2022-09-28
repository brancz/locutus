package e2etests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brancz/locutus/render"
	"github.com/efficientgo/e2e"
	"github.com/efficientgo/tools/core/pkg/errcapture"
	"github.com/efficientgo/tools/core/pkg/testutil"
	"github.com/pkg/errors"
)

const locutusImage = "quay.io/brancz/locutus:latest"

// TestFileExampleRender tests locutus in docker rendering of examples in `/example/files` directory.
// This e2e tests require `make docker` first.
func TestFileExampleRender(t *testing.T) {
	e, err := e2e.NewDockerEnvironment("e2e_file")
	testutil.Ok(t, err)
	t.Cleanup(e.Close)

	f := e.Runnable("locutus").Future()
	testutil.Ok(t, copyDir("../example/files", filepath.Join(f.Dir(), "files")))

	locutus := f.Init(e2e.StartOptions{
		Image: locutusImage,
		User:  "1000",
		Command: e2e.NewCommand(
			"--log-level=none",
			"--render-only",
			"--trigger=oneoff",
			"--renderer=file",
			"--renderer.file.dir="+filepath.Join(f.InternalDir(), "files", "manifests"),
			"--renderer.file.rollout="+filepath.Join(f.InternalDir(), "files", "rollout.yaml"),
		),
	})

	out, err := locutus.RunOnce(context.TODO())
	testutil.Ok(t, err)

	outMap := objectsMap(t, out)
	testutil.Equals(t, 1, len(outMap))

	expectedJSON, err := ioutil.ReadFile("../example/files/manifests/grafana/deployment.json")
	testutil.Equals(t, compactJSON(t, expectedJSON), outMap["/grafana/deployment.json"])
}

func compactJSON(t testing.TB, in []byte) string {
	t.Helper()

	b := bytes.Buffer{}
	testutil.Ok(t, json.Compact(&b, in))
	return b.String()
}

func objectsMap(t testing.TB, rawRender string) map[string]string {
	t.Helper()

	var res render.Result
	testutil.Ok(t, json.NewDecoder(strings.NewReader(rawRender)).Decode(&res))

	ret := make(map[string]string, len(res.Objects))
	for k, v := range res.Objects {
		b := bytes.Buffer{}
		testutil.Ok(t, json.NewEncoder(&b).Encode(v))

		ret[k] = compactJSON(t, b.Bytes())
	}
	return ret
}

func copyDir(src, dst string) error {
	entries, err := ioutil.ReadDir(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, os.ModePerm); err != nil {
		return err
	}
	for _, entry := range entries {
		sourcePath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dst, entry.Name())

		fileInfo, err := os.Stat(sourcePath)
		if err != nil {
			return err
		}

		switch fileInfo.Mode() & os.ModeType {
		case os.ModeDir:
			if err := os.MkdirAll(destPath, os.ModePerm); err != nil {
				return err
			}
			if err := copyDir(sourcePath, destPath); err != nil {
				return err
			}
		case os.ModeSymlink:
			return errors.Errorf("symlinks are not implemented; got %v", sourcePath)
		default:
			if err := copyFile(sourcePath, destPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(src, dst string) (err error) {
	s, err := os.Stat(src)
	if err != nil {
		return err
	}

	if !s.Mode().IsRegular() {
		return errors.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer errcapture.Do(&err, source.Close, "src close")

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer errcapture.Do(&err, destination.Close, "dst close")
	_, err = io.Copy(destination, source)
	return err
}
