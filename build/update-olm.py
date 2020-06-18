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
    'maturity': 'beta',
}

_KUBEVIRT_ICON_TYPE="image/png"
_KUBEVIRT_ICON_BASE_64="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"

_CRD_INFOS = {
    'nodemaintenances.kubevirt.io': {
        'displayName': 'KubeVirt node maintenance',
        'description': \
                'Represents a deployment of node maintenance crd',
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
    manifest['spec']['icon'][0]['mediatype'] = _KUBEVIRT_ICON_TYPE
    manifest['spec']['icon'][0]['base64data'] = _KUBEVIRT_ICON_BASE_64

    for crd in manifest['spec']['customresourcedefinitions']['owned']:
        crd.update(_CRD_INFOS.get(crd['name'], {}))

    yaml.safe_dump(manifest, sys.stdout, default_flow_style=False)

if __name__ == '__main__':
    for arg in sys.argv[1:]:
        try:
            process(arg)
        except Exception as ex:
            logging.error('error processing %r: %s', arg, ex)
            sys.exit(1)
# keep going!
