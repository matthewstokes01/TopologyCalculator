#!/bin/bash

namespaces=$(kubectl get ns --no-headers | awk '{print $1}')
bannedNamespaces=(
kube-node-lease
kube-public
kube-system
)
bannedDeployments=(
)
echo $(date +%T) >> az-report.log

for namespace in $namespaces
do
  if [[ ${bannedNamespaces[@]} =~ $namespace ]]
  then
    echo "${namespace} is protected"
  else
    deployments=$(kubectl get deployment -n $namespace --no-headers | awk '{print $1}')
    for deployment in $deployments
    do
        echo " " >> az-report.log
        echo DEPLOYMENT: $deployment >> az-report.log
        azs=("eu-west-2a" "eu-west-2b" "eu-west-2c")
        matchLabels=$(kubectl get deployment $deployment -n $namespace -o json | jq '.spec.selector.matchLabels' | jq 'to_entries[] |  "\(.key)=\(.value)"' | tr -d '"' | head -n 1)
        for az in "${azs[@]}"
        do
            nodesInAZ=$(kubectl get nodes -l "topology.kubernetes.io/zone=${az}" --no-headers | cut -d' ' -f1)
            totalPodsInZone=0
            for node in $nodesInAZ
            do
            numberOfPods=$(kubectl get pods -A -o wide -l=$matchLabels --field-selector spec.nodeName=$node --no-headers | grep Running | wc -l)
            if [ $numberOfPods -gt 0 ]; then
                totalPodsInZone=$(($totalPodsInZone + $numberOfPods))
            fi
            done
            echo "$az: $totalPodsInZone" >> az-report.log
        done
    done
  fi
done

echo $(date +%T) >> az-report.log