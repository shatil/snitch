// Package snitch reads and optionally reports ECS Cluster resource consumption
// to assist with auto scaling.
//
// Measurements are calculated from ECS Cluster data read by AWS SDK for Go,
// and if reporting is enabled, measurements are published to CloudWatch.
//
// Example IAM permissions required to run (feel free to adjust "Resource"
// appropriately):
//	{
//		"Version": "2012-10-17",
//		"Statement": [
//			{
//				"Sid": "PermitReadingFromECS",
//				"Effect": "Allow",
//				"Action": [
//					"ecs:DescribeContainerInstances",
//					"ecs:ListClusters",
//					"ecs:ListContainerInstances"
//				],
//				"Resource": [
//					"*"
//				]
//			},
//			{
//				"Sid": "PermitWritingToCloudWatch",
//				"Effect": "Allow",
//				"Action": [
//					"cloudwatch:GetMetricStatistics",
//					"cloudwatch:PutMetricData"
//				],
//				"Resource": [
//					"*"
//				]
//			}
//		]
//	}
//
// Authentication is done by AWS SDK for Go using your ~/.aws/credentials.
package snitch

import (
	"log"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/cloudwatch/cloudwatchiface"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/ecs/ecsiface"
)

// Snitcher communicates with web services to collect or report data.
type Snitcher struct {
	// AWS clients from Go SDK, drawn from *iface to simplify testing.
	CloudWatch cloudwatchiface.CloudWatchAPI
	ECS        ecsiface.ECSAPI
	// Namespace in CloudWatch to publish metrics to.
	Namespace *string
	// Whether to publish metrics to CloudWatch.
	ShouldPublish *bool
}

// WithAWS adds AWS clients to Snitcher.
func (sn *Snitcher) WithAWS() *Snitcher {
	conf := &aws.Config{}
	sess := session.Must(session.NewSession(conf))
	if sn.CloudWatch == nil {
		sn.CloudWatch = cloudwatchiface.CloudWatchAPI(cloudwatch.New(sess))
	}
	if sn.ECS == nil {
		sn.ECS = ecsiface.ECSAPI(ecs.New(sess))
	}
	return sn
}

// DiscoverTasks communicates pages of ECS Tasks' ARNs discovered in cluster.
//
// While I'm no fan of arrays of string pointers, that's what AWS SDK outputs.
// Communication channel can safely be ranged over:
//	for tasks := range sn.DiscoverTasks(cluster) {
//		log.Println(*cluster, "has", len(tasks), "tasks in cohort")
//	}
func (sn *Snitcher) DiscoverTasks(cluster *string) <-chan []*string {
	com := make(chan []*string)
	input := &ecs.ListTasksInput{
		Cluster: cluster,
	}
	go func() {
		err := sn.ECS.ListTasksPages(
			input,
			func(page *ecs.ListTasksOutput, last bool) bool {
				com <- page.TaskArns
				return len(page.TaskArns) > 0
			},
		)
		if err != nil {
			log.Printf("Failed to ListTasksPages for %q: %s", *cluster, err)
		}
		close(com)
	}()
	return com
}

// MeasureResources finds "lowest common multiple" among reservable resources
// for specified tasks within a cluster.
//
// Supply ECS cluster as aws.String() and ECS tasks are arrays communicated
// from DiscoverTasks.
func (sn *Snitcher) MeasureResources(cluster *string, tasks []*string) (cpu, memory int) {
	input := &ecs.DescribeTasksInput{
		Cluster: cluster,
		Tasks:   tasks,
	}
	output, err := sn.ECS.DescribeTasks(input)
	if err != nil {
		log.Printf("Failed to DescribeTasks on %q: %s", *cluster, err)
		return
	}
	for _, task := range output.Tasks {
		taskCPU, err := strconv.Atoi(*task.Cpu)
		if err != nil {
			log.Printf("Failed to convert %q CPU to int: %s", *cluster, err)
		}
		taskMemory, err := strconv.Atoi(*task.Memory)
		if err != nil {
			log.Printf("Failed to convert %q Memory to int: %s", *cluster, err)
		}
		if taskCPU > cpu {
			cpu = taskCPU
		}
		if taskMemory > memory {
			memory = taskMemory
		}
	}
	log.Printf("%q largest container in cohort has %d CPU Units, %d MiB RAM", *cluster, cpu, memory)
	return
}

