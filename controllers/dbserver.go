/*
Copyright 2020 Red Hat

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	ovnv1alpha1 "github.com/openstack-k8s-operators/ovn-operator/api/v1alpha1"
	util "github.com/openstack-k8s-operators/ovn-operator/pkg/common"
)

const (
	// OVSDBServerApp - App name
	OVSDBServerApp = "ovsdb-server"
	// OVSDBServerBootstrapApp - Bootstrap App name
	OVSDBServerBootstrapApp = "ovsdb-server-bootstrap"
)

const (
	hostsVolumeName = "hosts"
	runVolumeName   = "pod-run"
	dataVolumeName  = "data"

	ovnDBDir  = "/var/lib/openvswitch"
	ovnRunDir = "/ovn-run"

	dbStatusContainerName = "dbstatus"
	// DBServerContainerName - DBServer container name
	DBServerContainerName = "ovsdb-server"
)

func dbServerShell(server *ovnv1alpha1.OVSDBServer) *corev1.Pod {
	pod := &corev1.Pod{}
	pod.Name = server.Name
	pod.Namespace = server.Namespace

	return pod
}

func dbServerApply(
	pod *corev1.Pod,
	server *ovnv1alpha1.OVSDBServer,
	cluster *ovnv1alpha1.OVSDBCluster) {

	util.InitLabelMap(&pod.Labels)
	pod.Labels["app"] = OVSDBServerApp

	pod.Spec.RestartPolicy = corev1.RestartPolicyAlways

	// TODO
	// pod.Spec.Affinity

	dbPodVolumesApply(&pod.Spec.Volumes, server)

	if len(pod.Spec.InitContainers) != 2 {
		pod.Spec.InitContainers = make([]corev1.Container, 2)
	}
	hostsInitContainerApply(&pod.Spec.InitContainers[0], server, cluster)
	dbStatusContainerApply(&pod.Spec.InitContainers[1], server, cluster)

	if len(pod.Spec.Containers) != 1 {
		pod.Spec.Containers = make([]corev1.Container, 1)
	}

	dbContainer := &pod.Spec.Containers[0]
	dbServerContainerApply(dbContainer, server, cluster)

	dbContainer.ReadinessProbe = util.ExecProbe("/is_ready")
	dbContainer.ReadinessProbe.PeriodSeconds = 10
	dbContainer.ReadinessProbe.SuccessThreshold = 1
	dbContainer.ReadinessProbe.FailureThreshold = 1
	dbContainer.ReadinessProbe.TimeoutSeconds = 60

	dbContainer.LivenessProbe = util.ExecProbe("/is_live")
	dbContainer.LivenessProbe.InitialDelaySeconds = 60
	dbContainer.LivenessProbe.PeriodSeconds = 10
	dbContainer.LivenessProbe.SuccessThreshold = 1
	dbContainer.LivenessProbe.FailureThreshold = 3
	dbContainer.LivenessProbe.TimeoutSeconds = 10
}

func bootstrapPodShell(server *ovnv1alpha1.OVSDBServer) *corev1.Pod {
	pod := &corev1.Pod{}
	pod.Name = fmt.Sprintf("%s-bootstrap", server.Name)
	pod.Namespace = server.Namespace

	return pod
}

func bootstrapPodApply(
	pod *corev1.Pod,
	server *ovnv1alpha1.OVSDBServer,
	cluster *ovnv1alpha1.OVSDBCluster) {

	util.InitLabelMap(&pod.Labels)
	pod.Labels["app"] = OVSDBServerBootstrapApp

	pod.Spec.RestartPolicy = corev1.RestartPolicyOnFailure

	// TODO
	// pod.Spec.Affinity
	// We should ensure the bootstrap pod has the same affinity as the db
	// pod to better support late binding PVCs.

	dbPodVolumesApply(&pod.Spec.Volumes, server)

	if len(pod.Spec.InitContainers) != 2 {
		pod.Spec.InitContainers = make([]corev1.Container, 2)
	}
	hostsInitContainerApply(&pod.Spec.InitContainers[0], server, cluster)
	dbInitContainer := &pod.Spec.InitContainers[1]
	dbInitContainer.Name = "dbinit"
	dbInitContainer.Image = cluster.Spec.Image
	dbInitContainer.VolumeMounts = dbContainerVolumeMountsApply(dbInitContainer.VolumeMounts)
	dbInitContainer.Env = dbContainerEnvApply(dbInitContainer.Env, server)

	if server.Spec.ClusterID == nil {
		dbInitContainer.Command = []string{"/cluster-create"}
	} else {
		dbInitContainer.Command = []string{"/cluster-join", *server.Spec.ClusterID}
		dbInitContainer.Command = append(dbInitContainer.Command, server.Spec.InitPeers...)
	}

	if len(pod.Spec.Containers) != 1 {
		pod.Spec.Containers = make([]corev1.Container, 1)
	}
	dbStatusContainerApply(&pod.Spec.Containers[0], server, cluster)
}

func dbPodVolumesApply(volumes *[]corev1.Volume, server *ovnv1alpha1.OVSDBServer) {
	for _, vol := range []corev1.Volume{
		{Name: dataVolumeName, VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: pvcName(server)}},
		},
		{Name: runVolumeName, VolumeSource: util.EmptyDirVol()},
		{Name: hostsVolumeName, VolumeSource: util.EmptyDirVol()},
	} {
		updated := false
		for i := 0; i < len(*volumes); i++ {
			if (*volumes)[i].Name == vol.Name {
				(*volumes)[i] = vol
				updated = true
				break
			}
		}
		if !updated {
			*volumes = append(*volumes, vol)
		}
	}
}

func dbContainerVolumeMountsApply(mounts []corev1.VolumeMount) []corev1.VolumeMount {
	return util.MergeVolumeMounts(mounts, util.MountSetterMap{
		hostsVolumeName: util.VolumeMountWithSubpath("/etc/hosts", "hosts"),
		runVolumeName:   util.VolumeMount(ovnRunDir),
		dataVolumeName:  util.VolumeMount(ovnDBDir),
	})
}

func dbContainerEnvApply(envs []corev1.EnvVar, server *ovnv1alpha1.OVSDBServer) []corev1.EnvVar {
	envVars := map[string]env.Setter{}
	envVars["OVN_LOG_LEVEL"] = env.SetValue("info")
	envVars["OVN_RUNDIR"] = env.SetValue(ovnRunDir)
	envVars["OVN_DBDIR"] = env.SetValue(ovnDBDir)
	envVars["DB_TYPE"] = env.SetValue(string(server.Spec.DBType))
	envVars["SERVER_NAME"] = env.SetValue(serviceName(server))
	return env.MergeEnvs(envs, envVars)
}

// Define a local entry for the service in /hosts pointing to the pod IP. This
// allows ovsdb-server to bind to the 'service ip' on startup.
func hostsInitContainerApply(
	container *corev1.Container,
	server *ovnv1alpha1.OVSDBServer,
	cluster *ovnv1alpha1.OVSDBCluster) {

	const hostsTmpMount = "/hosts-new"
	container.Name = "override-local-service-ip"
	container.Image = cluster.Spec.Image
	container.Command = []string{
		"/bin/bash",
		"-c",
		"cp /etc/hosts $HOSTS_VOLUME/hosts; " +
			"echo \"$POD_IP $SERVER_NAME\" >> $HOSTS_VOLUME/hosts",
	}
	container.VolumeMounts = util.MergeVolumeMounts(container.VolumeMounts, util.MountSetterMap{
		hostsVolumeName: util.VolumeMount(hostsTmpMount),
	})
	envVars := map[string]env.Setter{}
	envVars["HOSTS_VOLUME"] = env.SetValue(hostsTmpMount)
	envVars["POD_IP"] = util.EnvDownwardAPI("status.podIP")
	envVars["SERVER_NAME"] = env.SetValue(serviceName(server))
	container.Env = env.MergeEnvs(container.Env, envVars)

	// XXX: Dev only. Both pods use this container, so this ensures we
	// always pull the latest image.
	container.ImagePullPolicy = corev1.PullAlways
}

func dbStatusContainerApply(
	container *corev1.Container,
	server *ovnv1alpha1.OVSDBServer,
	cluster *ovnv1alpha1.OVSDBCluster) {

	container.Name = dbStatusContainerName
	container.Image = cluster.Spec.Image
	container.Command = []string{"/dbstatus"}
	container.VolumeMounts = dbContainerVolumeMountsApply(container.VolumeMounts)
	container.Env = dbContainerEnvApply(container.Env, server)
}

func dbServerContainerApply(
	container *corev1.Container,
	server *ovnv1alpha1.OVSDBServer,
	cluster *ovnv1alpha1.OVSDBCluster) {

	container.Name = DBServerContainerName
	container.Image = cluster.Spec.Image
	container.Command = []string{"/dbserver"}
	container.VolumeMounts = dbContainerVolumeMountsApply(container.VolumeMounts)
	container.Env = dbContainerEnvApply(container.Env, server)
}
