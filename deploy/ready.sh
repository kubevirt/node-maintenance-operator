#!/bin/bash

if [[ -f /apiserver.local.config/certificates/apiserver.crt ]]; then
	CNT=$(grep -ic 0000:11BF /proc/net/tcp6)
	if [[ $CNT != 1 ]]; then
		exit 1
	fi
fi

