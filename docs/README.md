# `cloudenv`

I wish that AWS Lambda functions could be configured to use secrets stored in
AWS Parameter Store and AWS Secrets Manager in the same way that AWS ECS task
definitions can be. Specifically, I wish I could do this:

```yaml
Transform:
  - AWS::LanguageExtensions
  - cloudenv
  - AWS::Serverless-2016-10-31

Resources:
  Example:
    Type: AWS::Serverless::Function
    Properties:
      Architectures: [arm64]
      Runtime: python3.9
      Handler: index.handler
      Environment:
        Variables:
          HELLO: WORLD
        Secrets:
          MY_SECRET:        !Sub arn:aws:ssm:${AWS::Region}:${AWS::AccountId}:parameter/my/parameter/name
          MY_SECOND_SECRET: !Sub arn:aws:ssm:${AWS::Region}:${AWS::AccountId}:parameter/my/parameter/name
          THIRD_SECRET:     !Sub arn:aws:ssm:${AWS::Region}:${AWS::AccountId}:parameter/my/parameter/third
          REAL_SECRET:      !Sub arn:aws:secretsmanager:${AWS::Region}:${AWS::AccountId}:secret:mysecret-rlBksU
      InlineCode: |
        import os
        
        def handler(event, context):
          return { 
            'HELLO'           : os.environ['HELLO'],
            'MY_SECRET'       : os.environ['MY_SECRET'],
            'MY_SECOND_SECRET': os.environ['MY_SECOND_SECRET'],
            'THIRD_SECRET'    : os.environ['THIRD_SECRET'],
            'REAL_SECRET'     : os.environ['REAL_SECRET'],
          }
```

The code in this repo achieves the above. That's all you need to know if you want
to _use_ it. 

![example](/docs/execution-result.png)

## Supported runtimes

* dotnet6
* dotnetcore2.1
* dotnetcore3.1
* java11
* java8.al2
* nodejs10.x
* nodejs12.x
* nodejs14.x
* nodejs16.x
* python3.8
* python3.9
* ruby2.7
* provided (see below)
* provided.al2 (see below)

## How it's built

If you want to know how it was _built_, read on. It is made up of three parts:

An executable in [`cloudenv/cloudenv.go`](/cloudenv/cloudenv.go). This application
is bundled into a Lambda layer that does the following:

* Is invoked like so: `cloudenv /var/lang/bin/python3.9 /var/runtime/bootstrap.py`
* Looks for environment variables that look like either `MY_PASSWORD={aws-ssm}arn:aws:ssm:...` 
  or `OTHER_VAL={aws-sm}arn:aws:secretsmanager:...`.
* Fetches the values for those ARNs and substitutes them into the current 
  environment, using `ssm:GetParameters` and `secretsmanager:GetSecretValue`.
* Calls `exec()` to pass control to `/var/lang/bin/python3.9 /var/runtime/bootstrap.py`
* The Python code for the user's Lambda function can access the values at
  `os.environ.MY_PASSWORD` or `os.environ.OTHER_VAL`, with no AWS SDKs required.

This means that secrets are fetched during Lambda _init_ time, which is free on 
most runtimes. It also runs as parallel as possible for best performance.

The second part is a CloudFormation macro, seen on the third line of the example
YAML above. When included in a CloudFormation template, the `cloudenv` macro 
looks for any `AWS::Serverless::Function` that has an `Environment.Secrets` 
property (like the example function does) and:

* Moves these to the function's `Environment.Variables` section with the 
  appropriate `{aws-ssm}` or `{aws-sm}` prefix expected by the Lambda layer.
* Adds the (correct per CPU architecture) Lambda layer to the function's list 
  of `Layers`.
* Adds an `AWS_LAMBDA_EXEC_WRAPPER` environment variable to intercept function
  cold starts with the executable in the Lambda layer.
* Adds IAM policies that grant access to the **specific** values in Parameter
  Store and Secrets Manager.

The third part is a second two-line Lambda layer. It is an implementation detail
required by the fact that `provided` and `provided.al2` Lambda runtimes don't
support [wrapper scripts][wrapper-scripts]. So the function's `Handler` needs to
be a command that executes the actual Lambda function handler in those cases.

[wrapper-scripts]: https://docs.aws.amazon.com/lambda/latest/dg/runtimes-modify.html#runtime-wrapper
