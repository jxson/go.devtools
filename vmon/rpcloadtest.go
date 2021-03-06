// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/api/cloudmonitoring/v2beta2"

	"v.io/jiri/tool"
	"v.io/v23/context"
	"v.io/x/devtools/internal/monitoring"
	"v.io/x/devtools/internal/test"
)

// checkRPCLoadTest checks the result of RPC load test and sends the result to GCM.
func checkRPCLoadTest(v23ctx *context.T, ctx *tool.Context, s *cloudmonitoring.Service) error {
	// Parse result file.
	seq := ctx.NewSeq()
	resultFile := filepath.Join(os.Getenv("WORKSPACE"), "load_stats.json")
	bytes, err := seq.ReadFile(resultFile)
	if err != nil {
		return err
	}
	var results struct {
		MsecPerRpc float64
		Qps        float64
	}
	if err := json.Unmarshal(bytes, &results); err != nil {
		return nil
	}

	// Send to GCM.
	items := map[string]float64{
		"latency": results.MsecPerRpc,
		"qps":     results.Qps,
	}
	mdRpcLoadTest := monitoring.CustomMetricDescriptors["rpc-load-test"]
	fi, err := seq.Stat(resultFile)
	if err != nil {
		return err
	}
	timeStr := fi.ModTime().Format(time.RFC3339)
	for label, value := range items {
		_, err = s.Timeseries.Write(projectFlag, &cloudmonitoring.WriteTimeseriesRequest{
			Timeseries: []*cloudmonitoring.TimeseriesPoint{
				&cloudmonitoring.TimeseriesPoint{
					Point: &cloudmonitoring.Point{
						DoubleValue: value,
						Start:       timeStr,
						End:         timeStr,
					},
					TimeseriesDesc: &cloudmonitoring.TimeseriesDescriptor{
						Metric: mdRpcLoadTest.Name,
						Labels: map[string]string{
							mdRpcLoadTest.Labels[0].Key: label,
						},
					},
				},
			},
		}).Do()
		if err != nil {
			test.Fail(ctx, "%s: %f\n", label, value)
			return fmt.Errorf("Timeseries Write failed: %v", err)
		}
		test.Pass(ctx, "%s: %f\n", label, value)
	}
	return nil
}
