package daemon

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Enry metrics
var (
	totalEnryCalls = promauto.NewCounter(prometheus.CounterOpts{
		Name: "bblfshd_enry_total",
		Help: "The total number of calls to Enry to detect the language",
	})
	enryLangResults = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "bblfshd_enry_langs",
		Help: "The total number of Enry results for each programming language",
	}, []string{"lang"})
	enryOtherResults  = enryLangResults.WithLabelValues("other")
	enryDetectLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "bblfshd_enry_seconds",
		Help: "Time spent for detecting the language (seconds)",
	})
)

var (
	driverLabelNames = []string{"lang", "image"}
)

// Driver Control API metrics
var (
	driverInstallCalls = promauto.NewCounter(prometheus.CounterOpts{
		Name: "bblfshd_driver_install_total",
		Help: "The total number of calls to install a driver",
	})
	driverRemoveCalls = promauto.NewCounter(prometheus.CounterOpts{
		Name: "bblfshd_driver_remove_total",
		Help: "The total number of calls to remove a driver",
	})
)

// Public API metrics
var (
	parseCalls = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "bblfshd_parse_total",
		Help: "The total number of parse requests",
	}, []string{"vers"})
	parseErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "bblfshd_parse_errors",
		Help: "The total number of failed parse requests",
	}, []string{"vers"})
	parseLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "bblfshd_parse_seconds",
		Help: "Time spent on parse requests (seconds)",
	}, []string{"vers"})
	parseContentSize = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "bblfshd_parse_bytes",
		Help: "Size of parsed files",
	}, []string{"vers"})

	versionCalls = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "bblfshd_version_total",
		Help: "The total number of version requests",
	}, []string{"vers"})
	languagesCalls = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "bblfshd_languages_total",
		Help: "The total number of supported languages requests",
	}, []string{"vers"})

	versionCallsV1     = versionCalls.WithLabelValues("v1")
	languagesCallsV1   = languagesCalls.WithLabelValues("v1")
	parseCallsV1       = parseCalls.WithLabelValues("v1")
	parseErrorsV1      = parseErrors.WithLabelValues("v1")
	parseLatencyV1     = parseLatency.WithLabelValues("v1")
	parseContentSizeV1 = parseContentSize.WithLabelValues("v1")

	versionCallsV2     = versionCalls.WithLabelValues("v2")
	languagesCallsV2   = languagesCalls.WithLabelValues("v2")
	parseCallsV2       = parseCalls.WithLabelValues("v2")
	parseErrorsV2      = parseErrors.WithLabelValues("v2")
	parseLatencyV2     = parseLatency.WithLabelValues("v2")
	parseContentSizeV2 = parseContentSize.WithLabelValues("v2")
)

// Scaling metrics
var (
	driversSpawned = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "bblfshd_driver_spawn",
		Help: "The total number of driver spawn requests",
	}, driverLabelNames)
	driversSpawnErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "bblfshd_driver_spawn_errors",
		Help: "The total number of errors for driver spawn requests",
	}, driverLabelNames)
	driversKilled = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "bblfshd_driver_kill",
		Help: "The total number of driver kill requests",
	}, driverLabelNames)

	driversRunning = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "bblfshd_driver_scaling_total",
		Help: "The total number of drivers running",
	}, driverLabelNames)
	driversIdle = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "bblfshd_driver_scaling_idle",
		Help: "The total number of idle drivers",
	}, driverLabelNames)
	driversRequests = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "bblfshd_driver_scaling_load",
		Help: "The total number of requests waiting for a driver",
	}, driverLabelNames)
	driversTarget = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "bblfshd_driver_scaling_target",
		Help: "The target number of drivers instances",
	}, driverLabelNames)
)
