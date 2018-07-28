package snitch

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/cloudwatch/cloudwatchiface"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/ecs/ecsiface"
)

// FakeContainerInstance creates a mock container instance for testing.
//
// Without "ecs.instance-type" attribute, MetricData validation will fail since
// InstanceType is a required dimension.
func NewFakeContainerInstance(reg, rem []*ecs.Resource) *ecs.ContainerInstance {
	return &ecs.ContainerInstance{
		Attributes: []*ecs.Attribute{
			{
				Name:  aws.String("ecs.instance-type"),
				Value: aws.String("fake.2xlarge"),
			},
		},
		RegisteredResources: reg,
		RemainingResources:  rem,
	}
}

// FakeCloudWatch mocks CloudWatch for testing, with some fields added.
type FakeCloudWatch struct {
	cloudwatchiface.CloudWatchAPI
	payload       []*cloudwatch.PutMetricDataInput // Stores supplied `*PutMetricDataInput`.
	errorToReturn error                            // `error` to return from fake methods.
}

// PutMetricDataInput fake-publishes metrics to CloudWatch.
func (fake *FakeCloudWatch) PutMetricData(input *cloudwatch.PutMetricDataInput) (*cloudwatch.PutMetricDataOutput, error) {
	fake.payload = append(fake.payload, input)
	return nil, fake.errorToReturn
}

// FakeECS mocks AWS ECS to give us the responses we need.
type FakeECS struct {
	ecsiface.ECSAPI
	checkCluster                  bool                     // Check that expectedCluster name matches.
	errorToReturn                 error                    // `error` to return from fake methods.
	expectedCluster               *string                  // Cluster name we expect during testing.
	expectedClusterArns           []string                 // Expected ECS Cluster ARNs.
	expectedCPU                   int                      // Expected CPU Unit count for LCM container size.
	expectedDescribeTasksOutput   *ecs.DescribeTasksOutput // Expected response by DescribeTasks.
	expectedMemory                int                      // Expected Memory (RAM in MiB) for LCM container size.
	expectedContainerInstanceArns []string                 // Expected ECS Container Instance ARNs.
	expectedContainerInstances    []*ecs.ContainerInstance // Expected ECS Container Instance ARNs.
	expectedRegistered            []*ecs.Resource          // Expected registered ECS Cluster resources.
	expectedRemaining             []*ecs.Resource          // Expected remaining ECS Cluster resources.
	expectedTaskArns              []string                 // Expected ECS Task ARNs.
	expectedRegisteredPossible    int                      // Expected number of schedulable containers w/ "RegisteredResources".
	expectedRemainingPossible     int                      // Expected number of schedulable containers w/ "RemainingResources".
	t                             *testing.T               // Enable logging and failure in mock.
}

