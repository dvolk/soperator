package common

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"nebius.ai/slurm-operator/internal/consts"
	"nebius.ai/slurm-operator/internal/naming"
	renderutils "nebius.ai/slurm-operator/internal/render/utils"
	"nebius.ai/slurm-operator/internal/values"
)

// RenderConfigMapSlurmConfigs renders new [corev1.ConfigMap] containing '.conf' files for the following components:
//
// [consts.ConfigMapKeySlurmConfig] - Slurm config
// [consts.ConfigMapKeyCGroupConfig] - cgroup config
// [consts.ConfigMapKeySpankConfig] - SPANK plugins config
// [consts.ConfigMapKeyGresConfig] - gres config
func RenderConfigMapSlurmConfigs(cluster *values.SlurmCluster) (corev1.ConfigMap, error) {
	return corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      naming.BuildConfigMapSlurmConfigsName(cluster.Name),
			Namespace: cluster.Namespace,
			Labels:    RenderLabels(consts.ComponentTypeController, cluster.Name),
		},
		Data: map[string]string{
			consts.ConfigMapKeySlurmConfig:  generateSlurmConfig(cluster).Render(),
			consts.ConfigMapKeyCGroupConfig: generateCGroupConfig(cluster).Render(),
			consts.ConfigMapKeySpankConfig:  generateSpankConfig().Render(),
			consts.ConfigMapKeyGresConfig:   generateGresConfig().Render(),
		},
	}, nil
}

func generateSlurmConfig(cluster *values.SlurmCluster) renderutils.ConfigFile {
	res := &renderutils.PropertiesConfig{}

	res.AddProperty("ClusterName", cluster.Name)
	res.AddComment("")
	// example: SlurmctldHost=controller-0(controller-0.controller.slurm-poc.svc.cluster.local)
	for i := int32(0); i < cluster.NodeController.Size; i++ {
		hostName, hostFQDN := naming.BuildServiceHostFQDN(
			consts.ComponentTypeController,
			cluster.Namespace,
			cluster.Name,
			i,
		)
		res.AddProperty("SlurmctldHost", fmt.Sprintf("%s(%s)", hostName, hostFQDN))
	}
	res.AddComment("")
	res.AddProperty("AuthType", "auth/"+consts.Munge)
	res.AddProperty("CredType", "cred/"+consts.Munge)
	res.AddComment("")
	res.AddProperty("GresTypes", "gpu")
	res.AddProperty("MailProg", "/usr/bin/true")
	res.AddProperty("PluginDir", "/usr/lib/x86_64-linux-gnu/"+consts.Slurm)
	res.AddProperty("ProctrackType", "proctrack/cgroup")
	res.AddProperty("ReturnToService", 2)
	res.AddComment("")
	res.AddProperty("SlurmctldPidFile", "/var/run/"+consts.SlurmctldName+".pid")
	res.AddProperty("SlurmctldPort", cluster.NodeController.ContainerSlurmctld.Port)
	res.AddComment("")
	res.AddProperty("SlurmdPidFile", "/var/run/"+consts.SlurmdName+".pid")
	res.AddProperty("SlurmdPort", cluster.NodeWorker.ContainerSlurmd.Port)
	res.AddComment("")
	res.AddProperty("SlurmdSpoolDir", naming.BuildVolumeMountSpoolPath(consts.SlurmdName))
	res.AddComment("")
	res.AddProperty("SlurmUser", "root")
	res.AddComment("")
	res.AddProperty("StateSaveLocation", naming.BuildVolumeMountSpoolPath(consts.SlurmctldName))
	res.AddComment("")
	res.AddProperty("TaskPlugin", "task/cgroup,task/affinity")
	res.AddComment("")
	res.AddProperty("CliFilterPlugins", "cli_filter/user_defaults")
	res.AddComment("")
	res.AddProperty("LaunchParameters", "use_interactive_step")
	res.AddComment("")
	res.AddProperty("MaxJobCount", 1000) // Keep 1000 last jobs in controller memory
	res.AddProperty("MinJobAge", 86400)  // Don't remove jobs from controller memory after some time
	res.AddComment("")
	res.AddComment("HEALTH CHECKS")
	res.AddComment("https://slurm.schedmd.com/slurm.conf.html#OPT_HealthCheckInterval")
	res.AddProperty("HealthCheckInterval", 30)
	res.AddProperty("HealthCheckProgram", "/usr/bin/gpu_healthcheck.sh")
	res.AddProperty("HealthCheckNodeState", "ANY")
	res.AddComment("")
	res.AddProperty("InactiveLimit", 0)
	res.AddProperty("KillWait", 30)
	res.AddProperty("SlurmctldTimeout", 120)
	res.AddProperty("SlurmdTimeout", 300)
	res.AddProperty("Waittime", 0)
	res.AddComment("")
	res.AddComment("SCHEDULING")
	res.AddProperty("SchedulerType", "sched/backfill")
	res.AddProperty("SelectType", "select/cons_tres")
	res.AddProperty("SelectTypeParameters", "CR_Core_Memory")
	res.AddComment("")
	res.AddComment("LOGGING AND ACCOUNTING")
	res.AddProperty("JobCompType", "jobcomp/none")
	res.AddProperty("JobAcctGatherFrequency", 30)
	res.AddProperty("SlurmctldDebug", "debug3")
	res.AddProperty("SlurmctldLogFile", "/dev/null")
	res.AddProperty("SlurmdDebug", "debug3")
	res.AddProperty("SlurmdLogFile", "/dev/null")
	res.AddComment("")
	res.AddComment("COMPUTE NODES")
	res.AddComment("We're using the \"dynamic nodes\" feature: https://slurm.schedmd.com/dynamic_nodes.html")
	res.AddProperty("MaxNodeCount", "512")
	res.AddProperty("PartitionName", "main Nodes=ALL Default=YES MaxTime=INFINITE State=UP OverSubscribe=YES")

	return res
}

func generateCGroupConfig(cluster *values.SlurmCluster) renderutils.ConfigFile {
	res := &renderutils.PropertiesConfig{}
	res.AddProperty("CgroupMountpoint", "/sys/fs/cgroup")
	res.AddProperty("ConstrainCores", "yes")
	res.AddProperty("ConstrainDevices", "yes")
	res.AddProperty("ConstrainRAMSpace", "yes")
	if cluster.NodeWorker.CgroupVersion == consts.CGroupV2 {
		res.AddProperty("CgroupPlugin", "cgroup/v2")
		res.AddProperty("ConstrainSwapSpace", "no")
		res.AddProperty("EnableControllers", "yes")
		res.AddProperty("IgnoreSystemd", "yes")
		return res
	}
	res.AddProperty("ConstrainSwapSpace", "yes")
	res.AddProperty("CgroupPlugin", "cgroup/v1")
	return res
}

func generateSpankConfig() renderutils.ConfigFile {
	res := &renderutils.MultilineStringConfig{}
	res.AddLine(fmt.Sprintf("required chroot.so %s", consts.VolumeMountPathJail))
	res.AddLine("required spank_pyxis.so runtime_path=/run/pyxis execute_entrypoint=0 container_scope=global sbatch_support=1")
	return res
}

func generateGresConfig() renderutils.ConfigFile {
	res := &renderutils.PropertiesConfig{}
	res.AddProperty("AutoDetect", "nvml")
	return res
}
