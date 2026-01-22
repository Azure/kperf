// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package runner

import (
	"context"
	"encoding/json"
	"time"

	"fmt"
	"os"
	"path/filepath"

	"github.com/Azure/kperf/api/types"
	"github.com/Azure/kperf/cmd/kperf/commands/utils"
	"github.com/Azure/kperf/metrics"
	"github.com/Azure/kperf/request"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/urfave/cli"
	"gopkg.in/yaml.v2"
)

// Command represents runner subcommand.
var Command = cli.Command{
	Name:  "runner",
	Usage: "Setup benchmark to kube-apiserver from one endpoint",
	Subcommands: []cli.Command{
		runCommand,
	},
}

var runCommand = cli.Command{
	Name:  "run",
	Usage: "run a benchmark test to kube-apiserver",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "kubeconfig",
			Usage: "Path to the kubeconfig file",
			Value: utils.DefaultKubeConfigPath,
		},
		cli.IntFlag{
			Name:  "client",
			Usage: "Total number of HTTP clients",
			Value: 1,
		},
		cli.StringFlag{
			Name:     "config",
			Usage:    "Path to the configuration file",
			Required: true,
		},
		cli.IntFlag{
			Name:  "conns",
			Usage: "Total number of connections. It can override corresponding value defined by --config",
			Value: 1,
		},
		cli.StringFlag{
			Name:  "content-type",
			Usage: fmt.Sprintf("Content type (%v or %v)", types.ContentTypeJSON, types.ContentTypeProtobuffer),
			Value: string(types.ContentTypeJSON),
		},
		cli.Float64Flag{
			Name:  "rate",
			Usage: "Maximum requests per second (Zero means no limitation). It can override corresponding value defined by --config",
		},
		cli.IntFlag{
			Name:  "total",
			Usage: "Total number of requests. It can override corresponding value defined by --config",
			Value: 1000,
		},
		cli.StringFlag{
			Name:  "user-agent",
			Usage: "User Agent",
		},
		cli.BoolFlag{
			Name:  "disable-http2",
			Usage: "Disable HTTP2 protocol",
		},
		cli.IntFlag{
			Name:  "max-retries",
			Usage: "Retry request after receiving 429 http code (<=0 means no retry)",
			Value: 0,
		},
		cli.StringFlag{
			Name:  "result",
			Usage: "Path to the file which stores results",
		},
		cli.BoolFlag{
			Name:  "raw-data",
			Usage: "show raw letencies data in result",
		},
		cli.IntFlag{
			Name:  "duration",
			Usage: "Duration of the benchmark in seconds. It will be ignored if --total is set.",
			Value: 0,
		},
	},
	Action: func(cliCtx *cli.Context) error {
		kubeCfgPath := cliCtx.String("kubeconfig")

		profileCfg, err := loadConfig(cliCtx)
		if err != nil {
			return err
		}

		// Check for multi-spec CLI override conflict
		if len(profileCfg.Specs) > 1 && hasCliOverrides(cliCtx) {
			return fmt.Errorf("CLI flag overrides are not allowed when config has multiple specs")
		}

		// Use first spec for client configuration (all specs share same client pool)
		firstSpec := profileCfg.Specs[0]
		clientNum := firstSpec.Conns
		restClis, err := request.NewClients(kubeCfgPath,
			clientNum,
			request.WithClientUserAgentOpt(cliCtx.String("user-agent")),
			request.WithClientQPSOpt(firstSpec.Rate),
			request.WithClientContentTypeOpt(firstSpec.ContentType),
			request.WithClientDisableHTTP2Opt(firstSpec.DisableHTTP2),
		)
		if err != nil {
			return err
		}

		var f *os.File = os.Stdout
		outputFilePath := cliCtx.String("result")
		if outputFilePath != "" {
			outputFileDir := filepath.Dir(outputFilePath)

			_, err = os.Stat(outputFileDir)
			if err != nil && os.IsNotExist(err) {
				err = os.MkdirAll(outputFileDir, 0750)
			}
			if err != nil {
				return fmt.Errorf("failed to ensure output's dir %s: %w", outputFileDir, err)
			}

			f, err = os.Create(outputFilePath)
			if err != nil {
				return err
			}
			defer f.Close()
		}

		rawDataFlagIncluded := cliCtx.Bool("raw-data")

		// Execute all specs (handles both single and multiple specs uniformly)
		perSpecResults, aggregated, err := executeSpecs(context.TODO(), profileCfg.Specs, restClis)
		if err != nil {
			return err
		}

		err = printMultiSpecResults(f, rawDataFlagIncluded, perSpecResults, aggregated)
		if err != nil {
			return fmt.Errorf("error while printing response stats: %w", err)
		}

		return nil
	},
}

