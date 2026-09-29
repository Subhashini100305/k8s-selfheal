package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/aryausingh/k8s-selfheal/internal/metrics"
)

func main() {
	auditFile := flag.String(
		"audit-file",
		"",
		"path to audit records JSON array or JSONL file",
	)
	serve := flag.Bool(
		"serve",
		false,
		"serve Prometheus metrics at /metrics",
	)
	listenAddr := flag.String(
		"listen",
		":9091",
		"HTTP listen address when -serve is set",
	)

	flag.Parse()

	if *auditFile == "" {
		log.Fatal("missing required -audit-file path")
	}

	records, err := metrics.LoadAuditRecords(*auditFile)
	if err != nil {
		log.Fatal(err)
	}

	summary := metrics.Calculate(records)

	if *serve {
		handler, err := metrics.NewPrometheusHandler(
			summary,
			records,
		)
		if err != nil {
			log.Fatal(err)
		}

		mux := http.NewServeMux()
		mux.Handle("/metrics", handler)

		log.Printf(
			"serving Prometheus metrics on %s/metrics",
			*listenAddr,
		)

		log.Fatal(
			http.ListenAndServe(
				*listenAddr,
				mux,
			),
		)
	}

	output, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(output))
}
