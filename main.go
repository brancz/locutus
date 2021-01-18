package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"time"

	"github.com/brancz/locutus/client"
	"github.com/brancz/locutus/config"
	"github.com/brancz/locutus/render/file"
	"github.com/brancz/locutus/render/jsonnet"
	"github.com/brancz/locutus/rollout"
	"github.com/brancz/locutus/rollout/checks"
	"github.com/brancz/locutus/trigger"
	"github.com/brancz/locutus/trigger/interval"
	"github.com/brancz/locutus/trigger/oneoff"
	"github.com/brancz/locutus/trigger/resource"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/oklog/run"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	logLevelAll   = "all"
	logLevelDebug = "debug"
	logLevelInfo  = "info"
	logLevelWarn  = "warn"
	logLevelError = "error"
	logLevelNone  = "none"
)

var (
	availableLogLevels = []string{
		logLevelAll,
		logLevelDebug,
		logLevelInfo,
		logLevelWarn,
		logLevelError,
		logLevelNone,
	}
)

func Main() int {
	var (
		logLevel            string
		masterURL           string
		kubeconfig          string
		renderProviderName  string
		writeStatus         bool
		triggerProviderName string
		configFile          string
		renderOnly          bool

		rendererFileDirectory     string
		rendererFileRollout       string
		rendererJsonnetEntrypoint string

		triggerIntervalDuration time.Duration
		triggerResourceConfig   string
	)

	s := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	s.StringVar(&logLevel, "log-level", logLevelInfo, "Log level to use.")
	s.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	s.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	s.StringVar(&renderProviderName, "renderer", "", "The provider to use for rendering manifests.")
	s.StringVar(&triggerProviderName, "trigger", "", "The provider to use to trigger reconciling.")
	s.StringVar(&configFile, "config-file", "", "The config file whose content to pass to the render provider.")
	s.BoolVar(&renderOnly, "render-only", false, "Only render manifests to be rolled out and print to STDOUT.")

	s.StringVar(&rendererFileDirectory, "renderer.file.dir", "manifests/", "Directory to read files from.")
	s.StringVar(&rendererFileRollout, "renderer.file.rollout", "rollout.yaml", "Plain rollout spec to read.")
	s.StringVar(&rendererJsonnetEntrypoint, "renderer.jsonnet.entrypoint", "jsonnet/main.jsonnet", "Jsonnet file to execute to render.")

	s.DurationVar(&triggerIntervalDuration, "trigger.interval.duration", time.Minute, "Duration of interval in which to trigger.")
	s.StringVar(&triggerResourceConfig, "trigger.resource.config", "", "Path to configuration of resource triggers.")
	s.BoolVar(&writeStatus, "trigger.resource.write-status", true, "Whether to write status back to the originating resource.")

	if err := s.Parse(os.Args[1:]); err != nil {
		return 1
	}

	logger, err := logger(logLevel)
	if err != nil {
		fmt.Println(err)
		return 1
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector())
	reg.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))

	var cl *client.Client
	{
		konfig, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
		if err != nil {
			logger.Log("msg", "error building kubeconfig", "err", err)
			return 1
		}

		klient, err := kubernetes.NewForConfig(konfig)
		if err != nil {
			logger.Log("msg", "error building kubernetes clientset", "err", err)
			return 1
		}

		cl = client.NewClient(konfig, klient)
		cl.WithLogger(log.With(logger, "component", "client"))
		cl.SetUpdatePreparations(client.DefaultUpdatePreparations)
	}

	sources := map[string]func() ([]byte, error){}

	ctx := context.Background()
	var trigger trigger.Trigger
	{
		switch triggerProviderName {
		case "interval":
			trigger = interval.NewTrigger(logger, triggerIntervalDuration)
		case "oneoff":
			trigger = oneoff.NewTrigger(logger)
		case "resource":
			t, err := resource.NewTrigger(ctx, logger, cl, triggerResourceConfig, writeStatus)
			if err != nil {
				logger.Log("msg", "failed to create resource trigger", "err", err)
				return 1
			}

			triggerSources := t.InputSources()
			for name, sourceFunc := range triggerSources {
				level.Debug(logger).Log("msg", "adding dynamic import", "source", name)
				sources[name] = sourceFunc
			}

			trigger = t
		default:
			logger.Log("msg", "failed to find trigger provider")
			return 1
		}
	}

	var renderer rollout.Renderer
	{
		switch renderProviderName {
		case "jsonnet":
			renderer = jsonnet.NewRenderer(logger, rendererJsonnetEntrypoint, sources)
		case "file":
			renderer = file.NewRenderer(logger, rendererFileDirectory, rendererFileRollout)
		default:
			logger.Log("msg", "failed to find render provider")
			return 1
		}
	}

	c := checks.NewSuccessChecks(logger, cl)
	runner := rollout.NewRunner(reg, log.With(logger, "component", "rollout-runner"), cl, renderer, c, renderOnly)
	runner.SetObjectActions(rollout.DefaultObjectActions)

	trigger.Register(config.NewConfigPasser(configFile, runner))

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.InstrumentMetricHandler(reg, promhttp.HandlerFor(reg, promhttp.HandlerOpts{})))
	mux.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
	mux.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	mux.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	mux.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	mux.Handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))

	srv := &http.Server{Handler: mux}

	var g run.Group
	{
		l, err := net.Listen("tcp", ":8080")
		if err != nil {
			logger.Log("msg", "listening port 8080 failed", "err", err)
			return 1
		}
		g.Add(func() error {
			return srv.Serve(l)
		}, func(err error) {
			l.Close()
		})
	}
	{
		ctx, cancel := context.WithCancel(ctx)
		g.Add(func() error {
			return errors.Wrap(trigger.Run(ctx), "failed to run trigger")
		}, func(err error) {
			cancel()
		})
	}
	{
		g.Add(run.SignalHandler(context.Background(), os.Interrupt))
	}

	level.Info(logger).Log("msg", "running", "renderer", renderProviderName, "trigger", triggerProviderName)

	if err := g.Run(); err != nil {
		logger.Log("msg", "Unhandled error received. Exiting...", "err", err)
		return 1
	}

	return 0
}

func logger(logLevel string) (log.Logger, error) {
	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout))
	switch logLevel {
	case logLevelAll:
		logger = level.NewFilter(logger, level.AllowAll())
	case logLevelDebug:
		logger = level.NewFilter(logger, level.AllowDebug())
	case logLevelInfo:
		logger = level.NewFilter(logger, level.AllowInfo())
	case logLevelWarn:
		logger = level.NewFilter(logger, level.AllowWarn())
	case logLevelError:
		logger = level.NewFilter(logger, level.AllowError())
	case logLevelNone:
		logger = level.NewFilter(logger, level.AllowNone())
	default:
		return nil, fmt.Errorf("log level %v unknown, %v are possible values", logLevel, availableLogLevels)
	}
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	logger = log.With(logger, "caller", log.DefaultCaller)

	return logger, nil
}

func main() {
	os.Exit(Main())
}
