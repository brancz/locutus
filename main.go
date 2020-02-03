package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"net/http/pprof"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/brancz/locutus/client"
	"github.com/brancz/locutus/config"
	"github.com/brancz/locutus/render"
	"github.com/brancz/locutus/rollout"
	"github.com/brancz/locutus/rollout/checks"
	"github.com/brancz/locutus/trigger"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/oklog/run"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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
		triggerProviderName string
		configFile          string
		renderOnly          bool
	)

	renderers := render.Providers()
	triggers := trigger.Providers()

	s := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	renderers.RegisterFlags(s)
	triggers.RegisterFlags(s)
	s.StringVar(&logLevel, "log-level", logLevelInfo, "Log level to use.")
	s.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	s.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	s.StringVar(&renderProviderName, "renderer", "", "The provider to use for rendering manifests.")
	s.StringVar(&triggerProviderName, "trigger", "", "The provider to use to trigger reconciling.")
	s.StringVar(&configFile, "config-file", "", "The config file whose content to pass to the render provider.")
	s.BoolVar(&renderOnly, "render-only", false, "Only render manifests to be rolled out and print to STDOUT.")

	s.Parse(os.Args[1:])

	logger, err := logger(logLevel)
	if err != nil {
		logger.Log("msg", "error creating logger", err)
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector())
	reg.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		logger.Log("msg", "error building kubeconfig", "err", err)
	}

	kclient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		logger.Log("msg", "error building kubernetes clientset", "err", err)
	}

	cl := client.NewClient(log.With(logger, "component", "client"), cfg, kclient)
	if err != nil {
		logger.Log("msg", "failed to instantiate client", "err", err)
		return 1
	}
	cl.SetUpdatePreparations(client.DefaultUpdatePreparations)

	renderProvider, err := renderers.Select(renderProviderName)
	if err != nil {
		logger.Log("msg", "failed to find render provider", "err", err)
		return 1
	}

	triggerProvider, err := triggers.Select(triggerProviderName)
	if err != nil {
		logger.Log("msg", "failed to find trigger provider", "err", err)
		return 1
	}

	trigger, err := triggerProvider.NewTrigger(logger, cl)
	if err != nil {
		logger.Log("msg", "failed to create trigger", "err", err)
		return 1
	}

	c := checks.NewSuccessChecks(logger, cl)
	renderer := renderProvider.NewRenderer(logger)
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

	g := run.Group{}
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

	ctx, cancel := context.WithCancel(context.Background())
	g.Add(func() error {
		return errors.Wrap(trigger.Run(ctx.Done()), "failed to run trigger")
	}, func(err error) {
		cancel()
	})

	term := make(chan os.Signal)
	g.Add(func() error {
		signal.Notify(term, os.Interrupt, syscall.SIGTERM)

		select {
		case <-term:
			return nil
		}

		return nil
	}, func(err error) {
		close(term)
	})

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
