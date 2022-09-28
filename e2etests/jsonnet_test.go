package e2etests

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/efficientgo/e2e"
	"github.com/efficientgo/tools/core/pkg/testutil"
)

// TestJsonnetExamplesRender tests locutus in docker rendering of examples in `/example/jsonnet` directory.
// This e2e tests require `make docker` first.
func TestJsonnetExamplesRender(t *testing.T) {
	e, err := e2e.NewDockerEnvironment("e2e_jsonnet")
	testutil.Ok(t, err)
	t.Cleanup(e.Close)

	t.Run("jsonnet", func(t *testing.T) {
		f := e.Runnable("locutus").Future()
		testutil.Ok(t, copyDir("../example/jsonnet", filepath.Join(f.Dir(), "jsonnet")))

		locutus := f.Init(e2e.StartOptions{
			Image: locutusImage,
			User:  "1000",
			Command: e2e.NewCommand(
				"--log-level=none",
				"--render-only",
				"--trigger=oneoff",
				"--renderer=jsonnet",
				"--renderer.jsonnet.entrypoint="+filepath.Join(f.InternalDir(), "jsonnet", "main.jsonnet"),
			),
		})

		out, err := locutus.RunOnce(context.TODO())
		testutil.Ok(t, err)

		outMap := objectsMap(t, out)
		testutil.Equals(t, 1, len(outMap))

		expectedJSON, err := ioutil.ReadFile("../example/files/manifests/grafana/deployment.json")
		testutil.Equals(t, compactJSON(t, expectedJSON), outMap["deployment"])
	})

	t.Run("jsonnet-with-config", func(t *testing.T) {
		f := e.Runnable("locutus2").Future()
		testutil.Ok(t, copyDir("../example/jsonnet-with-config", filepath.Join(f.Dir(), "jsonnet")))

		locutus := f.Init(e2e.StartOptions{
			Image: locutusImage,
			User:  "1000",
			Command: e2e.NewCommand(
				"--log-level=none",
				"--render-only",
				"--trigger=oneoff",
				"--renderer=jsonnet",
				"--renderer.jsonnet.entrypoint="+filepath.Join(f.InternalDir(), "jsonnet", "main.jsonnet"),
				"--config-file="+filepath.Join(f.InternalDir(), "jsonnet", "config.json"),
			),
		})

		out, err := locutus.RunOnce(context.TODO())
		testutil.Ok(t, err)

		outMap := objectsMap(t, out)
		testutil.Equals(t, 1, len(outMap))

		expectedJSON, err := ioutil.ReadFile("../example/files/manifests/grafana/deployment.json")
		// Apply overrides:
		expectedJSONWithOverride := strings.Replace(
			compactJSON(t, expectedJSON),
			`},"name":"grafana","namespace":"default"`,
			`},"name":"overridden-name","namespace":"overridden-namespace"`,
			-1,
		)

		testutil.Equals(t, expectedJSONWithOverride, outMap["deployment"])
	})
}
