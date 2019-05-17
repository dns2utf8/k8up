# Overview

K8up is a backup operator that will handle PVC and app backups on a k8s/OpenShift cluster.

Just create a `schedule` object in the namespace you’d like to backup. It’s that easy. K8up takes care of the rest. It also provides a Prometheus endpoint for monitoring.

K8up is currently under heavy development and far from feature complete. But it should already be stable enough for production use.