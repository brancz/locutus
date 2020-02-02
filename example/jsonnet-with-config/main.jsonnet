local config = import 'generic-operator/config';

local grafanaDeployment = {
  apiVersion: 'apps/v1',
  kind: 'Deployment',
  metadata: {
    labels: {
      app: 'grafana',
    },
    name: config.name,
    namespace: config.namespace,
  },
  spec: {
    replicas: 1,
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
            image: 'grafana/grafana:5.1.0',
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
  rollout: (import 'rollout.json'),
}
