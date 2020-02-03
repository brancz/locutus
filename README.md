# Locutus

> This project is entirely experimental, it may disappear from one day to another. Do not depend on it. It is primarily to share ideas at this stage. APIs will break.

Configuration management for Kubernetes. Declaratively define of rollout, success, failure and rollback, in a Kubernetes native way, without having to know all the low-level bits, but with the possibility to dive into, explore, extend and customize everything.

## Motivations and comparisons

At CoreOS / Red Hat we built a wide range of automation tools on top of Kubernetes, sometimes referred to as "Operators" (application specific Kubernetes controllers), that manage the entire lifecycle of an application or an application stack, however while suitable for some cases, it's unsuitable for others. This project is research to some extend, in order to explore how much of this work can be implemented in a generic way, but still allowing users who want to have more fine grained control. At best this project can be used without having to write any Go code, however it is extensible, so that if one does find that the functionality that comes out of the box everything is extensible.

To some degree this project compares to the operator-sdk, but this project does not only embrace level-triggered deployment management but treats edge triggered deployments (as typically done through CI/CD pipelines) as much of a first class citizen as level-triggered ones.

## Concepts

Locutus deployments are made up of five concepts: a trigger, rendering of API objects, runtime and static configuration, a spec that declaratively describes steps of rolling out objects on Kubernetes, and feedback of the whole process.

More detailed descriptions and out of the box functionality:

* __Trigger__: A trigger can be just an interval, where every 1 minute the operator reconciles state. The most popular option here is to watch Kubernetes objects (primarily custom resources registered through CustomResourceDefinitions).
  This project offers 3 kinds of triggers out of the box:
    * One-off (the reconciling will only be executed once)
    * Interval (every x time-interval will be reconciled)
    * Watch a resources (for example those registered through a CustomResourceDefinition)

* __Renderer__: Once a trigger has triggered reconciling the first step that typically happens is that a number of Kubernetes manifests are dynamically rendered. In the simplest case this is just static files, in more complex cases configurations and manifests are rendered in sophisticated ways.
  This project offers 2 renderers out of the box:
    * File (files that are statically read from disk)
    * Jsonnet (a jsonnet VM is built at runtime and executes a jsonnet script)

* __Configuration__: A configuration is an input for renderers, based on which renderers can perform dynamic rendering.
  This project offers 2 configuration paradigms:
    * A static configuration from disk
    * Configuration by trigger (such as a custom resource, that triggered reconciling)

* __Rollout__: Rolling out changes typically happens by performing upsert type actions, and in some cases migrations of the stateful resources. This project defines a rollout specification, where the interface between the render step and rollout is that the output of rendering is a flat map of manifests, the key being their name and the value being the content of the manifest. For example a simple rollout specification could be:

```
apiVersion: workflow.kubernetes.io/v1alpha1
kind: Rollout
metadata:
  name: grafana-rollout-0
spec:
  groups:
  - steps:
    - object: "grafana/deployment.json"
      action: "CreateOrUpdate"
```

  Note the action `CreateOrUpdate`. Out of the box this project offers `CreateOrUpdate` and `CreateIfNotExist`, these actions are extensible, so any arbitrarily complex rollout scenario is possible, but requires writing additional go code. The actions provided out of the box work with any resource, meaning they can be used on standard Kubernetes objects, but also any extended objects such as those registered through CustomResourceDefinitions.

* __Feedback__ (TODO): Currently this project does not support providing feedback, but the rollout specification could be used to generically report status of an entire rollout, a group within a rollout or even individual steps of a rollout. Feedback could be either in form of writing status back into a custom resource status subresource, or more generic with webhooks.

## Usage

This section will layout a few examples gradually going from very simple to more complex use cases.

Firstly install the `locutus`:

```
go get github.com/brancz/locutus
```

### File without configuration

Let's get started, with a simple one-off file renderer. And just to see what is happening under the hood, we will just tell it to render, and not apply the rollout just yet.

```
locutus --kubeconfig $KUBECONFIG --renderer=file --renderer.file.dir=example/files/manifests/ --renderer.file.rollout=example/files/rollout.yaml --trigger=oneoff --render-only
```

That should have printed a json object with `grafana/deployment.yaml` as a key, and the manifest as the value. If the `--render-only` flag is removed, it will actually be applied against the cluster and exit.

```
locutus --kubeconfig $KUBECONFIG --renderer=file --renderer.file.dir=example/files/manifests/ --renderer.file.rollout=example/files/rollout.yaml --trigger=oneoff
```

That should have now created a simple Grafana deployment in the `default` namespace. If we want to, we can now also reconcile this with the "interval" trigger, to make sure the deployment is always in the state we want it to be in.

```
locutus --kubeconfig $KUBECONFIG --renderer=file --renderer.file.dir=example/files/manifests/ --renderer.file.rollout=example/files/rollout.yaml --trigger=interval
```

### Jsonnet without configuration

Although without configuration jsonnet is not much more useful, except that the language is much more expressive than plain yaml/json, and can be used to deduplicate and normalize. To use the jsonnet renderer the entrypoint file must be configured as well as the potential library paths.

```
locutus --kubeconfig $KUBECONFIG --renderer=jsonnet --renderer.jsonnet.entrypoint=example/jsonnet/main.jsonnet --trigger=oneoff --render-only
```

### Jsonnet with configuration

Jsonnet becomes more powerful however when it can be used to dynamically generate the manifests based on a configuration. For demonstration purposes, let's use a static configuration file. This could be useful in a CI environment where the same "package" needs to be deployed with different configurations.

```
locutus --kubeconfig $KUBECONFIG --renderer=jsonnet --renderer.jsonnet.entrypoint=example/jsonnet-with-config/main.jsonnet --trigger=oneoff --config-file=example/jsonnet-with-config/config.json --render-only
```

### Jsonnet with custom resources as configuration

Operators are typically not referred to as operators if they are not watching and acting on custom resources. In order to do that a CustomResourceDefinition needs to be registered.

```
kubectl create -f example/jsonnet-with-crd-as-config/crd.yaml
```

That registers the `Grafana` resource within the `monitoring.coreos.com/v1` API. Now the locutus can watch those objects, dynamically render with the custom resource as the input for jsonnet and apply the rendered manifests according to the rollout spec.

```
locutus --kubeconfig $KUBECONFIG --renderer=jsonnet --renderer.jsonnet.entrypoint=example/jsonnet-with-crd-as-config/main.jsonnet --trigger=resource --trigger.resource.config=example/jsonnet-with-crd-as-config/config.yaml
```

That command starts a long running process which watches the `Grafana` objects and starts reconciling on events. To trigger this we can create a `Grafana` object:

```
kubectl create -f example/jsonnet-with-crd-as-config/grafana.yaml
```

### Extending functionality

If the built in functionality is not sufficient, additional renderers, triggers and rollout actions can be injected with only few lines of code.

TODO(brancz): explain how everything is a library, and main.go can be viewed as just a sample implementation.

## Roadmap

* Feedback (writing status back into a CRD; webhooks)
* Using multiple resources as config
* Rollout success through Prometheus metrics
* Canary deployment action
* Rollbacks
