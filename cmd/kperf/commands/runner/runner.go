// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package runner

import (
	"context"
	"encoding/json"

	"fmt"
	"os"
	"path/filepath"

	"github.com/Azure/kperf/api/types"
	"github.com/Azure/kperf/cmd/kperf/commands/utils"
	"github.com/Azure/kperf/metrics"
	"github.com/Azure/kperf/request"

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
	},
	Action: func(cliCtx *cli.Context) error {
		kubeCfgPath := cliCtx.String("kubeconfig")

		profileCfg, err := loadConfig(cliCtx)
		if err != nil {
			return err
		}

		clientNum := profileCfg.Spec.Conns
		restClis, err := request.NewClients(kubeCfgPath,
			clientNum,
			request.WithClientUserAgentOpt(cliCtx.String("user-agent")),
			request.WithClientQPSOpt(profileCfg.Spec.Rate),
			request.WithClientContentTypeOpt(profileCfg.Spec.ContentType),
			request.WithClientDisableHTTP2Opt(profileCfg.Spec.DisableHTTP2),
		)
		if err != nil {
			return err
		}

		stats, err := request.Schedule(context.TODO(), &profileCfg.Spec, restClis)
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
		err = printResponseStats(f, rawDataFlagIncluded, stats)
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
		profileCfg.Spec.Rate = cliCtx.Float64(v)
	}
	if v := "conns"; cliCtx.IsSet(v) || profileCfg.Spec.Conns == 0 {
		profileCfg.Spec.Conns = cliCtx.Int(v)
	}
	if v := "client"; cliCtx.IsSet(v) || profileCfg.Spec.Client == 0 {
		profileCfg.Spec.Client = cliCtx.Int(v)
	}
	if v := "total"; cliCtx.IsSet(v) || profileCfg.Spec.Total == 0 {
		profileCfg.Spec.Total = cliCtx.Int(v)
	}
	if v := "content-type"; cliCtx.IsSet(v) || profileCfg.Spec.ContentType == "" {
		profileCfg.Spec.ContentType = types.ContentType(cliCtx.String(v))
	}
	if v := "disable-http2"; cliCtx.IsSet(v) {
		profileCfg.Spec.DisableHTTP2 = cliCtx.Bool(v)
	}
	if v := "max-retries"; cliCtx.IsSet(v) {
		profileCfg.Spec.MaxRetries = cliCtx.Int(v)
	}

	if err := profileCfg.Validate(); err != nil {
		return nil, err
	}
	return &profileCfg, nil
}

// printResponseStats prints types.RunnerMetricReport into underlying file.
func printResponseStats(f *os.File, rawDataFlagIncluded bool, stats *request.Result) error {
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

	if rawDataFlagIncluded {
		output.LatenciesByURL = stats.LatenciesByURL
		output.Errors = stats.Errors
	}

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")

	err := encoder.Encode(output)
	if err != nil {
		return fmt.Errorf("failed to encode json: %w", err)
	}
	return nil
}