// NewFakeECS constructs a new mock ECS "service" with pre-populated data.
func NewFakeECS(t *testing.T) *FakeECS {
	fake := &FakeECS{
		checkCluster:    true,
		expectedCluster: aws.String("fake-ecs-cluster"),
		expectedClusterArns: []string{
			"arn:aws:ecs:us-east-1:123456789012:cluster/fake-ecs-cluster",
			"arn:aws:ecs:us-east-1:123456789012:cluster/another-fake-ecs-cluster",
			"arn:aws:ecs:us-east-1:123456789012:cluster/who-even-uses-fargate",
		},
		expectedContainerInstanceArns: []string{
			"arn:aws:ecs:us-east-1:123456789012:container-instance/30ed79a6-8ecd-4d7e-89ed-1415960b679a",
			"arn:aws:ecs:us-east-1:123456789012:container-instance/31b326d2-2d50-4203-b000-44deabe3487a",
			"arn:aws:ecs:us-east-1:123456789012:container-instance/4c684147-c27f-478d-9b22-111c85648f6f",
		},
		expectedTaskArns: []string{
			"arn:aws:ecs:us-east-1:123456789012:task/1394beef-718f-42d7-b37b-97363e9ac917",
			"arn:aws:ecs:us-east-1:123456789012:task/6649bf9d-7b1d-4ed7-9920-e0404ed4f2e5",
			"arn:aws:ecs:us-east-1:123456789012:task/b9cfa5da-e760-457a-8673-1b61eb668b33",
		},
		t: t,
	}
	instanceCPU := 8192
	instanceMemory := 15468
	fake.expectedRegistered = []*ecs.Resource{
		{
			DoubleValue:  aws.Float64(0),
			IntegerValue: aws.Int64(int64(instanceCPU)),
			LongValue:    aws.Int64(0),
			Name:         aws.String("CPU"),
			Type:         aws.String("INTEGER"),
		},
		{
			DoubleValue:  aws.Float64(0),
			IntegerValue: aws.Int64(int64(instanceMemory)),
			LongValue:    aws.Int64(0),
			Name:         aws.String("MEMORY"),
			Type:         aws.String("INTEGER"),
		},
	}
	fake.expectedCPU = 2560
	fake.expectedMemory = 3072
	fake.expectedRemaining = []*ecs.Resource{
		{
			DoubleValue:  aws.Float64(0),
			IntegerValue: aws.Int64(int64(instanceCPU - fake.expectedCPU)),
			LongValue:    aws.Int64(0),
			Name:         aws.String("CPU"),
			Type:         aws.String("INTEGER"),
		},
		{
			DoubleValue:  aws.Float64(0),
			IntegerValue: aws.Int64(int64(instanceMemory - fake.expectedMemory)),
			LongValue:    aws.Int64(0),
			Name:         aws.String("MEMORY"),
			Type:         aws.String("INTEGER"),
		},
	}
	fake.expectedContainerInstances = []*ecs.ContainerInstance{
		NewFakeContainerInstance(fake.expectedRegistered, fake.expectedRemaining),
		NewFakeContainerInstance(fake.expectedRegistered, fake.expectedRemaining),
		NewFakeContainerInstance(fake.expectedRegistered, fake.expectedRemaining),
	}
	fake.expectedRegisteredPossible = len(fake.expectedContainerInstances) * ContainersPossible(fake.expectedCPU, fake.expectedMemory, fake.expectedContainerInstances[0].RegisteredResources)
	fake.expectedRemainingPossible = len(fake.expectedContainerInstances) * ContainersPossible(fake.expectedCPU, fake.expectedMemory, fake.expectedContainerInstances[0].RemainingResources)
	fake.expectedDescribeTasksOutput = &ecs.DescribeTasksOutput{
		Tasks: []*ecs.Task{
			{Cpu: aws.String(strconv.Itoa(fake.expectedCPU)), Memory: aws.String("1440")},
			{Cpu: aws.String("1024"), Memory: aws.String(strconv.Itoa(fake.expectedMemory))},
			{Cpu: aws.String("invalidCPU"), Memory: aws.String("invalidMemory")},
		},
	}
	return fake
}

// ListTasksPages fake-paginates listing of ECS Tasks.
func (fake *FakeECS) ListTasksPages(input *ecs.ListTasksInput, pager func(*ecs.ListTasksOutput, bool) bool) error {
	if fake.checkCluster && *fake.expectedCluster != *input.Cluster {
		fake.t.Errorf("expected cluster name %q but got %q", *fake.expectedCluster, *input.Cluster)
	}
	output := &ecs.ListTasksOutput{
		TaskArns: aws.StringSlice(fake.expectedTaskArns),
	}
	pager(output, true)
	return fake.errorToReturn
}

// DescribeTasks fake-describes ECS Tasks.
//
// Although in reality it's supposed to be related to the Task ARN and all...
// it's actually not. We care just for a few of the fields embedded in each
// Task.
func (fake *FakeECS) DescribeTasks(input *ecs.DescribeTasksInput) (*ecs.DescribeTasksOutput, error) {
	return fake.expectedDescribeTasksOutput, fake.errorToReturn
}

