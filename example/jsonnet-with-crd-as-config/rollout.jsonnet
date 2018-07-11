{
  apiVersion: 'workflow.kubernetes.io/v1alpha1',
  kind: 'Rollout',
  metadata: {
    name: 'jsonnet',
  },
  spec: {
    groups: [
      {
        name: 'Rollout Grafana',
        steps: [
          {
            action: 'CreateOrUpdate',
            object: 'deployment',
            success: [
              {
                fieldComparisons: [
                  {
                    name: 'Generation correct',
                    path: '{.metadata.generation}',
                    value: {
                      path: '{.status.observedGeneration}',
                    },
                  },
                  {
                    name: 'All replicas updated',
                    path: '{.status.replicas}',
                    value: {
                      path: '{.status.updatedReplicas}',
                    },
                  },
                  {
                    name: 'No replica unavailable',
                    path: '{.status.unavailableReplicas}',
                    default: 0,
                    value: {
                      static: 0,
                    },
                  },
                ],
              },
            ],
          },
        ],
      },
    ],
  },
}
