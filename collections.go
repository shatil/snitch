package snitch

import (
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
)

// ClusterResources maps how many containers of lowest common multiple size can
// be launched by each EC2 Instance Type in an ECS Cluster.
//
// "Lowest common multiple" means the largest container a cluster currently
// runs, whether it's the by largest CPU Unit count or Memory (RAM in MiB).
type ClusterResources struct {
	Cluster    *string
	Resources  map[string]map[string]int
	CPU        map[string]int
	Memory     map[string]int
	Registered map[string]int
	Remaining  map[string]int
}

// NewClusterResources creates a structure to map "RegisteredSchedulable" or
// "RemainingSchedulable" to count per *instanceType.
func NewClusterResources(cluster *string) *ClusterResources {
	cr := &ClusterResources{
		Cluster:    cluster,
		Resources:  map[string]map[string]int{},
		CPU:        map[string]int{},
		Memory:     map[string]int{},
		Registered: map[string]int{},
		Remaining:  map[string]int{},
	}
	cr.Resources["LowestCommonMultipleCPU"] = cr.CPU
	cr.Resources["LowestCommonMultipleMemory"] = cr.Memory
	cr.Resources["RegisteredSchedulable"] = cr.Registered
	cr.Resources["RemainingSchedulable"] = cr.Remaining
	return cr
}

// ToMetricData formats metrics as AWS CloudWatch-compatible metric data.
func (cr *ClusterResources) ToMetricData() (metricData []*cloudwatch.MetricDatum) {
	clusterDimension := &cloudwatch.Dimension{
		Name:  aws.String("ClusterName"),
		Value: cr.Cluster,
	}
	timestamp := aws.Time(time.Now())
	for metricName, metricResources := range cr.Resources {
		for instanceType, value := range metricResources {
			dimensions := []*cloudwatch.Dimension{
				clusterDimension,
				&cloudwatch.Dimension{
					Name:  aws.String("InstanceType"),
					Value: aws.String(instanceType),
				},
			}
			datum := &cloudwatch.MetricDatum{
				MetricName: aws.String(metricName),
				Dimensions: dimensions,
				Timestamp:  timestamp,
				Value:      aws.Float64(float64(value)),
				Unit:       aws.String("Count"),
			}
			metricData = append(metricData, datum)
		}
	}
	return
}