func (fake *FakeECS) ListContainerInstances(input *ecs.ListContainerInstancesInput) (*ecs.ListContainerInstancesOutput, error) {
	if "ACTIVE" != *input.Status {
		fake.t.Errorf("ListContainerInstances should look for ACTIVE only, got: %q", *input.Status)
	}
	output := &ecs.ListContainerInstancesOutput{
		ContainerInstanceArns: aws.StringSlice(fake.expectedContainerInstanceArns),
	}
	return output, fake.errorToReturn
}

func (fake *FakeECS) DescribeContainerInstances(input *ecs.DescribeContainerInstancesInput) (*ecs.DescribeContainerInstancesOutput, error) {
	if fake.checkCluster && *fake.expectedCluster != *input.Cluster {
		fake.t.Errorf("expected cluster name %q but got %q", *fake.expectedCluster, *input.Cluster)
	}
	output := &ecs.DescribeContainerInstancesOutput{
		ContainerInstances: fake.expectedContainerInstances,
	}
	return output, fake.errorToReturn
}

func (fake *FakeECS) ListClustersPages(input *ecs.ListClustersInput, pager func(*ecs.ListClustersOutput, bool) bool) error {
	for i := 0; i < len(fake.expectedClusterArns); i++ {
		output := &ecs.ListClustersOutput{
			ClusterArns: aws.StringSlice(fake.expectedClusterArns[i : i+1]),
		}
		pager(output, i+1 == len(fake.expectedClusterArns))
	}
	return fake.errorToReturn
}

// TestSnitcherPublish attempts to fake-publish to CloudWatch.
func TestSnitcher_Publish(t *testing.T) {
	fake := &FakeCloudWatch{}
	sn := &Snitcher{
		Namespace:  aws.String("Testable/Namespace"),
		CloudWatch: fake,
	}
	cr := NewClusterResources(aws.String("ecs-self-publishing-cluster"))
	cr.Registered["fake.instanceType"] += 5
	cr.Registered["another.fakeInstanceType"] += 10
	sn.Publish(cr.ToMetricData())
	published := fake.payload[0]
	metricData := published.MetricData
	numMetrics := 0
	for _, instanceCounts := range cr.Resources {
		for range instanceCounts {
			numMetrics++
		}
	}
	if *sn.Namespace != *published.Namespace {
		t.Errorf("Expected %q as Namespace, but got %q", *sn.Namespace, *published.Namespace)
	}
	if numMetrics != len(metricData) {
		t.Errorf("Expected %d inputs, but got %d", numMetrics, len(metricData))
	}
	// Force traversal of err logging.
	sn.Publish(metricData)
}

// TestSnitcher_PublishValidate forces Validate() failure (in
// service/cloudwatch/api.go), in this case by missing namespace.
//
// TODO(shatil): add some form of comparison test here.
func TestSnitcher_PublishValidate(t *testing.T) {
	sn := &Snitcher{}
	cr := NewClusterResources(aws.String("ecs-publish-validate-failure"))
	cr.Registered["fake.publishValidateFailure"] += 5
	cr.Registered["another.publishValidateFailure"] += 10
	sn.Publish(cr.ToMetricData())
}

// TestSnitcher_PublishError traverses error-handling code path.
//
// TODO(shatil): add some form of comparison test here.
func TestSnitcher_PublishError(t *testing.T) {
	fake := &FakeCloudWatch{
		errorToReturn: errors.New("triggering CloudWatch PutMetricData error"),
	}
	sn := &Snitcher{
		Namespace:  aws.String("Publish/Error"),
		CloudWatch: fake,
	}
	cr := NewClusterResources(aws.String("ecs-publish-error"))
	cr.Registered["fake.publishError"] += 5
	cr.Registered["another.publishError"] += 10
	sn.Publish(cr.ToMetricData())
}