// ListContainerInstances produces a cluster's container instance ARNs ("IDs").
//
// Requires IAM permission "ecs:ListContainerInstances".
//
// BUG(shatil): ListContainerInstances output isn't paginated, so we see
// first 100 containers' ARNs only.
func (sn Snitcher) ListContainerInstances(cluster *string) []*string {
	input := &ecs.ListContainerInstancesInput{
		Cluster: cluster,
		Status:  aws.String("ACTIVE"),
	}
	output, err := sn.ECS.ListContainerInstances(input)
	if err != nil {
		log.Printf("Failed to ListContainerInstances in %q! %s", *cluster, err)
		return []*string{}
	}
	return output.ContainerInstanceArns
}

// DescribeContainerInstances gathers descriptions of ECS Container Instances.
//
// Requires IAM permission "ecs:DescribeContainerInstances".
func (sn *Snitcher) DescribeContainerInstances(cluster *string, instances []*string) []*ecs.ContainerInstance {
	input := &ecs.DescribeContainerInstancesInput{
		Cluster:            cluster,
		ContainerInstances: instances,
	}
	output, err := sn.ECS.DescribeContainerInstances(input)
	if err != nil {
		log.Printf("Failed to DescribeContainerInstances for %q! %s", *cluster, err)
		return []*ecs.ContainerInstance{}
	}
	return output.ContainerInstances
}

// DescribeResourcesByInstanceType collates an ECS Cluster's registered and
// remaining resources by EC2 Instance Type.
//	instances := sn.ListContainerInstances(cluster)
//	metricData := sn.DescribeResourcesByInstanceType(cluster, instances, cpu, memory)
//
// EC2 Instance Type is gleaned from ECS Attribute "ecs.instance-type", which I
// think is supplied by ECS.
func (sn *Snitcher) DescribeResourcesByInstanceType(cluster *string, instances []*string, cpu, memory int) []*cloudwatch.MetricDatum {
	cr := NewClusterResources(cluster)
	for _, container := range sn.DescribeContainerInstances(cluster, instances) {
		instanceType := getInstanceType(container.Attributes)
		// Look, Ma, no KeyError: https://play.golang.org/p/jI4VOhMjcNc
		cr.CPU[instanceType] = cpu
		cr.Memory[instanceType] = memory
		cr.Registered[instanceType] += ContainersPossible(cpu, memory, container.RegisteredResources)
		cr.Remaining[instanceType] += ContainersPossible(cpu, memory, container.RemainingResources)
	}
	log.Printf("%q has %+v", *cluster, cr.Resources)
	return cr.ToMetricData()
}

// DiscoverClusters reads ECS Clusters' ARNs like
// "arn:aws:ecs:ca-central-1:123456789012:cluster/my-cluster" and communicates
// derived Cluster nanme, like "my-cluster", to output channel.
//
// Requires "ecs:ListClusters" IAM permission.
func (sn *Snitcher) DiscoverClusters() <-chan *string {
	com := make(chan *string)
	go func() {
		err := sn.ECS.ListClustersPages(
			&ecs.ListClustersInput{},
			func(page *ecs.ListClustersOutput, last bool) bool {
				for _, arn := range page.ClusterArns {
					com <- aws.String(strings.Split(*arn, ":cluster/")[1])
				}
				return len(page.ClusterArns) > 0
			},
		)
		if err != nil {
			log.Println("Failed to ListClustersPages!", err)
		}
		close(com)
	}()
	return com
}

