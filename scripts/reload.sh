#!/bin/bash

scripts/build.sh

kubectl delete -f manifests/

kubectl apply -f manifests/