func TestSnitcher_DiscoverTasks(t *testing.T) {
	fake := NewFakeECS(t)
	sn := &Snitcher{
		ECS: fake,
	}
	for page := range sn.DiscoverTasks(fake.expectedCluster) {
		for index, arn := range fake.expectedTaskArns {
			if arn != *page[index] {
				t.Errorf("Expected %q ECS Task ARN but got %q", arn, *page[index])
			}
		}
	}
	fake.errorToReturn = errors.New("chan should close even when there's an error")
	<-sn.DiscoverTasks(fake.expectedCluster)
}

func TestSnitcher_MeasureResources(t *testing.T) {
	fake := NewFakeECS(t)
	sn := &Snitcher{ECS: fake}
	cpu, memory := sn.MeasureResources(fake.expectedCluster, <-sn.DiscoverTasks(fake.expectedCluster))
	if fake.expectedCPU != cpu {
		t.Errorf("expected %d CPU Units but got %d", fake.expectedCPU, cpu)
	}
	if fake.expectedMemory != memory {
		t.Errorf("expected %d memory but got %d", fake.expectedMemory, memory)
	}
}

func TestSnitcher_MeasureResourcesError(t *testing.T) {
	fake := NewFakeECS(t)
	fake.errorToReturn = errors.New("cpu, memory ought to be zero when DiscoverTasks errors")
	sn := &Snitcher{ECS: fake}
	if cpu, memory := sn.MeasureResources(fake.expectedCluster, <-sn.DiscoverTasks(fake.expectedCluster)); cpu+memory != 0 {
		t.Errorf("expected cpu, memory to be 0, 0 during error, but got %d, %d", cpu, memory)
	}
}

func TestSnitcher_ListContainerInstances(t *testing.T) {
	fake := NewFakeECS(t)
	sn := &Snitcher{ECS: fake}
	for index, arn := range aws.StringValueSlice(sn.ListContainerInstances(fake.expectedCluster)) {
		if fake.expectedContainerInstanceArns[index] != arn {
			t.Errorf("expected %q among Container Instance ARNs in place of %q", fake.expectedContainerInstanceArns[index], arn)
		}
	}
	fake.errorToReturn = errors.New("during error there should be no Container Instance ARNs")
	if actual := len(sn.ListContainerInstances(fake.expectedCluster)); actual != 0 {
		t.Errorf("expected 0 Container Instance ARNs but got %d", actual)
	}
}

func TestSnitcher_DescribeContainerInstances(t *testing.T) {
	fake := NewFakeECS(t)
	sn := &Snitcher{ECS: fake}
	containerInstances := sn.DescribeContainerInstances(fake.expectedCluster, sn.ListContainerInstances(fake.expectedCluster))
	if len(containerInstances) == 0 {
		t.Error("expected some containers but got", containerInstances)
	}
	for index, containerInstance := range containerInstances {
		if fake.expectedContainerInstances[index] != containerInstance {
			t.Errorf("unexpected order of container instances (%s expected; got %s)", fake.expectedContainerInstances[index], containerInstance)
		}
	}
	fake.errorToReturn = errors.New("there should be no containers returned on error")
	if containerInstances := sn.DescribeContainerInstances(fake.expectedCluster, sn.ListContainerInstances(fake.expectedCluster)); len(containerInstances) != 0 {
		t.Error(fake.errorToReturn)
	}
}

func TestSnitcher_DescribeResourcesByInstanceType(t *testing.T) {
	fake := NewFakeECS(t)
	sn := &Snitcher{ECS: fake}
	measurements := sn.DescribeResourcesByInstanceType(
		fake.expectedCluster,
		aws.StringSlice(fake.expectedContainerInstanceArns),
		fake.expectedCPU,
		fake.expectedMemory,
	)
	if len(measurements) == 0 {
		t.Error("expectd some measurements but got:", measurements)
	}
}

func Test_getInstanceType(t *testing.T) {
	expected := "wanted.2xl"
	attributes := []*ecs.Attribute{
		{Name: aws.String("ecs.missing-instance-type"), Value: aws.String("missing")},
		{Name: aws.String("ecs.instance-type"), Value: aws.String(expected)},
	}
	if got := getInstanceType(attributes); got != expected {
		t.Errorf("getInstanceType() = %v, want %v", got, expected)
	}
	if got := getInstanceType([]*ecs.Attribute{}); got != "" {
		t.Errorf("getInstanceType() = %v, want empty string", got)
	}
}

