package main

import (
	"os"

	"github.com/aws/aws-lambda-go/lambda"

	"github.com/shatil/snitch"
)

// So CLI invocation, testing, etc., work outside of Lambda:
// https://github.com/aws/aws-lambda-go/blob/master/lambda/entry.go
var lambdaStart = lambda.Start

func main() {
	if os.Getenv("_LAMBDA_SERVER_PORT") == "" {
		lambdaStart = func(interface{}) {
			snitch.Main()
		}
	}
	lambdaStart(snitch.Main)
}
