#!/bin/bash

## Create 30 deployments with their own namespace


for i in {1..30}
do
    echo $i
    kubectl create namespace my-dep${i}
    kubectl create deployment my-dep${i} -n my-dep${i} --image=nginx --replicas=30
done