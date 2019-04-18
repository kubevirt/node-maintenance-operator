#!/usr/bin/env python3

import logging
import sys
import yaml


_ANNOTATIONS = {
    'categories': 'OpenShift Optional',
    'containerImage': 'quay.io/kubevirt/node-maintenance-operator',
    'repository': 'https://github.com/kubevirt/node-maintenance-operator',
    'description': \
        'Node-maintenance-operator maintains nodes in cluster',
}
_DESCRIPTION = "Node maintenance operator"
_NAMESPACE = 'node-maintenance-operator'
_SPEC = {
    'description': _DESCRIPTION,
    'provider': {
        'name': 'KubeVirt project'
    },
    'maintainers': [{
        'name': 'KubeVirt project',
        'email': 'kubevirt-dev@googlegroups.com',
    }],
    'keywords': [
        'KubeVirt', 'Virtualization', 'Node-maintenance'
    ],
    'links': [{
        'name': 'KubeVirt',
        'url': 'https://kubevirt.io',
    }, {
        'name': 'Source Code',
        'url': 'https://github.com/kubevirt/node-maintenance-operator'
    }],
    'labels': {
        'alm-owner-kubevirt': 'nodemaintenanceoperator',
        'operated-by': 'nodemaintenanceoperator',
    },
    'selector': {
        'matchLabels': {
            'alm-owner-kubevirt': 'nodemaintenanceoperator',
            'operated-by': 'nodemaintenanceoperator',
        },
    },
}

_CRD_INFOS = {
    'nodemaintenances.kubevirt.io': {
        'displayName': 'KubeVirt node maintenance',
        'description': \
                'Represents a deployment of node maintenance',
        'specDescriptors': [{
            'description': \
                'The version of the node maintenance to deploy',
            'displayName': 'Version',
            'path': 'version',
            'x-descriptors': [
                'urn:alm:descriptor:io.kubernetes.node-maintenance:version',
            ],
        }],
    }
}


def process(path):
    with open(path, 'rt') as fh:
        manifest = yaml.safe_load(fh)

    manifest['metadata']['namespace'] = _NAMESPACE
    manifest['metadata']['annotations'].update(_ANNOTATIONS)

    manifest['spec'].update(_SPEC)

    for crd in manifest['spec']['customresourcedefinitions']['owned']:
        crd.update(_CRD_INFOS.get(crd['name'], {}))

    yaml.safe_dump(manifest, sys.stdout)


if __name__ == '__main__':
    for arg in sys.argv[1:]:
        try:
            process(arg)
        except Exception as ex:
            logging.error('error processing %r: %s', arg, ex)
# keep going!