// TestContainersPossible ensures calculation for number of containers possible
// to schedule is correct.
//
// Hardcoding values to ensure accuracy of logic.
func TestContainersPossible(t *testing.T) {
	nameCPU := aws.String("CPU")
	nameMemory := aws.String("MEMORY")
	type args struct {
		possible  int
		cpu       int
		memory    int
		resources []*ecs.Resource
	}
	for _, arg := range []args{
		{4, 1024, 2048, []*ecs.Resource{{Name: nameCPU, IntegerValue: aws.Int64(8192)}, {Name: nameMemory, IntegerValue: aws.Int64(8192)}}},
		{0, 1024, 2048, []*ecs.Resource{{Name: nameCPU, IntegerValue: aws.Int64(0)}, {Name: nameMemory, IntegerValue: aws.Int64(8192)}}},
		{3, 1024, 1024, []*ecs.Resource{{Name: nameCPU, IntegerValue: aws.Int64(3072)}, {Name: nameMemory, IntegerValue: aws.Int64(8192)}}},
	} {
		if got := ContainersPossible(arg.cpu, arg.memory, arg.resources); got != arg.possible {
			t.Errorf("expected ContainersPossible() = %d; got %d", arg.possible, got)
		}
	}
}

func TestSnitcher_DiscoverClusters(t *testing.T) {
	fake := NewFakeECS(t)
	sn := &Snitcher{ECS: fake}
	clusterNames := sn.DiscoverClusters()
	for _, arn := range fake.expectedClusterArns {
		name := aws.StringValue(<-clusterNames)
		if !strings.HasSuffix(arn, name) {
			t.Errorf("expected cluster ARN %q to end with cluster name %q", arn, name)
		}
	}
}

func TestSnitcher_DiscoverClustersError(t *testing.T) {
	// For some reason errorToReturn doesn't work right if NewFakeECS constructor is used here like this:
	//	fake = NewFakeECS(t)
	//	fake.errorToReturn = errors.New("traverse if err != nil")
	fake := &FakeECS{
		errorToReturn: errors.New("traverse if err != nil"),
	}
	sn := &Snitcher{ECS: fake}
	<-sn.DiscoverClusters()
}

func TestSnitcher_WithAWS(t *testing.T) {
	sn := &Snitcher{}
	if sn != sn.WithAWS() {
		t.Errorf("expected Snitcher to modify and return itself")
	}
	if sn.CloudWatch == nil {
		t.Errorf("expected Snitcher to have CloudWatch client")
	}
	if sn.ECS == nil {
		t.Errorf("expected Snitcher to have ECS client")
	}

}

func TestRun(t *testing.T) {
	cw := &FakeCloudWatch{}
	ecs := NewFakeECS(t)
	// ListTasksPages and DescribeContainerInstances check for matching cluster
	// name, which in this case we don't want.
	ecs.checkCluster = false
	sn := &Snitcher{
		CloudWatch:    cw,
		ECS:           ecs,
		Namespace:     aws.String("Collector/Test"),
		ShouldPublish: aws.Bool(true),
	}
	Run(sn)
	if len(cw.payload) == 0 {
		t.Error("missing FakeCloudWatch payload after test")
	}
}

func TestSnitcher_MeasureClusterEmpty(t *testing.T) {
	// Ensure empty response from FakeECS.
	ecs := &FakeECS{
		expectedDescribeTasksOutput: &ecs.DescribeTasksOutput{},
	}
	sn := &Snitcher{
		ECS: ecs,
	}
	actual := sn.MeasureCluster(aws.String("this cluster doesn't exist"))
	if len(actual) != 0 {
		t.Errorf("expected 0 data points but got %d", len(actual))
	}
}
