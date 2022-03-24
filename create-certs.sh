#! /bin/bash
# Copyright (c) 2019 Intel Corporation
#
# SPDX-License-Identifier: Apache-2.0


WEBHOOK_NS=${1:-"default"}
WEBHOOK_NAME=${2:-"pod-annotate"}
WEBHOOK_SVC="${WEBHOOK_NAME}-webhook"

# Create CA for signing webhook cert
openssl genrsa -out ca.key 2048
openssl req -x509 -new -nodes -key ca.key -subj "/CN=${WEBHOOK_SVC}.${WEBHOOK_NS}.ca" -days 365 -out ca.crt

# Create certs for our webhook
openssl req -newkey rsa:2048 -nodes -keyout webhook.key -subj "/CN=${WEBHOOK_SVC}.${WEBHOOK_NS}.svc" -out ./webhook.csr
echo "subjectAltName=DNS:${WEBHOOK_SVC}.${WEBHOOK_NS}.svc" > extensions.txt
echo "extendedKeyUsage=serverAuth" >> extensions.txt
openssl x509 -req -extfile extensions.txt -days 365 -in webhook.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out webhook.crt

# Create certs secrets for k8s
kubectl create -n ${WEBHOOK_NS} secret generic \
    ${WEBHOOK_SVC}-certs \
    --from-file=key.pem=./webhook.key \
    --from-file=cert.pem=./webhook.crt \
    --dry-run -o yaml > ./deploy/webhook-certs.yaml

# Set the CABundle on the webhook registration
CA_BUNDLE=$(cat ca.crt | base64 -w0)
sed -e "s/PROJECT_NAMESPACE/${WEBHOOK_NS}/" -e "s/CA_BUNDLE/${CA_BUNDLE}/" ./deploy/webhook-registration.yaml.tpl > ./deploy/webhook-registration.yaml

# Clean
rm ca.key ca.crt webhook.key webhook.csr webhook.crt
