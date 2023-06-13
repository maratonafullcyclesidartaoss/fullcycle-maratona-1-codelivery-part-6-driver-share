#!/bin/bash
kubectl testkube create test --file ../create_driver_load.js --type k6/script --name create-driver-load
kubectl testkube run test create-driver-load -f