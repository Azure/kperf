// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package replay

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Azure/kperf/api/types"
	"github.com/Azure/kperf/cmd/kperf/commands/utils"
	"github.com/Azure/kperf/metrics"
	"github.com/Azure/kperf/replay"

	"github.com/urfave/cli"
)

var runCommand = cli.Command{
	Name:  "run",
	Usage: "Run a replay test from a profile (local mode)",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "kubeconfig",
			Usage: "Path to the kubeconfig file",
			Value: utils.DefaultKubeConfigPath,
		},
		cli.StringFlag{
			Name:     "config",
			Usage:    "Path to the replay profile file (YAML, supports .yaml.gz for gzip compressed)",
			Required: true,
		},
		cli.StringFlag{
			Name:  "result",
			Usage: "Path to the file which stores results (JSON)",
		},
		cli.BoolFlag{
			Name:  "raw-data",
			Usage: "Include raw latency data in result",
		},
	},
	Action: func(cliCtx *cli.Context) error {
		kubeCfgPath := cliCtx.String("kubeconfig")
		configPath := cliCtx.String("config")

		// Load the replay profile
		profile, err := replay.LoadProfile(context.Background(), configPath)
		if err != nil {
			return fmt.Errorf("failed to load profile: %w", err)
		}

		fmt.Printf("Loaded replay profile: %s\n", profile.Description)
		fmt.Printf("  Total requests: %d\n", len(profile.Requests))
		fmt.Printf("  Duration: %dms\n", profile.Duration())
		fmt.Printf("  Runner count: %d\n", profile.Spec.RunnerCount)

		// Run the replay
		result, err := replay.Schedule(context.Background(), kubeCfgPath, profile)
		if err != nil {
			return fmt.Errorf("failed to run replay: %w", err)
		}

		// Print summary to stdout
		fmt.Printf("\nReplay completed:\n")
		fmt.Printf("  Total requests: %d\n", result.TotalRequests)
		fmt.Printf("  Requests run: %d\n", result.TotalRun)
		fmt.Printf("  Requests failed: %d\n", result.TotalFailed)
		fmt.Printf("  Duration: %s\n", result.Duration)
		fmt.Printf("  Bytes received: %d\n", result.Aggregated.TotalReceivedBytes)

		// Write result to file or stdout
		var f *os.File = os.Stdout
		outputFilePath := cliCtx.String("result")
		if outputFilePath != "" {
			outputFileDir := filepath.Dir(outputFilePath)

			if _, err := os.Stat(outputFileDir); os.IsNotExist(err) {
				if err := os.MkdirAll(outputFileDir, 0750); err != nil {
					return fmt.Errorf("failed to create output directory %s: %w", outputFileDir, err)
				}
			}

			f, err = os.Create(outputFilePath)
			if err != nil {
				return fmt.Errorf("failed to create result file: %w", err)
			}
			defer f.Close()
		}

		rawDataFlagIncluded := cliCtx.Bool("raw-data")

		// Build report
		report := buildReplayReport(result, rawDataFlagIncluded)

		// Write JSON
		encoder := json.NewEncoder(f)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			return fmt.Errorf("failed to encode result: %w", err)
		}

		return nil
	},
}

// ReplayReport is the output format for replay results.
type ReplayReport struct {
	types.RunnerMetricReport
	// RunnerCount is the number of runners used.
	RunnerCount int `json:"runnerCount"`
	// TotalRun is the number of requests actually executed.
	TotalRun int `json:"totalRun"`
	// TotalFailed is the number of requests that failed.
	TotalFailed int `json:"totalFailed"`
}

// buildReplayReport builds a ReplayReport from ScheduleResult.
func buildReplayReport(result *replay.ScheduleResult, includeRawData bool) ReplayReport {
	report := ReplayReport{
		RunnerMetricReport: types.RunnerMetricReport{
			Total:                    result.TotalRequests,
			ErrorStats:               metrics.BuildErrorStatsGroupByType(result.Aggregated.Errors),
			Duration:                 result.Duration.String(),
			TotalReceivedBytes:       result.Aggregated.TotalReceivedBytes,
			PercentileLatenciesByURL: map[string][][2]float64{},
		},
		RunnerCount: len(result.RunnerResults),
		TotalRun:    result.TotalRun,
		TotalFailed: result.TotalFailed,
	}

	metrics.BuildPercentileLatenciesReport(&report.RunnerMetricReport, result.Aggregated.LatenciesByURL, includeRawData, result.Aggregated.Errors)

	return report
}