// ContainersPossible calculates how many containers are possible to launch.
//
// This calculates how many containers can be scheduled per EC2 Instance, since
// array of ECS Resources is supplied per-Instance. cpu and memory provided
// indicate the number of CPU Units and Memory (RAM in MiB) a container will
// need to launch.
func ContainersPossible(cpu, memory int, resources []*ecs.Resource) (canSchedule int) {
	var byCPU, byMemory int
	for _, resource := range resources {
		switch *resource.Name {
		case "CPU":
			byCPU += int(*resource.IntegerValue) / cpu
		case "MEMORY":
			byMemory += int(*resource.IntegerValue) / memory
		}
	}
	if byCPU < byMemory {
		canSchedule += byCPU
	} else {
		canSchedule += byMemory
	}
	return
}

// getInstanceType figures out the EC2 Instance Type from an array of ECS
// Attributes.
func getInstanceType(attributes []*ecs.Attribute) string {
	for _, attr := range attributes {
		if *attr.Name == "ecs.instance-type" {
			return *attr.Value
		}
	}
	return ""
}

// MeasureCluster measures how many containers an ECS Cluster can schedule.
func (sn *Snitcher) MeasureCluster(cluster *string) []*cloudwatch.MetricDatum {
	var cpu, memory int
	for tasks := range sn.DiscoverTasks(cluster) {
		cohortCPU, cohortMemory := sn.MeasureResources(cluster, tasks)
		if cohortCPU > cpu {
			cpu = cohortCPU
		}
		if cohortMemory > memory {
			memory = cohortMemory
		}
	}
	if cpu == 0 || memory == 0 {
		log.Printf("%q doesn't appear to be running any Tasks; skipping", *cluster)
		return []*cloudwatch.MetricDatum{}
	}
	log.Printf("%q lowest common multiple is %d CPU Units, %d MiB RAM", *cluster, cpu, memory)
	instances := sn.ListContainerInstances(cluster)
	return sn.DescribeResourcesByInstanceType(cluster, instances, cpu, memory)
}

// Measure how many containers an ECS Cluster can schedule.
func (sn *Snitcher) Measure() (metricData []*cloudwatch.MetricDatum) {
	com := make(chan []*cloudwatch.MetricDatum)
	defer close(com)
	numClusters := 0 // Since we don't know how many Clusters.
	for cluster := range sn.DiscoverClusters() {
		go func(cluster *string) {
			com <- sn.MeasureCluster(cluster)
		}(cluster)
		numClusters++
	}
	for i := 0; i < numClusters; i++ {
		metricData = append(metricData, <-com...)
	}
	return
}

// Publish metrics to CloudWatch.
//
// BUG(shatil): Publish must submit in batches of 20 MetricDatum because:
// https://github.com/aws/aws-sdk-go/issues/2019
func (sn *Snitcher) Publish(metricData []*cloudwatch.MetricDatum) {
	input := &cloudwatch.PutMetricDataInput{
		Namespace: sn.Namespace,
	}
	batchSize := 20
	log.Printf("Publishing %d metrics in batches of %d", len(metricData), batchSize)
	for i := 0; i < len(metricData); i += batchSize {
		end := i + batchSize
		if end > len(metricData) {
			end = len(metricData)
		}
		input.MetricData = metricData[i:end]
		if err := input.Validate(); err != nil {
			log.Println("Failed to validate metrics:", err)
			log.Println("Invalid metrics:", input.GoString())
		} else if _, err = sn.CloudWatch.PutMetricData(input); err != nil {
			log.Printf("Failed to publish %d metrics to CloudWatch: %s", len(input.MetricData), err)
			log.Printf("Metrics not published: %s", input.GoString())
		} else {
			log.Printf("Published %d metrics: %s", len(input.MetricData), input.GoString())
		}
	}
}

// Run measures and maybe publishes findings.
//
// During CLI or AWS Lambda usage, this is your entrypoint function. Lambda can
// use these handy environment variables in place of CLI arguments:
//	AWS_REGION for AWS Region (required unless ~/.aws/config sets it)
func Run(sn *Snitcher) {
	sn.WithAWS()
	metricData := sn.Measure()
	if *sn.ShouldPublish {
		sn.Publish(metricData)
	}
}
