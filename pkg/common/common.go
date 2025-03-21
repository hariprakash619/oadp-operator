package common

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

const (
	Velero                       = "velero"
	NodeAgent                    = "node-agent"
	VeleroNamespace              = "oadp-operator"
	OADPOperator                 = "oadp-operator"
	OADPOperatorVelero           = "oadp-operator-velero"
	DataMover                    = "volume-snapshot-mover"
	DataMoverController          = "data-mover-controller"
	DataMoverControllerContainer = "data-mover-controller-container"
	OADPOperatorServiceAccount   = "openshift-adp-controller-manager"
	VolSyncDeploymentName        = "volsync-controller-manager"
	VolSyncDeploymentNamespace   = "openshift-operators"
)

// Images
const (
	VeleroImage          = "quay.io/konveyor/velero:latest"
	OpenshiftPluginImage = "quay.io/konveyor/openshift-velero-plugin:latest"
	AWSPluginImage       = "quay.io/konveyor/velero-plugin-for-aws:latest"
	AzurePluginImage     = "quay.io/konveyor/velero-plugin-for-microsoft-azure:latest"
	GCPPluginImage       = "quay.io/konveyor/velero-plugin-for-gcp:latest"
	CSIPluginImage       = "quay.io/konveyor/velero-plugin-for-csi:latest"
	VSMPluginImage       = "quay.io/konveyor/velero-plugin-for-vsm:latest"
	// DataMoverImage is the data mover controller for data mover CRs - VolumeSnapshotBackup and VolumeSnapshotRestore
	DataMoverImage      = "quay.io/konveyor/volume-snapshot-mover:latest"
	RegistryImage       = "quay.io/konveyor/registry:latest"
	KubeVirtPluginImage = "quay.io/konveyor/kubevirt-velero-plugin:v0.2.0"
)

// Plugin names
const (
	VeleroPluginForAWS       = "velero-plugin-for-aws"
	VeleroPluginForAzure     = "velero-plugin-for-microsoft-azure"
	VeleroPluginForGCP       = "velero-plugin-for-gcp"
	VeleroPluginForCSI       = "velero-plugin-for-csi"
	VeleroPluginForVSM       = "velero-plugin-for-vsm"
	VeleroPluginForOpenshift = "openshift-velero-plugin"
	KubeVirtPlugin           = "kubevirt-velero-plugin"
)

// Environment Vars keys
const (
	LDLibraryPathEnvKey            = "LD_LIBRARY_PATH"
	VeleroNamespaceEnvKey          = "VELERO_NAMESPACE"
	VeleroScratchDirEnvKey         = "VELERO_SCRATCH_DIR"
	AWSSharedCredentialsFileEnvKey = "AWS_SHARED_CREDENTIALS_FILE"
	AzureCredentialsFileEnvKey     = "AZURE_CREDENTIALS_FILE"
	GCPCredentialsEnvKey           = "GOOGLE_APPLICATION_CREDENTIALS"
	HTTPProxyEnvVar                = "HTTP_PROXY"
	HTTPSProxyEnvVar               = "HTTPS_PROXY"
	NoProxyEnvVar                  = "NO_PROXY"
)

const defaultMode = int32(420)

func DefaultModePtr() *int32 {
	var mode int32 = defaultMode
	return &mode
}

// append labels together
func AppendUniqueLabels(userLabels ...map[string]string) (map[string]string, error) {
	return AppendUniqueKeyStringOfStringMaps(userLabels...)
}

func AppendUniqueKeyStringOfStringMaps(userLabels ...map[string]string) (map[string]string, error) {
	base := map[string]string{}
	for _, labels := range userLabels {
		if labels == nil {
			continue
		}
		for k, v := range labels {
			if base[k] == "" {
				base[k] = v
			} else if base[k] != v {
				return nil, fmt.Errorf("conflicting key %s with value %s may not override %s", k, v, base[k])
			}
		}
	}
	return base, nil
}

// append env vars together where the first one wins
func AppendUniqueEnvVars(userEnvVars ...[]corev1.EnvVar) []corev1.EnvVar {
	base := []corev1.EnvVar{}
	for _, envVars := range userEnvVars {
		if envVars == nil {
			continue
		}
		for _, envVar := range envVars {
			if !containsEnvVar(base, envVar) {
				base = append(base, envVar)
			}
		}
	}
	return base
}

func containsEnvVar(envVars []corev1.EnvVar, envVar corev1.EnvVar) bool {
	for _, e := range envVars {
		if e.Name == envVar.Name {
			return true
		}
	}
	return false
}
