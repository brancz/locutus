package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"strings"
	"time"

	"github.com/brancz/locutus/client"
	"github.com/brancz/locutus/config"
	"github.com/brancz/locutus/db"
	"github.com/brancz/locutus/render/file"
	"github.com/brancz/locutus/render/jsonnet"
	"github.com/brancz/locutus/rollout"
	"github.com/brancz/locutus/rollout/checks"
	"github.com/brancz/locutus/source"
	"github.com/brancz/locutus/trigger"
	"github.com/brancz/locutus/trigger/database"
	"github.com/brancz/locutus/trigger/interval"
	"github.com/brancz/locutus/trigger/oneoff"
	"github.com/brancz/locutus/trigger/resource"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/oklog/run"
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

type stringList []string

func (l *stringList) String() string {
	return strings.Join(*l, ",")
}

func (l *stringList) Set(value string) error {
	for _, s := range strings.Split(value, ",") {
		*l = append(*l, s)
	}
	return nil
}

func Main() int {
	var (
		logLevel           string
		masterURL          string
		kubeconfig         string
		renderProviderName string
		writeStatus        bool
		configFile         string
		renderOnly         bool
		oneOff             bool

		rendererFileDirectory     string
		rendererFileRollout       string
		rendererJsonnetJpaths     stringList
		rendererJsonnetEntrypoint string
		rendererJsonnetExtStrs    stringList

		databaseConnectionsFile string

		sourceDatabaseFile string

		triggerIntervalDuration time.Duration
		triggerResourceConfig   string
		triggerDatabaseConfig   string
	)

	s := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	s.StringVar(&logLevel, "log-level", logLevelInfo, "Log level to use.")
	s.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	s.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	s.StringVar(&renderProviderName, "renderer", "", "The provider to use for rendering manifests.")
	s.StringVar(&configFile, "config-file", "", "The config file whose content to pass to the render provider.")
	s.BoolVar(&renderOnly, "render-only", false, "Only render manifests to be rolled out and print to STDOUT.")
	s.BoolVar(&oneOff, "one-off", false, "Only render and rollout once, then exit.")
	s.StringVar(&databaseConnectionsFile, "database-connections-file", "", "File to read database connections from.")

	s.StringVar(&rendererFileDirectory, "renderer.file.dir", "manifests/", "Directory to read files from.")
	s.StringVar(&rendererFileRollout, "renderer.file.rollout", "rollout.yaml", "Plain rollout spec to read.")
	s.StringVar(&rendererJsonnetEntrypoint, "renderer.jsonnet.entrypoint", "jsonnet/main.jsonnet", "Jsonnet file to execute to render.")
	s.Var(&rendererJsonnetExtStrs, "renderer.jsonnet.ext-str", "Jsonnet ext-str to pass to the jsonnet VM.")
	s.Var(&rendererJsonnetJpaths, "renderer.jsonnet.jpaths", "Jsonnet jpaths to pass to the jsonnet VM.")

	s.StringVar(&sourceDatabaseFile, "source.database.file", "", "File to read database queries from as sources.")

	s.DurationVar(&triggerIntervalDuration, "trigger.interval.duration", time.Duration(0), "Duration of interval in which to trigger.")
	s.StringVar(&triggerResourceConfig, "trigger.resource.config", "", "Path to configuration of resource triggers.")
	s.StringVar(&triggerDatabaseConfig, "trigger.database.config", "", "Path to configuration of database triggers.")
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
		cl.SetUpdateChecks(client.DefaultUpdateChecks)
	}

	ctx := context.Background()
	sources := map[string]func(context.Context) ([]byte, error){}
	triggers := []trigger.Trigger{}

	if triggerResourceConfig != "" {
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

		triggers = append(triggers, t)
	}

	if triggerIntervalDuration > 0 {
		triggers = append(triggers, interval.NewTrigger(logger, triggerIntervalDuration))
	}

	var databaseConnections *db.Connections
	if databaseConnectionsFile != "" {
		databaseConnections, err = db.FromFile(ctx, reg, databaseConnectionsFile)
		if err != nil {
			logger.Log("msg", "failed to read database connections", "err", err)
			return 1
		}

		if sourceDatabaseFile != "" {
			s, err := source.NewDatabaseSources(
				logger,
				databaseConnections,
				sourceDatabaseFile,
			)
			if err != nil {
				logger.Log("msg", "failed to create cockroachdb source", "err", err)
				return 1
			}

			sources, err := s.InputSources()
			if err != nil {
				logger.Log("msg", "failed to create cockroachdb source", "err", err)
				return 1
			}

			for name, sourceFunc := range sources {
				level.Debug(logger).Log("msg", "adding dynamic import", "source", name)
				sources[name] = sourceFunc
			}
		}
	}

	if triggerDatabaseConfig != "" {
		t, err := database.NewTrigger(
			logger,
			databaseConnections,
			triggerDatabaseConfig,
		)
		if err != nil {
			logger.Log("msg", "failed to create resource trigger", "err", err)
			return 1
		}

		triggers = append(triggers, t)
	}

	if oneOff {
		triggers = []trigger.Trigger{oneoff.NewTrigger(logger)}
	}

	if len(triggers) == 0 {
		logger.Log("msg", "no triggers configured")
		return 1
	}

	var renderer rollout.Renderer
	{
		switch renderProviderName {
		case "jsonnet":
			renderer, err = jsonnet.NewRenderer(
				logger,
				rendererJsonnetEntrypoint,
				sources,
				[]string(rendererJsonnetExtStrs),
				[]string(rendererJsonnetJpaths),
			)
			if err != nil {
				logger.Log("msg", "failed to create jsonnet renderer", "err", err)
				return 1
			}
		case "file":
			renderer = file.NewRenderer(logger, rendererFileDirectory, rendererFileRollout)
		default:
			logger.Log("msg", "failed to find render provider")
			return 1
		}
	}

	c, err := checks.NewChecks(logger, cl, databaseConnections, checks.DefaultChecks)
	if err != nil {
		logger.Log("msg", "failed to create checks", "err", err)
		return 1
	}
	runner := rollout.NewRunner(reg, log.With(logger, "component", "rollout-runner"), cl, renderer, c, renderOnly)
	runner.SetObjectActions(rollout.DefaultObjectActions)

	for _, trigger := range triggers {
		trigger.Register(config.NewFileConfigPasser(configFile, runner))
	}

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
	for _, trigger := range triggers {
		ctx, cancel := context.WithCancel(ctx)
		g.Add(func() error {
			level.Info(logger).Log("msg", "starting trigger...", "trigger", fmt.Sprintf("%T", trigger))

			if err := trigger.Run(ctx); err != nil {
				return fmt.Errorf("failed to run trigger: %w", err)
			}

			return nil
		}, func(err error) {
			cancel()
		})
	}
	{
		g.Add(run.SignalHandler(context.Background(), os.Interrupt))
	}

	level.Info(logger).Log("msg", "running", "renderer", renderProviderName)

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
