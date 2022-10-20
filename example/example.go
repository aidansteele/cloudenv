package main

import (
	"context"
	"encoding/json"
	"github.com/aws/aws-lambda-go/lambda"
	"os"
)

func main() {
	lambda.Start(handle)
}

func handle(ctx context.Context, _ json.RawMessage) (any, error) {
	env := map[string]string{}
	keys := []string{"HELLO", "MY_SECRET", "MY_SECOND_SECRET", "THIRD_SECRET", "REAL_SECRET"}
	for _, key := range keys {
		env[key] = os.Getenv(key)
	}

	return env, nil
}
