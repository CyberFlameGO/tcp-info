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
	"math"
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

	prometheus.MustRegister(FetchTimeMsecSummary)
	prometheus.MustRegister(ConnectionCountSummary)
	prometheus.MustRegister(CacheSizeSummary)

	prometheus.MustRegister(EntryFieldCountHistogram)
	prometheus.MustRegister(FileSizeHistogram)
	prometheus.MustRegister(RowSizeHistogram)

	// Common metrics
	prometheus.MustRegister(FileCount)
	prometheus.MustRegister(ErrorCount)
	prometheus.MustRegister(WarningCount)

	port := fmt.Sprintf(":%d", promPort)
	log.Println("Exporting prometheus metrics on", port)
	go http.ListenAndServe(port, mux)
}

var (
	// FetchTimeMsecSummary measures the latency (in msec) to fetch tcp-info records from kernel.
	// Provides metrics:
	//    tcpinfo_Fetch_Time_Msec_Summary
	// Example usage:
	//    metrics.FetchTimeMsecSummary.With(prometheus.Labels{"af": "ipv6"}).observe(float64)
	FetchTimeMsecSummary = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name: "tcpinfo_Fetch_Time_Msec_Summary",
		Help: "The total time to fetch tcp-info records, in milliseconds.",
	}, []string{"af"})

	// ConnectionCountSummary the (total) number of TCP connections collected, by type.
	// Provides metrics:
	//    tcpinfo_Connection_Count_Summary
	// Example usage:
	//    metrics.ConnectionCountSummary.With(prometheus.Labels{"af": "ipv6"}).observe(float64)
	ConnectionCountSummary = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name: "tcpinfo_Connection_Count_Summary",
		Help: "The (total) number of TCP connections collected, by type.",
	}, []string{"af"})

	// CacheSizeSummary measures the size of the connection cache.
	// Provides metrics:
	//    tcpinfo_Cache_Size_Summary
	// Example usage:
	//    metrics.CacheSizeSummary.observe()
	CacheSizeSummary = prometheus.NewSummary(prometheus.SummaryOpts{
		Name: "tcpinfo_Connection_Cache_Size_Summary",
		Help: "The number of entries in the connection cache.",
	})

	// ErrorCount measures the number of annotation errors
	// Provides metrics:
	//    tcpinfo_Error_Count
	// Example usage:
	//    metrics.ErrorCount.With(prometheus.Labels{"source", "foobar"}).Inc()
	ErrorCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tcpinfo_Error_Count",
			Help: "The total number of errors encountered.",
		}, []string{"source"})

	// WarningCount measures the number of annotation warnings
	// Provides metrics:
	//    tcpinfo_Warning_Count
	// Example usage:
	//    metrics.WarningCount.With(prometheus.Labels{"source", "foobar"}).Inc()
	WarningCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tcpinfo_Warning_Count",
			Help: "The total number of Warnings encountered.",
		}, []string{"source"})

	// FileCount counts the number of files written.
	//
	// Provides metrics:
	//   tcpinfo_New_File_Count
	// Example usage:
	//   metrics.FileCount.Inc()
	FileCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "tcpinfo_New_File_Count",
			Help: "Number of files created.",
		},
	)

	// TODO(dev): bytes/row - generalize this metric for any file type.
	//
	// RowSizeHistogram provides a histogram of bq row json sizes.  It is intended primarily for
	// NDT, so the bins are fairly large.  NDT average json is around 200K
	//
	// Provides metrics:
	//   etl_row_json_size_bucket{table="...", le="..."}
	//   ...
	//   etl_row_json_size_sum{table="...", le="..."}
	//   etl_row_json_size_count{table="...", le="..."}
	// Usage example:
	//   metrics.RowSizeHistogram.WithLabelValues(
	//           "ndt").Observe(len(json))
	RowSizeHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "etl_row_json_size",
			Help: "Row json size distributions.",
			Buckets: []float64{
				0,
				100, 130, 180, 240, 320, 420, 560, 750,
				1000, 1300, 1800, 2400, 3200, 4200, 5600, 7500,
				10000, 13000, 18000, 24000, 32000, 42000, 56000, 75000,
				100000, 130000, 180000, 240000, 320000, 420000, 560000, 750000,
				1000000, 1300000, 1800000, 2400000, 3200000, 4200000, 5600000, 7500000,
				10000000, // 10MiB
				math.Inf(+1),
			},
		},
		[]string{"table"},
	)

	// TODO(dev): rows/test - generalize this metric for any file type.
	//
	// EntryFieldCountHistogram provides a histogram of (approximate) row field counts.  It is intended primarily for
	// NDT, so the bins are fairly large.  NDT snapshots typically total about 10k
	// fields, 99th percentile around 35k fields, and occasionally as many as 50k.
	// Smaller field count bins included so that it is possibly useful for other
	// parsers.
	//
	// Provides metrics:
	//   etl_entry_field_count_bucket{table="...", le="..."}
	//   ...
	//   etl_entry_field_count_sum{table="...", le="..."}
	//   etl_entry_field_count_count{table="...", le="..."}
	// Usage example:
	//   metrics.EntryFieldCountHistogram.WithLabelValues(
	//           "ndt").Observe(fieldCount)
	EntryFieldCountHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "etl_entry_field_count",
			Help: "total snapshot field count distributions.",
			Buckets: []float64{
				0,
				10, 13, 18, 24, 32, 42, 56, 75,
				100, 130, 180, 240, 320, 420, 560, 750,
				1000, 1300, 1800, 2400, 3200, 4200, 5600, 7500,
				10000, 13000, 18000, 24000, 32000, 42000, 56000, 75000,
				100000, 130000, 180000, 240000, 320000, 420000, 560000, 750000,
				1000000, // 1 MiB
				math.Inf(+1),
			},
		},
		[]string{"table"},
	)

	// FileSizeHistogram provides a histogram of source file sizes. The bucket
	// sizes should cover a wide range of input file sizes, but should not have too
	// many buckets, because there are also three vector dimensions.
	//
	// Example usage:
	//   metrics.FileSizeHistogram.WithLabelValues(
	//       "ndt", "c2s_snaplog", "parsed").Observe(size)
	FileSizeHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "etl_test_file_size_bytes",
			Help: "Size of individual test files.",
			Buckets: []float64{
				0,
				1000, 2500, 5000, 10000, 25000, 50000,
				100000, 250000, 500000, 1000000, 2500000, 5000000,
				10000000, 25000000, 50000000, 100000000, 250000000, 500000000,
				1000000000, // 1 gb
				math.Inf(+1),
			},
		},
		[]string{"table", "kind", "group"},
	)
)
