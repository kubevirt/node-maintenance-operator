#!/bin/bash

#the readiness probe determines if the operator deployment is ready, to do this it checks that port 4345 is being listened at.
#this is needed for the following scenario: the deployment was just up but the web server for the webhook
#wasn't yet initialised. At that moment in time the create request for the CRD object passed without
#webhook validation, as apiserver failed to make the request for the webhook.
#Therefore the fix was to let the operator deployment be up only if the web server of the webhook is up and listening.
#
#to do so it checks in /proc/net/tcp6 if someone is listening hex(4543)==0x11b4,
#The reason why this ends up as tcp6 is that the http server of the webhook listens on all interfaces
#for port 4543. I had to do all these exercises as there is no other way to check in a script
#if the port is being listened at, due to the limited tools of the operator image.
#(had to do netstat without netstat)

if [[ -f /apiserver.local.config/certificates/apiserver.crt ]]; then
	CNT=$(grep -ic 0000:11BF /proc/net/tcp6)
	if [[ $CNT != 1 ]]; then
		exit 1
	fi
fi

