local config = import 'generic-operator/config';

local grafanaDeployment = {
  apiVersion: 'apps/v1',
  kind: 'Deployment',
  metadata: {
    labels: {
      app: 'grafana',
    },
    name: 'grafana-' + config.metadata.name,
    namespace: config.metadata.namespace,
    ownerReferences: [{
      apiVersion: config.apiVersion,
      blockOwnerdeletion: true,
      controller: true,
      kind: config.kind,
      name: config.metadata.name,
      uid: config.metadata.uid,
    }],
  },
  spec: {
    replicas: config.spec.replicas,
    selector: {
      matchLabels: {
        app: 'grafana',
      },
    },
    template: {
      metadata: {
        labels: {
          app: 'grafana',
        },
      },
      spec: {
        containers: [
          {
            image: 'grafana/grafana:5.2.2',
            name: 'grafana',
            ports: [
              {
                containerPort: 3000,
                name: 'http',
              },
            ],
          },
        ],
      },
    },
  },
};

{
  objects: {
    deployment: grafanaDeployment,
  },
  rollout: (import 'rollout.jsonnet'),
}
