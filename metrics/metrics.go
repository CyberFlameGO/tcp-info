// The metrics package defines prometheus metric types and provides
// convenience methods to add accounting to various parts of the pipeline.
//
// When defining new operations or metrics, these are helpful values to track:
//  - things coming into or go out of the system: requests, files, tests, api calls.
//  - the success or error status of any of the above.
//  - the distribution of processing latency.
package metrics

import (
	"fmt"
	"log"
	"net/http"
	"net/http/pprof"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func SetupPrometheus(promPort int) {
	if promPort <= 0 {
		log.Println("Not exporting prometheus metrics")
		return
	}

	// Define a custom serve mux for prometheus to listen on a separate port.
	// We listen on a separate port so we can forward this port on the host VM.
	// We cannot forward port 8080 because it is used by AppEngine.
	mux := http.NewServeMux()
	// Assign the default prometheus handler to the standard exporter path.
	mux.Handle("/metrics", promhttp.Handler())
	// Assign the pprof handling paths to the external port to access individual
	// instances.
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	prometheus.MustRegister(SyscallTimeMsec)

	prometheus.MustRegister(ConnectionCountHistogram)
	prometheus.MustRegister(CacheSizeHistogram)

	prometheus.MustRegister(NewFileCount)
	prometheus.MustRegister(ErrorCount)

	port := fmt.Sprintf(":%d", promPort)
	log.Println("Exporting prometheus metrics on", port)
	go http.ListenAndServe(port, mux)
}

var (
	// SyscallTimeMsec tracks the latency in the syscall.  It does NOT include
	// the time to process the netlink messages.
	SyscallTimeMsec = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "tcpinfo_syscall_time_msec",
			Help: "netlink syscall latency distribution",
			Buckets: []float64{
				1.0, 1.25, 1.6, 2.0, 2.5, 3.2, 4.0, 5.0, 6.3, 7.9,
				10, 12.5, 16, 20, 25, 32, 40, 50, 63, 79,
				100,
			},
		},
		[]string{"af"})

	// ConnectionCountHistogram tracks the number of connections returned by
	// each syscall.  This ??? includes local connections that are NOT recorded
	// in the cache or output.
	ConnectionCountHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "tcpinfo_connection_count_histogram",
			Help: "connection count histogram",
			Buckets: []float64{
				1, 2, 3, 4, 5, 6, 8,
				10, 12.5, 16, 20, 25, 32, 40, 50, 63, 79,
				100, 125, 160, 200, 250, 320, 400, 500, 630, 790,
				1000, 1250, 1600, 2000, 2500, 3200, 4000, 5000, 6300, 7900,
				10000, 12500, 16000, 20000, 25000, 32000, 40000, 50000, 63000, 79000,
				10000000,
			},
		},
		[]string{"af"})

	// CacheSizeHistogram tracks the number of entries in connection cache.
	CacheSizeHistogram = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name: "tcpinfo_cache_count_histogram",
			Help: "cache connection count histogram",
			Buckets: []float64{
				1, 2, 3, 4, 5, 6, 8,
				10, 12.5, 16, 20, 25, 32, 40, 50, 63, 79,
				100, 125, 160, 200, 250, 320, 400, 500, 630, 790,
				1000, 1250, 1600, 2000, 2500, 3200, 4000, 5000, 6300, 7900,
				10000, 12500, 16000, 20000, 25000, 32000, 40000, 50000, 63000, 79000,
				10000000,
			},
		})

	// ErrorCount measures the number of errors
	// Provides metrics:
	//    tcpinfo_Error_Count
	// Example usage:
	//    metrics.ErrorCount.With(prometheus.Labels{"type", "foobar"}).Inc()
	ErrorCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tcpinfo_error_count",
			Help: "The total number of errors encountered.",
		}, []string{"type"})

	// NewFileCount counts the number of connection files written.
	//
	// Provides metrics:
	//   tcpinfo_new_file_count
	// Example usage:
	//   metrics.FileCount.Inc()
	NewFileCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "tcpinfo_new_file_count",
			Help: "Number of files created.",
		},
	)
)
