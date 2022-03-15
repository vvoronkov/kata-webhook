#! /bin/bash
# Copyright (c) 2019 Intel Corporation
#
# SPDX-License-Identifier: Apache-2.0


WEBHOOK_NS=${1:-"default"}
WEBHOOK_NAME=${2:-"pod-annotate"}
WEBHOOK_SVC="${WEBHOOK_NAME}-webhook"

# Create CA for signing webhook cert
openssl genrsa -out webhookCA.key 2048
openssl req -x509 -new -nodes -key webhookCA.key -subj "/CN=${WEBHOOK_SVC}.${WEBHOOK_NS}.svc.ca" -days 365 -out webhookCA.crt

# Create certs for our webhook
openssl genrsa -out webhook.key 2048
openssl req -new -key ./webhook.key -subj "/CN=${WEBHOOK_SVC}.${WEBHOOK_NS}.svc" -out ./webhook.csr
openssl x509 -req -days 365 -in webhook.csr -CA webhookCA.crt -CAkey webhookCA.key -CAcreateserial -out webhook.crt

# Create certs secrets for k8s
kubectl create -n ${WEBHOOK_NS} secret generic \
    ${WEBHOOK_SVC}-certs \
    --from-file=key.pem=./webhookCA.key \
    --from-file=cert.pem=./webhookCA.crt \
    --dry-run -o yaml > ./deploy/webhook-certs.yaml

# Set the CABundle on the webhook registration
CA_BUNDLE=$(cat webhookCA.crt | base64 -w0)
sed -e "s/PROJECT_NAMESPACE/${WEBHOOK_NS}/" -e "s/CA_BUNDLE/${CA_BUNDLE}/" ./deploy/webhook-registration.yaml.tpl > ./deploy/webhook-registration.yaml

# Clean
rm webhookCA.key webhookCA.crt webhook.key webhook.csr webhook.crt
