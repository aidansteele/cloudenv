package main

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"golang.org/x/sync/errgroup"
	"os"
	"strings"
	"syscall"
)

const prefixSsm = "{aws-ssm}"
const prefixSm = "{aws-sm}"

func main() {
	ctx := context.Background()

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}

	// extract set of parameter names and secret names to fetch
	envmap := map[string]string{}
	params := map[string]string{}
	secrets := map[string]string{}

	for _, kv := range os.Environ() {
		name, value, _ := strings.Cut(kv, "=")
		envmap[name] = value

		if strings.HasPrefix(value, prefixSsm) {
			param := strings.TrimPrefix(value, prefixSsm)
			params[param] = ""
		} else if strings.HasPrefix(value, prefixSm) {
			secret := strings.TrimPrefix(value, prefixSm)
			secrets[secret] = ""
		}
	}

	// populate map with values from parameter store
	err = populateParameters(ctx, ssm.NewFromConfig(cfg), params)
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}

	// populate map with values from secrets manager
	err = populateSecrets(ctx, secretsmanager.NewFromConfig(cfg), secrets, 5)
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}

	// turn env map back into a slice for exec syscall
	envslice := make([]string, len(envmap))
	for name, value := range envmap {
		if strings.HasPrefix(value, prefixSsm) {
			param := strings.TrimPrefix(value, prefixSsm)
			value = params[param]
		} else if strings.HasPrefix(value, prefixSm) {
			secret := strings.TrimPrefix(value, prefixSm)
			value = secrets[secret]
		}

		envslice = append(envslice, fmt.Sprintf("%s=%s", name, value))
	}

	// now pass control to the program proper
	err = syscall.Exec(os.Args[1], os.Args[1:], envslice)
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
}

func populateParameters(ctx context.Context, api *ssm.Client, params map[string]string) error {
	arnChunks := chunk[string](keys(params), 10)
	for _, arns := range arnChunks {
		names := make([]string, len(arns))
		for i, arn := range arns {
			// arn:aws:ssm:us-east-1:514202201242:parameter/arn
			split := strings.SplitN(arn, ":", 6)
			names[i] = strings.TrimPrefix(split[5], "parameter")
		}

		get, err := api.GetParameters(ctx, &ssm.GetParametersInput{
			Names:          names,
			WithDecryption: aws.Bool(true),
		})
		if err != nil {
			return fmt.Errorf("getting parameters: %w", err)
		}

		if len(get.InvalidParameters) > 0 {
			return fmt.Errorf("invalid parameters: %s", strings.Join(get.InvalidParameters, ", "))
		}

		for _, parameter := range get.Parameters {
			params[*parameter.ARN] = *parameter.Value
		}
	}

	return nil
}

func populateSecrets(ctx context.Context, api *secretsmanager.Client, secrets map[string]string, concurrency int) error {
	inch := make(chan string, len(secrets))
	for secret := range secrets {
		inch <- secret
	}
	close(inch)

	outch := make(chan map[string]string, len(secrets))

	g, ctx := errgroup.WithContext(ctx)
	for idx := 0; idx < concurrency; idx++ {
		g.Go(func() error {
			for secret := range inch {
				gsv, err := api.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
					SecretId: &secret,
				})
				if err != nil {
					return fmt.Errorf(": %w", err)
				}

				if gsv.SecretString != nil {
					outch <- map[string]string{secret: *gsv.SecretString}
				} else if gsv.SecretBinary != nil {
					outch <- map[string]string{secret: string(gsv.SecretBinary)}
				}
			}

			return nil
		})
	}

	err := g.Wait()
	if err != nil {
		return fmt.Errorf("getting secrets: %w", err)
	}

	for idx := 0; idx < len(secrets); idx++ {
		out := <-outch
		for k, v := range out {
			secrets[k] = v
		}
	}

	close(outch)
	return nil
}

func keys[K comparable, V any](m map[K]V) []K {
	slice := make([]K, len(m))

	idx := 0
	for t := range m {
		slice[idx] = t
		idx++
	}

	return slice
}

func chunk[T any](slice []T, chunkSize int) [][]T {
	var chunks [][]T

	for i := 0; i < len(slice); i += chunkSize {
		end := i + chunkSize

		// necessary check to avoid slicing beyond
		// slice capacity
		if end > len(slice) {
			end = len(slice)
		}

		chunks = append(chunks, slice[i:end])
	}

	return chunks
}
