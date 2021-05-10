![OSCAR-CLI logo](img/oscar-cli.png)

[![build](https://github.com/grycap/oscar-cli/actions/workflows/main.yaml/badge.svg)](https://github.com/grycap/oscar-cli/actions/workflows/main.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/grycap/oscar-cli)](https://goreportcard.com/report/github.com/grycap/oscar-cli)
[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white&style=flat)](https://pkg.go.dev/github.com/grycap/oscar-cli)
[![License](https://img.shields.io/github/license/grycap/oscar-cli)](https://github.com/grycap/oscar-cli/blob/main/LICENSE)

OSCAR-CLI provides a command line interface to interact with [OSCAR](https://github.com/grycap/oscar) clusters in a simple way. It supports service management, workflows definition from FDL (Functions Definition Language) files and the ability to manage files from OSCAR's compatible storage providers (MinIO, AWS S3 and Onedata).

## Download

### Releases

The easy way to download OSCAR-CLI is through the github [releases page](https://github.com/grycap/oscar-cli/releases). There are binaries for multiple platforms and OS. If you need a binary for another platform, please open an [issue](https://github.com/grycap/oscar-cli/issues).

### Install from source

If you have [go](https://golang.org/doc/install) installed, you can get it from source directly by executing:

```sh
go get github.com/grycap/oscar-cli
```

## Example workflow

The folder `example-workflow` contains all the necessary files to create a simple workflow to test the tool. The workflow is composed by the [plant-classification](https://github.com/grycap/oscar/tree/master/examples/plant-classification-theano) and the [grayify (ImageMagick)](https://github.com/grycap/oscar/tree/master/examples/imagemagick) examples.

In the `example-workflow.yaml` file you can find the definition of the two OSCAR services and its connection via a MinIO bucket. As can be seen, the identifier used for the OSCAR cluster in the workflow definition is `oscar-test`, so firstly you must add a pre-deployed cluster (deployment instructions can be found [here](https://grycap.github.io/oscar)) with the same identifier (or change the identifier in the definition file):

```
echo $MY_PASSWORD | oscar-cli cluster add oscar-test https://my-oscar.cluster my-username --password-stdin
```


Note that the user-script files are under the same folder, so to correctly apply the workflow you must be in the `example-workflow` directory:

```
cd example-workflow
oscar-cli apply example-workflow.yaml
```

Now, you can check that the two services are successfully deployed by running:

```
oscar-cli service list
```

And, to trigger the execution of the workflow, you can upload the `input-image.jpg` file to the `in` folder in the `example-workflow` MinIO bucket:

```
oscar-cli service put-file plants minio.default input-image.jpg example-workflow/in/image.jpg
```

To check that the service has been successfully invoked and a Kubernetes job has been created, you can run:

```
oscar-cli service logs list plants
```

You can also retrieve the logs from a service's job with:

```
oscar-cli service logs get plants JOB_NAME
```

Once the job has finished, its status will change to "Succeeded" and an output file will be uploaded to the `med` folder in the `example-workflow` MinIO bucket, triggering the grayify service. You can check the logs as in the previous steps, only changing the service name.

Finally, when the grayify service's job ends the execution, the result of the workflow will be stored in the `res` folder of the same bucket. You can download the resulting file by executing:

```
oscar-cli service get-file plants minio.default example-workflow/res/image.jpg result.jpg
```

## Available commands

  - [apply](#apply)
  - [cluster](#cluster)
    - [add](#add)
    - [default](#default)
    - [info](#info)
    - [list](#list)
    - [remove](#remove)
  - [service](#service)
    - [get](#get)
    - [list](#list-1)
    - [remove](#remove-1)
    - [logs list](#logs-list)
    - [logs get](#logs-get)
    - [logs remove](#logs-remove)
    - [get-file](#get-file)
    - [put-file](#put-file)
    - [list-files](#list-files)
  - [version](#version)
  - [help](#help)

### apply

Apply a FDL file to create or edit services in clusters.

```
Usage:
  oscar-cli apply FDL_FILE [flags]

Aliases:
  apply, a

Flags:
      --config string   set the location of the config file (YAML or JSON)
  -h, --help            help for apply
```

### cluster

Manages the configuration of clusters.

#### Subcommands

##### add

Add a new existing cluster to oscar-cli.

```
Usage:
  oscar-cli cluster add IDENTIFIER ENDPOINT USERNAME {PASSWORD | --password-stdin} [flags]

Aliases:
  add, a

Flags:
      --disable-ssl      disable verification of ssl certificates for the added cluster
  -h, --help             help for add
      --password-stdin   take the password from stdin

Global Flags:
      --config string   set the location of the config file (YAML or JSON)
```

##### default

Show or set the default cluster.

```
Usage:
  oscar-cli cluster default [flags]

Aliases:
  default, d

Flags:
  -h, --help         help for default
  -s, --set string   set a default cluster by passing its IDENTIFIER

Global Flags:
      --config string   set the location of the config file (YAML or JSON)
```

##### info

Show information of an OSCAR cluster.

```
Usage:
  oscar-cli cluster info [flags]

Aliases:
  info, i

Flags:
  -c, --cluster string   set the cluster
  -h, --help             help for info

Global Flags:
      --config string   set the location of the config file (YAML or JSON)
```

##### list

List the configured OSCAR clusters.

```
Usage:
  oscar-cli cluster list [flags]

Aliases:
  list, ls

Flags:
  -h, --help   help for list

Global Flags:
      --config string   set the location of the config file (YAML or JSON)
```

##### remove

Remove a cluster from the configuration file.

```
Usage:
  oscar-cli cluster remove IDENTIFIER [flags]

Aliases:
  remove, rm

Flags:
  -h, --help   help for remove

Global Flags:
      --config string   set the location of the config file (YAML or JSON)
```

### service

Manages the services within a cluster.

#### Subcommands

##### get

Get the definition of a service.

```
Usage:
  oscar-cli service get SERVICE_NAME [flags]

Aliases:
  get, g

Flags:
  -c, --cluster string   set the cluster
  -h, --help             help for get

Global Flags:
      --config string   set the location of the config file (YAML or JSON)
```

##### list

List the available services in a cluster.

```
Usage:
  oscar-cli service list [flags]

Aliases:
  list, ls

Flags:
  -c, --cluster string   set the cluster
  -h, --help             help for list

Global Flags:
      --config string   set the location of the config file (YAML or JSON)
```

##### remove

Remove a service from the cluster.

```
Usage:
  oscar-cli service remove SERVICE_NAME... [flags]

Aliases:
  remove, rm

Flags:
  -c, --cluster string   set the cluster
  -h, --help             help for remove

Global Flags:
      --config string   set the location of the config file (YAML or JSON)
```

##### logs list

List the logs from a service.

```
Usage:
  oscar-cli service logs list SERVICE_NAME [flags]

Aliases:
  list, ls

Flags:
  -h, --help             help for list
  -s, --status strings   filter by status (Pending, Running, Succeeded or Failed), multiple values can be specified by a comma-separated string

Global Flags:
  -c, --cluster string   set the cluster
      --config string    set the location of the config file (YAML or JSON)
```

##### logs get

Get the logs from a service's job.

```
Usage:
  oscar-cli service logs get SERVICE_NAME JOB_NAME [flags]

Aliases:
  get, g

Flags:
  -h, --help              help for get
  -t, --show-timestamps   show timestamps in the logs

Global Flags:
  -c, --cluster string   set the cluster
      --config string    set the location of the config file (YAML or JSON)
```

##### logs remove

Remove a service's job along with its logs.

```
Usage:
  oscar-cli service logs remove SERVICE_NAME {JOB_NAME... | --succeeded | --all} [flags]

Aliases:
  remove, rm

Flags:
  -a, --all         remove all logs from the service
  -h, --help        help for remove
  -s, --succeeded   remove succeeded logs from the service

Global Flags:
  -c, --cluster string   set the cluster
      --config string    set the location of the config file (YAML or JSON)
```

##### get-file

Get a file from a service's storage provider.

The STORAGE_PROVIDER argument follows the format STORAGE_PROVIDER_TYPE.STORAGE_PROVIDER_NAME,
being the STORAGE_PROVIDER_TYPE one of the three supported storage providers (MinIO, S3 or Onedata)
and the STORAGE_PROVIDER_NAME is the identifier for the provider set in the service's definition.

```
Usage:
  oscar-cli service get-file SERVICE_NAME STORAGE_PROVIDER REMOTE_FILE LOCAL_FILE [flags]

Aliases:
  get-file, gf

Flags:
  -c, --cluster string   set the cluster
  -h, --help             help for get-file

Global Flags:
      --config string   set the location of the config file (YAML or JSON)
```

##### put-file

Put a file in a service's storage provider.

The STORAGE_PROVIDER argument follows the format STORAGE_PROVIDER_TYPE.STORAGE_PROVIDER_NAME,
being the STORAGE_PROVIDER_TYPE one of the three supported storage providers (MinIO, S3 or Onedata)
and the STORAGE_PROVIDER_NAME is the identifier for the provider set in the service's definition.

```
Usage:
  oscar-cli service put-file SERVICE_NAME STORAGE_PROVIDER LOCAL_FILE REMOTE_FILE [flags]

Aliases:
  put-file, pf

Flags:
  -c, --cluster string   set the cluster
  -h, --help             help for put-file

Global Flags:
      --config string   set the location of the config file (YAML or JSON)
```

##### list-files

List files from a service's storage provider path.

The STORAGE_PROVIDER argument follows the format STORAGE_PROVIDER_TYPE.STORAGE_PROVIDER_NAME,
being the STORAGE_PROVIDER_TYPE one of the three supported storage providers (MinIO, S3 or Onedata)
and the STORAGE_PROVIDER_NAME is the identifier for the provider set in the service's definition.

```
Usage:
  oscar-cli service list-files SERVICE_NAME STORAGE_PROVIDER REMOTE_PATH [flags]

Aliases:
  list-files, list-file, lsf

Flags:
  -c, --cluster string   set the cluster
  -h, --help             help for list-files

Global Flags:
      --config string   set the location of the config file (YAML or JSON)
```

### version

Print the version.

```
Usage:
  oscar-cli version [flags]

Aliases:
  version, v

Flags:
  -h, --help   help for version
```

### help

Help provides help for any command in the application.
Simply type oscar-cli help [path to command] for full details.

```
Usage:
  oscar-cli help [command] [flags]

Flags:
  -h, --help   help for help
```
