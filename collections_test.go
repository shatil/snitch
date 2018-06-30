package snitch

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
)

// TestToMetricData verifies conversion of collected resource counts to
// CloudWatch data points.
func TestToMetricData(t *testing.T) {
	beforeTimestamp := time.Now()
	expectedNumberOfDimensions := 2
	expectedInstanceType := "my5.InstanceType"
	expected := NewClusterResources(aws.String("my-shiny-cluster"))
	expectedRegisteredSchedulable := 13
	expectedRemainingSchedulable := 3
	expectedCPU := 1024
	expectedMemory := 2048
	expected.CPU[expectedInstanceType] += expectedCPU
	expected.Memory[expectedInstanceType] += expectedMemory
	expected.Registered[expectedInstanceType] += expectedRegisteredSchedulable
	expected.Remaining[expectedInstanceType] += expectedRemainingSchedulable
	metricData := expected.ToMetricData()
	for _, datum := range metricData {
		switch *datum.MetricName {
		case "LowestCommonMultipleCPU":
			if expectedCPU != int(*datum.Value) {
				t.Errorf("Expected %d LowestCommonMultipleCPU but got %d", expectedCPU, int(*datum.Value))
			}
		case "LowestCommonMultipleMemory":
			if expectedMemory != int(*datum.Value) {
				t.Errorf("Expected %d LowestCommonMultipleMemory but got %d", expectedMemory, int(*datum.Value))
			}
		case "RegisteredSchedulable":
			if expectedRegisteredSchedulable != int(*datum.Value) {
				t.Errorf("Expected %d RegisteredSchedulable but got %d", expectedRegisteredSchedulable, int(*datum.Value))
			}
		case "RemainingSchedulable":
			if expectedRemainingSchedulable != int(*datum.Value) {
				t.Errorf("Expected %d RemainingSchedulable but got %d", expectedRemainingSchedulable, int(*datum.Value))
			}
		}
		if len(datum.Dimensions) != expectedNumberOfDimensions {
			t.Error("Expected", expectedNumberOfDimensions, "dimensions, but got:", datum.GoString())
		}
		actualClusterName := ""
		actualInstanceType := ""
		missingClusterName := true
		missingInstanceType := true
		for _, dimension := range datum.Dimensions {
			switch *dimension.Name {
			case "ClusterName":
				actualClusterName = *dimension.Value
				missingClusterName = false
			case "InstanceType":
				actualInstanceType = *dimension.Value
				missingInstanceType = false
			}
		}
		if missingClusterName {
			t.Error("Missing ClusterName or InstanceType among dimensions:", datum.GoString())
		}
		if missingInstanceType {
			t.Error("Missing InstanceType or InstanceType among dimensions:", datum.GoString())
		}
		if *expected.Cluster != actualClusterName {
			t.Errorf("Expected ClusterName %q but got %q", *expected.Cluster, actualClusterName)
		}
		if expectedInstanceType != actualInstanceType {
			t.Errorf("Expected InstanceType %q but got %q", expectedInstanceType, actualInstanceType)
		}
		if "Count" != *datum.Unit {
			t.Errorf("Expected Unit to be Count, but it's %q", *datum.Unit)
		}
		if beforeTimestamp.After(*datum.Timestamp) {
			t.Errorf("Expected Timestamp to be _after_ %q but got %q", beforeTimestamp, *datum.Timestamp)
		}
	}
}
