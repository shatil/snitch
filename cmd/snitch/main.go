package main

import (
	"flag"
	"os"

	"github.com/aws/aws-lambda-go/lambda"

	"github.com/shatil/snitch"
)

// Package arranged so CLI invocation, testing, etc., work outside of Lambda:
// https://github.com/aws/aws-lambda-go/blob/master/lambda/entry.go
//
// However, I haven't actually written tests since this is in its own package.
var lambdaStart = lambda.Start
var sn *snitch.Snitcher

func main() {
	if os.Getenv("_LAMBDA_SERVER_PORT") == "" {
		lambdaStart = func(interface{}) {
			sn = &snitch.Snitcher{
				Namespace:     flag.String("n", "", "metrics namespace in CloudWatch"),
				ShouldPublish: flag.Bool("p", false, "do publish findings to CloudWatch"),
			}
			if !flag.Parsed() {
				flag.Parse()
			}
			snitch.Run(sn)
		}
	}
	lambdaStart(snitch.Run)
}
