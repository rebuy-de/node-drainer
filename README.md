# node-drainer

[![Build Status](https://travis-ci.org/rebuy-de/node-drainer.svg?branch=master)](https://travis-ci.org/rebuy-de/node-drainer)
[![license](https://img.shields.io/github/license/rebuy-de/node-drainer.svg)]()

Utilise the power of AWS Auto Scaling group (ASG) lifecycle hooks and drain your Kubernetes nodes gracefully.
**node-drainer** reads AWS ASG lifecycle hook messages from an SQS queue.
For each item received it contacts Kubernetes, taints the node to be shut down and evicts any pods not tolerant to the taint.
Meant to be run in side Kubernetes with a single replica only.

> **Development Status** *node-drainer* is designed for internal use only.
> Expect breaking changes any time, but if you experience any issue, feel free
> to open an issue anyway.
>
> :fire: Consider using the [AWS Node Termination
> Handler](https://github.com/aws/aws-node-termination-handler), which is the
> official tool from AWS. It has better support and we will ourselves likely
> with to it, if we experience a bigger roadblock with node-drainer.

## Use cases
*node-drainer* is useful whenever any of the Kubernetes worker nodes running in AWS must be shut down. Graceful eviction of Kubernetes pods from terminated nodes ensures continuous operation of services when:
- Performing a rolling Kubernetes cluster update
- Changing AWS EC2 instance types for worker nodes
- Updating the AWS EC2 instance image on worker nodes
- Scaling down the number of workers periodically when the cluster load is low

## Usage
All of **node-drainer**'s configuration is done using command line arguments, with the intention to be defined inside a Kubernetes deployment yaml file.

For a full list of parameters run:
```
./node-drainer -h
```

**node-drainer** can be configured to run outside of Kubernetes too, for testing purposes or otherwise. Below are two configuration examples.

### Running locally
When running locally we have to specify a valid kubeconfig file path as well as any AWS credentials needed. In the following example we are using a pre-configured AWS profile.
```
node-drainer --kubeconfig /example/kubeconfig/path --profile example_aws_profile --region example_region --queue-name example_queue_name
```
### Running in Kubernetes
When running inside a Kubernetes cluster in a pod, the Kubernetes configuration information is picked up automatically. We still have to configure AWS access as usual.
```
node-drainer --access-key-id example_id --secret-access-key example_secret --region example_region --queue-name example_queue_name
```

## Installation

* Binaries for *node-drainer* are provided for each release [here](https://github.com/rebuy-de/node-drainer/releases).
* Docker containers are are provided [here](https://quay.io/repository/rebuy/node-drainer). To obtain the latest docker image run `docker pull quay.io/rebuy/node-drainer:master`.
* For deploying *node-drainer* docker image to your Kubernetes cluster you can use the sample manifest files (found [here](https://github.com/rebuy-de/node-drainer/tree/master/samples)), just remember to fill in your own AWS credentials. I you use RBAC in Kubernetes you can also take advantage of the sample service account configuration.

To compile *node-drainer* from source you need a working
[Golang](https://golang.org/doc/install) development environment. The sources
must be cloned to `$GOPATH/src/github.com/rebuy-de/node-drainer`.

Also you need to install [godep](github.com/golang/dep/cmd/dep),
[golint](https://github.com/golang/lint/) and [GNU
Make](https://www.gnu.org/software/make/).

Then you just need to run `make build` to compile a binary into the project
directory or `make install` to install *node-drainer* into `$GOPATH/bin`. With
`make xc` you can cross compile *node-drainer* for other platforms.

## Contact channels
Feel free to create a GitHub Issue for any questions, bug reports or feature requests.

## How to contribute
You can contribute to *node-drainer* by forking this repository, making your changes and creating a Pull Request against our repository. If you are unsure how to solve a problem or have other questions about a contributions, please create a GitHub issue.
