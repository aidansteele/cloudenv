import json
import os


def handler(event, context):
    print(json.dumps({'before': event}))

    resources = event['fragment']['Resources']
    for name in resources:
        resource = resources[name]
        if resource['Type'] == 'AWS::Serverless::Function':
            handle(name, resource)

    print(json.dumps({'after': event}))

    return {
        'status': 'success',
        'requestId': event['requestId'],
        'fragment': event['fragment'],
    }


def handle(name, resource):
    props = resource['Properties']
    secrets = props['Environment'].pop('Secrets')
    print(json.dumps(secrets))

    env = props['Environment'].get('Variables', {})
    props['Environment']['Variables'] = env

    package_type = props.get('PackageType', 'Zip')

    if len(secrets) > 0:
        if package_type == 'Zip':
            layers = props.get('Layers', [])
            props['Layers'] = layers

            if props.get('Architectures', ['x86_64'])[0] == 'arm64':
                layers.append(os.environ.get('LayerArm64'))
            else:
                layers.append(os.environ.get('LayerX8664'))

            env['AWS_LAMBDA_EXEC_WRAPPER'] = '/opt/cloudenv'

            runtime = props['Runtime']
            if runtime == 'provided' or runtime == 'provided.al2':
                layers.append(os.environ.get('BootstrapLayer'))
        elif package_type == 'Image':
            image_config = props.get('ImageConfig', {})
            entrypoint = image_config.get('EntryPoint', [])
            entrypoint = ['/opt/cloudenv'].extend(entrypoint)
            image_config['EntryPoint'] = entrypoint
            props['ImageConfig'] = image_config

    ssmParams = []
    smSecrets = []

    for key in secrets:
        arn = secrets[key]

        prefix = ""
        service = arn.split(":")[2]
        if service == "ssm":
            ssmParams.append(arn)
            prefix = "{aws-ssm}"
        elif service == "secretsmanager":
            smSecrets.append(arn)
            prefix = "{aws-sm}"
        else:
            raise "oh no!"

        env[key] = prefix + arn

    if len(ssmParams) > 0:
        policies = props.get('Policies', [])
        props['Policies'] = policies
        policies.append({
            'Statement': [
                {
                    'Effect': 'Allow',
                    'Action': 'ssm:GetParameters',
                    'Resource': list(set(ssmParams))
                }
            ]
        })

    if len(smSecrets) > 0:
        policies = props.get('Policies', [])
        props['Policies'] = policies
        policies.append({
            'Statement': [
                {
                    'Effect': 'Allow',
                    'Action': 'secretsmanager:GetSecretValue',
                    'Resource': list(set(smSecrets))
                }
            ]
        })