// loadConfig loads and validates the config.
func loadConfig(cliCtx *cli.Context) (*types.LoadProfile, error) {
	var profileCfg types.LoadProfile

	cfgPath := cliCtx.String("config")

	cfgInRaw, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", cfgPath, err)
	}

	if err := yaml.Unmarshal(cfgInRaw, &profileCfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal %s from yaml format: %w", cfgPath, err)
	}

	// override value by flags
	if v := "rate"; cliCtx.IsSet(v) {
		profileCfg.Specs[0].Rate = cliCtx.Float64(v)
	}
	if v := "conns"; cliCtx.IsSet(v) || profileCfg.Specs[0].Conns == 0 {
		profileCfg.Specs[0].Conns = cliCtx.Int(v)
	}
	if v := "client"; cliCtx.IsSet(v) || profileCfg.Specs[0].Client == 0 {
		profileCfg.Specs[0].Client = cliCtx.Int(v)
	}
	if v := "total"; cliCtx.IsSet(v) {
		profileCfg.Specs[0].Total = cliCtx.Int(v)
	}
	if v := "duration"; cliCtx.IsSet(v) {
		profileCfg.Specs[0].Duration = cliCtx.Int(v)
	}
	if profileCfg.Specs[0].Total > 0 && profileCfg.Specs[0].Duration > 0 {
		klog.Warningf("both total:%v and duration:%v are set, duration will be ignored\n", profileCfg.Specs[0].Total, profileCfg.Specs[0].Duration)
		profileCfg.Specs[0].Duration = 0
	}
	if profileCfg.Specs[0].Total == 0 && profileCfg.Specs[0].Duration == 0 {
		// Use default total value
		profileCfg.Specs[0].Total = cliCtx.Int("total")
	}
	if v := "content-type"; cliCtx.IsSet(v) || profileCfg.Specs[0].ContentType == "" {
		profileCfg.Specs[0].ContentType = types.ContentType(cliCtx.String(v))
	}
	if v := "disable-http2"; cliCtx.IsSet(v) {
		profileCfg.Specs[0].DisableHTTP2 = cliCtx.Bool(v)
	}
	if v := "max-retries"; cliCtx.IsSet(v) {
		profileCfg.Specs[0].MaxRetries = cliCtx.Int(v)
	}

	if err := profileCfg.Validate(); err != nil {
		return nil, err
	}
	return &profileCfg, nil
}

// hasCliOverrides checks if any CLI override flags are set.
func hasCliOverrides(cliCtx *cli.Context) bool {
	overrideFlags := []string{"rate", "conns", "client", "total", "duration",
		"content-type", "disable-http2", "max-retries"}
	for _, flag := range overrideFlags {
		if cliCtx.IsSet(flag) {
			return true
		}
	}
	return false
}

// executeSpecs runs all specs sequentially and returns per-spec + aggregated results.
func executeSpecs(ctx context.Context, specs []types.LoadProfileSpec, restClis []rest.Interface) ([]*request.Result, *request.Result, error) {
	if len(specs) == 0 {
		return nil, nil, fmt.Errorf("no specs to execute")
	}

	results := make([]*request.Result, 0, len(specs))
	totalDuration := time.Duration(0)

	for i, spec := range specs {
		klog.V(2).Infof("Executing spec %d/%d", i+1, len(specs))

		result, err := request.Schedule(ctx, &spec, restClis)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to execute spec %d: %w", i+1, err)
		}

		results = append(results, result)
		totalDuration += result.Duration
	}

	aggregated := aggregateResults(results)
	aggregated.Duration = totalDuration

	return results, aggregated, nil
}

// aggregateResults combines multiple results into single aggregated result.
func aggregateResults(results []*request.Result) *request.Result {
	aggregated := &request.Result{
		ResponseStats: types.ResponseStats{
			Errors:             make([]types.ResponseError, 0),
			LatenciesByURL:     make(map[string][]float64),
			TotalReceivedBytes: 0,
		},
		Total: 0,
	}

	for _, result := range results {
		// Aggregate errors
		aggregated.Errors = append(aggregated.Errors, result.Errors...)

		// Aggregate latencies by URL
		for url, latencies := range result.LatenciesByURL {
			if _, exists := aggregated.LatenciesByURL[url]; !exists {
				aggregated.LatenciesByURL[url] = make([]float64, 0)
			}
			aggregated.LatenciesByURL[url] = append(aggregated.LatenciesByURL[url], latencies...)
		}

		// Sum bytes and requests
		aggregated.TotalReceivedBytes += result.TotalReceivedBytes
		aggregated.Total += result.Total
	}

	return aggregated
}

// printMultiSpecResults prints results for multiple specs with aggregated summary.
func printMultiSpecResults(f *os.File, rawDataFlagIncluded bool, perSpecResults []*request.Result, aggregated *request.Result) error {
	// Build per-spec reports
	perSpecReports := make([]types.RunnerMetricReport, 0, len(perSpecResults))
	for _, result := range perSpecResults {
		report := buildRunnerMetricReport(result, rawDataFlagIncluded)
		perSpecReports = append(perSpecReports, report)
	}

	// Build aggregated report
	aggregatedReport := buildRunnerMetricReport(aggregated, rawDataFlagIncluded)

	// Create multi-spec report
	multiReport := types.MultiSpecRunnerMetricReport{
		PerSpecResults: perSpecReports,
		Aggregated:     aggregatedReport,
	}

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")

	err := encoder.Encode(multiReport)
	if err != nil {
		return fmt.Errorf("failed to encode json: %w", err)
	}
	return nil
}

// buildRunnerMetricReport builds a RunnerMetricReport from request.Result.
func buildRunnerMetricReport(stats *request.Result, includeRawData bool) types.RunnerMetricReport {
	output := types.RunnerMetricReport{
		Total:              stats.Total,
		ErrorStats:         metrics.BuildErrorStatsGroupByType(stats.Errors),
		Duration:           stats.Duration.String(),
		TotalReceivedBytes: stats.TotalReceivedBytes,
		PercentileLatenciesByURL: map[string][][2]float64{},
	}

	total := 0
	for _, latencies := range stats.LatenciesByURL {
		total += len(latencies)
	}
	latencies := make([]float64, 0, total)
	for _, l := range stats.LatenciesByURL {
		latencies = append(latencies, l...)
	}
	output.PercentileLatencies = metrics.BuildPercentileLatencies(latencies)

	for u, l := range stats.LatenciesByURL {
		output.PercentileLatenciesByURL[u] = metrics.BuildPercentileLatencies(l)
	}

	if includeRawData {
		output.LatenciesByURL = stats.LatenciesByURL
		output.Errors = stats.Errors
	}

	return output
}
