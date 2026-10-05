package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/aryausingh/k8s-selfheal/internal/metrics"
)

func main() {
	auditFile := flag.String(
		"audit-file",
		"",
		"path to audit records JSON array or JSONL file",
	)
	runDir := flag.String(
		"run-dir",
		"",
		"path to one run archive directory containing meta.json and audit.jsonl",
	)
	runsDir := flag.String(
		"runs-dir",
		"",
		"path to directory containing per-run archive directories",
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

	if *auditFile == "" && *runDir == "" && *runsDir == "" {
		log.Fatal("missing required input: use -audit-file, -run-dir, or -runs-dir")
	}

	var records []metrics.AuditRecord
	var metas []metrics.RunMeta
	if *runDir != "" || *runsDir != "" {
		if *runDir != "" && *runsDir != "" {
			log.Fatal("use only one of -run-dir or -runs-dir")
		}
		archives, err := loadRunArchives(*runDir, *runsDir)
		if err != nil {
			log.Fatal(err)
		}
		for _, archive := range archives {
			metas = append(metas, archive.Meta)
			records = append(records, archive.Records...)
		}
	} else {
		var err error
		records, err = metrics.LoadAuditRecords(*auditFile)
		if err != nil {
			log.Fatal(err)
		}
	}

	summary := metrics.Calculate(records)
	if len(metas) > 0 {
		var err error
		summary.ExperimentRecovery, err = metrics.CalculateExperimentRecovery(
			metas,
		)
		if err != nil {
			log.Fatal(err)
		}
	}

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

func loadRunArchives(runDir string, runsDir string) ([]metrics.RunArchive, error) {
	if runDir != "" {
		archive, err := metrics.LoadRunArchive(runDir)
		if err != nil {
			return nil, err
		}
		return []metrics.RunArchive{archive}, nil
	}

	entries, err := os.ReadDir(runsDir)
	if err != nil {
		return nil, fmt.Errorf("read runs directory %q: %w", runsDir, err)
	}
	var archives []metrics.RunArchive
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		archive, err := metrics.LoadRunArchive(filepath.Join(runsDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		archives = append(archives, archive)
	}
	return archives, nil
}
