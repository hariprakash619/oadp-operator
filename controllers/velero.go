package controllers

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/openshift/oadp-operator/pkg/credentials"
	"github.com/operator-framework/operator-lib/proxy"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"

	//"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	"github.com/openshift/oadp-operator/pkg/common"
	"github.com/vmware-tanzu/velero/pkg/install"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	//"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	Server   = "server"
	Registry = "Registry"
	//TODO: Check for default secret names
	VeleroAWSSecretName   = "cloud-credentials"
	VeleroAzureSecretName = "cloud-credentials-azure"
	VeleroGCPSecretName   = "cloud-credentials-gcp"
	enableCSIFeatureFlag  = "EnableCSI"
)

var (
	veleroLabelSelector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"k8s-app":   "openshift-adp",
			"component": common.Velero,
			"deploy":    common.Velero,
		},
	}
	oadpAppLabel = map[string]string{
		"app.kubernetes.io/name":       common.Velero,
		"app.kubernetes.io/managed-by": common.OADPOperator,
		"app.kubernetes.io/component":  Server,
		oadpv1alpha1.OadpOperatorLabel: "True",
	}
)

// TODO: Remove this function as it's no longer being used
func (r *DPAReconciler) ReconcileVeleroServiceAccount(log logr.Logger) (bool, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}
	veleroSa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.Velero,
			Namespace: dpa.Namespace,
		},
	}
	op, err := controllerutil.CreateOrPatch(r.Context, r.Client, veleroSa, func() error {
		// Setting controller owner reference on the velero SA
		err := controllerutil.SetControllerReference(&dpa, veleroSa, r.Scheme)
		if err != nil {
			return err
		}

		// update the SA template
		veleroSaUpdate, err := r.veleroServiceAccount(&dpa)
		veleroSa = veleroSaUpdate
		return err
	})

	if err != nil {
		return false, err
	}

	//TODO: Review velero SA status and report errors and conditions

	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		// Trigger event to indicate velero SA was created or updated
		r.EventRecorder.Event(veleroSa,
			corev1.EventTypeNormal,
			"VeleroServiceAccountReconciled",
			fmt.Sprintf("performed %s on velero service account %s/%s", op, veleroSa.Namespace, veleroSa.Name),
		)
	}
	return true, nil
}

// TODO: Remove this function as it's no longer being used
//TODO: Temporary solution for Non-OLM Operator install
func (r *DPAReconciler) ReconcileVeleroCRDs(log logr.Logger) (bool, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}

	// check for Non-OLM install and proceed with Velero supporting CRD installation
	/*if velero.Spec.OlmManaged != nil && !*velero.Spec.OlmManaged {
		err := r.InstallVeleroCRDs(log)
		if err != nil {
			return false, err
		}
	}*/

	return true, nil
}

// TODO: Remove this function as it's no longer being used
func (r *DPAReconciler) InstallVeleroCRDs(log logr.Logger) error {
	var err error
	// Install CRDs
	for _, unstructuredCrd := range install.AllCRDs().Items {
		foundCrd := &v1.CustomResourceDefinition{}
		crd := &v1.CustomResourceDefinition{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredCrd.Object, crd); err != nil {
			return err
		}
		// Add Conversion to the spec, as this will be returned in the foundCrd
		crd.Spec.Conversion = &v1.CustomResourceConversion{
			Strategy: v1.NoneConverter,
		}
		if err = r.Client.Get(r.Context, types.NamespacedName{Name: crd.ObjectMeta.Name}, foundCrd); err != nil {
			if errors.IsNotFound(err) {
				// Didn't find CRD, we should create it.
				log.Info("Creating CRD", "CRD.Name", crd.ObjectMeta.Name)
				if err = r.Client.Create(r.Context, crd); err != nil {
					return err
				}
			} else {
				// Return other errors
				return err
			}
		} else {
			// CRD exists, check if it's updated.
			if !reflect.DeepEqual(foundCrd.Spec, crd.Spec) {
				// Specs aren't equal, update and fix.
				log.Info("Updating CRD", "CRD.Name", crd.ObjectMeta.Name, "foundCrd.Spec", foundCrd.Spec, "crd.Spec", crd.Spec)
				foundCrd.Spec = *crd.Spec.DeepCopy()
				if err = r.Client.Update(r.Context, foundCrd); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// TODO: Remove this function as it's no longer being used
func (r *DPAReconciler) ReconcileVeleroClusterRoleBinding(log logr.Logger) (bool, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}
	veleroCRB, err := r.veleroClusterRoleBinding(&dpa)
	if err != nil {
		return false, err
	}
	op, err := controllerutil.CreateOrPatch(r.Context, r.Client, veleroCRB, func() error {
		// Setting controller owner reference on the velero CRB
		// TODO: HOW DO I DO THIS?? ALAY HALP PLZ
		/*err := controllerutil.SetControllerReference(&velero, veleroCRB, r.Scheme)
		if err != nil {
			return err
		}*/

		// update the CRB template
		veleroCRBUpdate, err := r.veleroClusterRoleBinding(&dpa)
		veleroCRB = veleroCRBUpdate
		return err
	})

	if err != nil {
		return false, err
	}

	//TODO: Review velero CRB status and report errors and conditions

	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		// Trigger event to indicate velero SA was created or updated
		r.EventRecorder.Event(veleroCRB,
			corev1.EventTypeNormal,
			"VeleroClusterRoleBindingReconciled",
			fmt.Sprintf("performed %s on velero clusterrolebinding %s", op, veleroCRB.Name),
		)
	}
	return true, nil
}

func (r *DPAReconciler) ReconcileVeleroDeployment(log logr.Logger) (bool, error) {
	dpa := oadpv1alpha1.DataProtectionApplication{}
	if err := r.Get(r.Context, r.NamespacedName, &dpa); err != nil {
		return false, err
	}

	veleroDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.Velero,
			Namespace: dpa.Namespace,
		},
	}
	op, err := controllerutil.CreateOrPatch(r.Context, r.Client, veleroDeployment, func() error {

		// Setting Deployment selector if a new object is created as it is immutable
		if veleroDeployment.ObjectMeta.CreationTimestamp.IsZero() {
			veleroDeployment.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: r.getDpaAppLabels(&dpa),
			}
		}

		// update the Deployment template
		err := r.buildVeleroDeployment(veleroDeployment, &dpa)
		if err != nil {
			return err
		}

		// Setting controller owner reference on the velero deployment
		return controllerutil.SetControllerReference(&dpa, veleroDeployment, r.Scheme)
	})

	if err != nil {
		if errors.IsInvalid(err) {
			cause, isStatusCause := errors.StatusCause(err, metav1.CauseTypeFieldValueInvalid)
			if isStatusCause && cause.Field == "spec.selector" {
				// recreate deployment
				// TODO: check for in-progress backup/restore to wait for it to finish
				log.Info("Found immutable selector from previous deployment, recreating Velero Deployment")
				err := r.Delete(r.Context, veleroDeployment)
				if err != nil {
					return false, err
				}
				return r.ReconcileVeleroDeployment(log)
			}
		}

		return false, err
	}

	//TODO: Review velero deployment status and report errors and conditions

	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated {
		// Trigger event to indicate velero deployment was created or updated
		r.EventRecorder.Event(veleroDeployment,
			corev1.EventTypeNormal,
			"VeleroDeploymentReconciled",
			fmt.Sprintf("performed %s on velero deployment %s/%s", op, veleroDeployment.Namespace, veleroDeployment.Name),
		)
	}
	return true, nil
}

func (r *DPAReconciler) veleroServiceAccount(dpa *oadpv1alpha1.DataProtectionApplication) (*corev1.ServiceAccount, error) {
	annotations := make(map[string]string)
	sa := install.ServiceAccount(dpa.Namespace, annotations)
	sa.Labels = r.getDpaAppLabels(dpa)
	return sa, nil
}

func (r *DPAReconciler) veleroClusterRoleBinding(dpa *oadpv1alpha1.DataProtectionApplication) (*rbacv1.ClusterRoleBinding, error) {
	crb := install.ClusterRoleBinding(dpa.Namespace)
	crb.Labels = r.getDpaAppLabels(dpa)
	return crb, nil
}

// Build VELERO Deployment
func (r *DPAReconciler) buildVeleroDeployment(veleroDeployment *appsv1.Deployment, dpa *oadpv1alpha1.DataProtectionApplication) error {

	if dpa == nil {
		return fmt.Errorf("DPA CR cannot be nil")
	}
	if veleroDeployment == nil {
		return fmt.Errorf("velero deployment cannot be nil")
	}

	//check if CSI plugin is added in spec
	for _, plugin := range dpa.Spec.Configuration.Velero.DefaultPlugins {
		if plugin == oadpv1alpha1.DefaultPluginCSI {
			// CSI plugin is added so ensure that CSI feature flags is set
			dpa.Spec.Configuration.Velero.FeatureFlags = append(dpa.Spec.Configuration.Velero.FeatureFlags, enableCSIFeatureFlag)
			break
		}
	}
	_, err := r.ReconcileRestoreResourcesVersionPriority(dpa)
	if err != nil {
		return fmt.Errorf("error creating configmap for restore resource version priority:" + err.Error())
	}

	// get resource requirements for velero deployment
	// ignoring err here as it is checked in validator.go
	veleroResourceReqs, _ := r.getVeleroResourceReqs(dpa)

	// TODO! Reuse removeDuplicateValues with interface type
	dpa.Spec.Configuration.Velero.DefaultPlugins = removeDuplicatePluginValues(dpa.Spec.Configuration.Velero.DefaultPlugins)
	dpa.Spec.Configuration.Velero.FeatureFlags = removeDuplicateValues(dpa.Spec.Configuration.Velero.FeatureFlags)
	installDeployment := install.Deployment(veleroDeployment.Namespace,
		install.WithResources(veleroResourceReqs),
		install.WithImage(getVeleroImage(dpa)),
		install.WithFeatures(dpa.Spec.Configuration.Velero.FeatureFlags),
		install.WithAnnotations(dpa.Spec.PodAnnotations),
		// use WithSecret false even if we have secret because we use a different VolumeMounts and EnvVars
		// see: https://github.com/vmware-tanzu/velero/blob/ed5809b7fc22f3661eeef10bdcb63f0d74472b76/pkg/install/deployment.go#L223-L261
		// our secrets are appended to containers/volumeMounts in credentials.AppendPluginSpecificSpecs function
		install.WithSecret(false),
		install.WithServiceAccountName(common.Velero),
	)
	veleroDeploymentName := veleroDeployment.Name
	veleroDeployment.TypeMeta = installDeployment.TypeMeta
	veleroDeployment.Spec = installDeployment.Spec
	veleroDeployment.Labels, _ = common.AppendUniqueLabels(veleroDeployment.Labels, installDeployment.Labels)
	veleroDeployment.Name = veleroDeploymentName
	return r.customizeVeleroDeployment(dpa, veleroDeployment)
}

func removeDuplicatePluginValues(slice []oadpv1alpha1.DefaultPlugin) []oadpv1alpha1.DefaultPlugin {
	if slice == nil {
		return nil
	}
	keys := make(map[oadpv1alpha1.DefaultPlugin]bool)
	list := []oadpv1alpha1.DefaultPlugin{}
	for _, entry := range slice {
		if _, found := keys[entry]; !found { //add entry to list if not found in keys already
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list // return the result through the passed in argument
}

// remove duplicate entry in string slice
func removeDuplicateValues(slice []string) []string {
	if slice == nil {
		return nil
	}
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range slice {
		if _, found := keys[entry]; !found { //add entry to list if not found in keys already
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list // return the result through the passed in argument
}

func (r *DPAReconciler) customizeVeleroDeployment(dpa *oadpv1alpha1.DataProtectionApplication, veleroDeployment *appsv1.Deployment) error {
	//append dpa labels
	var err error
	veleroDeployment.Labels, err = common.AppendUniqueLabels(veleroDeployment.Labels, r.getDpaAppLabels(dpa))
	if err != nil {
		return fmt.Errorf("velero deployment label: %v", err)
	}
	if veleroDeployment.Spec.Selector == nil {
		veleroDeployment.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: make(map[string]string),
		}
	}
	if veleroDeployment.Spec.Selector.MatchLabels == nil {
		veleroDeployment.Spec.Selector.MatchLabels = make(map[string]string)
	}
	veleroDeployment.Spec.Selector.MatchLabels, err = common.AppendUniqueLabels(veleroDeployment.Spec.Selector.MatchLabels, veleroDeployment.Labels, r.getDpaAppLabels(dpa))
	if err != nil {
		return fmt.Errorf("velero deployment selector label: %v", err)
	}
	veleroDeployment.Spec.Template.Labels, err = common.AppendUniqueLabels(veleroDeployment.Spec.Template.Labels, veleroDeployment.Labels)
	if err != nil {
		return fmt.Errorf("velero deployment template label: %v", err)
	}
	// add custom pod labels
	if dpa.Spec.Configuration.Velero != nil && dpa.Spec.Configuration.Velero.PodConfig != nil && dpa.Spec.Configuration.Velero.PodConfig.Labels != nil {
		veleroDeployment.Spec.Template.Labels, err = common.AppendUniqueLabels(veleroDeployment.Spec.Template.Labels, dpa.Spec.Configuration.Velero.PodConfig.Labels)
		if err != nil {
			return fmt.Errorf("velero deployment template custom label: %v", err)
		}
	}

	isSTSNeeded := r.isSTSTokenNeeded(dpa.Spec.BackupLocations, dpa.Namespace)

	// Selector: veleroDeployment.Spec.Selector,
	veleroDeployment.Spec.Replicas = pointer.Int32(1)
	if dpa.Spec.Configuration.Velero.PodConfig != nil {
		veleroDeployment.Spec.Template.Spec.Tolerations = dpa.Spec.Configuration.Velero.PodConfig.Tolerations
		veleroDeployment.Spec.Template.Spec.NodeSelector = dpa.Spec.Configuration.Velero.PodConfig.NodeSelector
	}
	veleroDeployment.Spec.Template.Spec.Volumes = append(veleroDeployment.Spec.Template.Spec.Volumes,
		corev1.Volume{
			Name: "certs",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})

	if isSTSNeeded {
		expirationSeconds := int64(3600)
		veleroDeployment.Spec.Template.Spec.Volumes = append(veleroDeployment.Spec.Template.Spec.Volumes,
			corev1.Volume{
				Name: "bound-sa-token",
				VolumeSource: corev1.VolumeSource{
					Projected: &corev1.ProjectedVolumeSource{
						DefaultMode: common.DefaultModePtr(),
						Sources: []corev1.VolumeProjection{
							{
								ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
									Audience:          "openshift",
									ExpirationSeconds: &expirationSeconds,
									Path:              "token",
								},
							},
						},
					},
				},
			},
		)
	}
	//add any default init containers here if needed eg: setup-certificate-secret
	// When you do this
	// - please set the ImagePullPolicy to Always, and
	// - please also update the test
	if veleroDeployment.Spec.Template.Spec.InitContainers == nil {
		veleroDeployment.Spec.Template.Spec.InitContainers = []corev1.Container{}
	}

	// attach DNS policy and config if enabled
	veleroDeployment.Spec.Template.Spec.DNSPolicy = dpa.Spec.PodDnsPolicy
	if !reflect.DeepEqual(dpa.Spec.PodDnsConfig, corev1.PodDNSConfig{}) {
		veleroDeployment.Spec.Template.Spec.DNSConfig = &dpa.Spec.PodDnsConfig
	}

	var veleroContainer *corev1.Container
	for i, container := range veleroDeployment.Spec.Template.Spec.Containers {
		if container.Name == common.Velero {
			veleroContainer = &veleroDeployment.Spec.Template.Spec.Containers[i]
			break
		}
	}
	if err := r.customizeVeleroContainer(dpa, veleroDeployment, veleroContainer, isSTSNeeded); err != nil {
		return err
	}

	providerNeedsDefaultCreds, hasCloudStorage, err := r.noDefaultCredentials(*dpa)
	if err != nil {
		return err
	}

	if dpa.Spec.Configuration.Velero.LogLevel != "" {
		logLevel, err := logrus.ParseLevel(dpa.Spec.Configuration.Velero.LogLevel)
		if err != nil {
			return fmt.Errorf("invalid log level %s, use: %s", dpa.Spec.Configuration.Velero.LogLevel, "trace, debug, info, warning, error, fatal, or panic")
		}
		veleroContainer.Args = append(veleroContainer.Args, "--log-level", logLevel.String())
	}

	// Setting async operations server parameter ItemOperationSyncFrequency
	if dpa.Spec.Configuration.Velero.ItemOperationSyncFrequency != "" {
		ItemOperationSyncFrequencyString := dpa.Spec.Configuration.Velero.ItemOperationSyncFrequency
		if err != nil {
			return err
		}
		veleroContainer.Args = append(veleroContainer.Args, fmt.Sprintf("--item-operation-sync-frequency=%v", ItemOperationSyncFrequencyString))
	}

	// Setting async operations server parameter DefaultItemOperationTimeout
	if dpa.Spec.Configuration.Velero.DefaultItemOperationTimeout != "" {
		DefaultItemOperationTimeoutString := dpa.Spec.Configuration.Velero.DefaultItemOperationTimeout
		if err != nil {
			return err
		}
		veleroContainer.Args = append(veleroContainer.Args, fmt.Sprintf("--default-item-operation-timeout=%v", DefaultItemOperationTimeoutString))
	}

	if dpa.Spec.Configuration.Velero.ResourceTimeout != "" {
		resourceTimeoutString := dpa.Spec.Configuration.Velero.ResourceTimeout
		if err != nil {
			return err
		}
		veleroContainer.Args = append(veleroContainer.Args, fmt.Sprintf("--resource-timeout=%v", resourceTimeoutString))
	}

	// Set defaults to avoid update events
	if veleroDeployment.Spec.Strategy.Type == "" {
		veleroDeployment.Spec.Strategy.Type = appsv1.RollingUpdateDeploymentStrategyType
	}
	if veleroDeployment.Spec.Strategy.RollingUpdate == nil {
		veleroDeployment.Spec.Strategy.RollingUpdate = &appsv1.RollingUpdateDeployment{
			MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
			MaxSurge:       &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
		}
	}
	if veleroDeployment.Spec.RevisionHistoryLimit == nil {
		veleroDeployment.Spec.RevisionHistoryLimit = pointer.Int32(10)
	}
	if veleroDeployment.Spec.ProgressDeadlineSeconds == nil {
		veleroDeployment.Spec.ProgressDeadlineSeconds = pointer.Int32(600)
	}
	setPodTemplateSpecDefaults(&veleroDeployment.Spec.Template)
	return credentials.AppendPluginSpecificSpecs(dpa, veleroDeployment, veleroContainer, providerNeedsDefaultCreds, hasCloudStorage)
}

func (r *DPAReconciler) customizeVeleroContainer(dpa *oadpv1alpha1.DataProtectionApplication, veleroDeployment *appsv1.Deployment, veleroContainer *corev1.Container, isSTSNeeded bool) error {
	if veleroContainer == nil {
		return fmt.Errorf("could not find velero container in Deployment")
	}
	veleroContainer.ImagePullPolicy = corev1.PullAlways
	veleroContainer.VolumeMounts = append(veleroContainer.VolumeMounts,
		corev1.VolumeMount{
			Name:      "certs",
			MountPath: "/etc/ssl/certs",
		},
	)

	if isSTSNeeded {
		veleroContainer.VolumeMounts = append(veleroContainer.VolumeMounts,
			corev1.VolumeMount{
				Name:      "bound-sa-token",
				MountPath: "/var/run/secrets/openshift/serviceaccount",
				ReadOnly:  true,
			})
	}
	// append velero PodConfig envs to container
	if dpa.Spec.Configuration != nil && dpa.Spec.Configuration.Velero != nil && dpa.Spec.Configuration.Velero.PodConfig != nil && dpa.Spec.Configuration.Velero.PodConfig.Env != nil {
		veleroContainer.Env = common.AppendUniqueEnvVars(veleroContainer.Env, dpa.Spec.Configuration.Velero.PodConfig.Env)
	}
	// Append proxy settings to the container from environment variables
	veleroContainer.Env = common.AppendUniqueEnvVars(veleroContainer.Env, proxy.ReadProxyVarsFromEnv())
	if dpa.BackupImages() {
		veleroContainer.Env = common.AppendUniqueEnvVars(veleroContainer.Env, []corev1.EnvVar{{
			Name:  "OPENSHIFT_IMAGESTREAM_BACKUP",
			Value: "true",
		}})
	}

	// Check if data-mover is enabled and set the env var so that the csi data-mover code path is triggred
	if r.checkIfDataMoverIsEnabled(dpa) {
		veleroContainer.Env = common.AppendUniqueEnvVars(veleroContainer.Env, []corev1.EnvVar{{
			Name:  "VOLUME_SNAPSHOT_MOVER",
			Value: "true",
		}})

		if len(dpa.Spec.Features.DataMover.Timeout) > 0 {
			veleroContainer.Env = common.AppendUniqueEnvVars(veleroContainer.Env, []corev1.EnvVar{{
				Name:  "DATAMOVER_TIMEOUT",
				Value: dpa.Spec.Features.DataMover.Timeout,
			}})
		}
	}

	// Enable user to specify --fs-backup-timeout (defaults to 1h)
	fsBackupTimeout := "1h"
	if dpa.Spec.Configuration.Restic != nil && len(dpa.Spec.Configuration.Restic.Timeout) > 0 {
		fsBackupTimeout = dpa.Spec.Configuration.Restic.Timeout
	}
	// Append restic timeout option manually. Not configurable via install package, missing from podTemplateConfig struct. See: https://github.com/vmware-tanzu/velero/blob/8d57215ded1aa91cdea2cf091d60e072ce3f340f/pkg/install/deployment.go#L34-L45
	veleroContainer.Args = append(veleroContainer.Args, fmt.Sprintf("--fs-backup-timeout=%s", fsBackupTimeout))

	setContainerDefaults(veleroContainer)
	return nil
}

func (r *DPAReconciler) isSTSTokenNeeded(bsls []oadpv1alpha1.BackupLocation, ns string) bool {

	for _, bsl := range bsls {
		if bsl.CloudStorage != nil {
			bucket := &oadpv1alpha1.CloudStorage{}
			err := r.Get(r.Context, client.ObjectKey{
				Name:      bsl.CloudStorage.CloudStorageRef.Name,
				Namespace: ns,
			}, bucket)
			if err != nil {
				//log
				return false
			}
			if bucket.Spec.EnableSharedConfig != nil && *bucket.Spec.EnableSharedConfig {
				return true
			}
		}
	}

	return false
}

func getVeleroImage(dpa *oadpv1alpha1.DataProtectionApplication) string {
	if dpa.Spec.UnsupportedOverrides[oadpv1alpha1.VeleroImageKey] != "" {
		return dpa.Spec.UnsupportedOverrides[oadpv1alpha1.VeleroImageKey]
	}
	if os.Getenv("RELATED_IMAGE_VELERO") == "" {
		return common.VeleroImage
	}
	return os.Getenv("RELATED_IMAGE_VELERO")
}

func (r *DPAReconciler) getDpaAppLabels(dpa *oadpv1alpha1.DataProtectionApplication) map[string]string {
	//append dpa name
	if dpa != nil {
		return getAppLabels(dpa.Name)
	}
	return nil
}

func getAppLabels(instanceName string) map[string]string {
	labels := make(map[string]string)
	//copy base labels
	for k, v := range oadpAppLabel {
		labels[k] = v
	}
	//append instance name
	if instanceName != "" {
		labels["app.kubernetes.io/instance"] = instanceName
	}
	return labels
}

// Get Velero Resource Requirements
func (r *DPAReconciler) getVeleroResourceReqs(dpa *oadpv1alpha1.DataProtectionApplication) (corev1.ResourceRequirements, error) {

	// Set default values
	ResourcesReqs := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
	}

	if dpa != nil && dpa.Spec.Configuration != nil && dpa.Spec.Configuration.Velero != nil && dpa.Spec.Configuration.Velero.PodConfig != nil {
		// Set custom limits and requests values if defined on VELERO Spec
		if dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Requests.Cpu() != nil && dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Requests.Cpu().Value() != 0 {
			parsedQuantity, err := resource.ParseQuantity(dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Requests.Cpu().String())
			ResourcesReqs.Requests[corev1.ResourceCPU] = parsedQuantity
			if err != nil {
				return ResourcesReqs, err
			}
		}

		if dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Requests.Memory() != nil && dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Requests.Memory().Value() != 0 {
			parsedQuantity, err := resource.ParseQuantity(dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Requests.Memory().String())
			ResourcesReqs.Requests[corev1.ResourceMemory] = parsedQuantity
			if err != nil {
				return ResourcesReqs, err
			}
		}

		if dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Limits.Cpu() != nil && dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Limits.Cpu().Value() != 0 {
			if ResourcesReqs.Limits == nil {
				ResourcesReqs.Limits = corev1.ResourceList{}
			}
			parsedQuantity, err := resource.ParseQuantity(dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Limits.Cpu().String())
			ResourcesReqs.Limits[corev1.ResourceCPU] = parsedQuantity
			if err != nil {
				return ResourcesReqs, err
			}
		}

		if dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Limits.Memory() != nil && dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Limits.Memory().Value() != 0 {
			if ResourcesReqs.Limits == nil {
				ResourcesReqs.Limits = corev1.ResourceList{}
			}
			parsedQuantiy, err := resource.ParseQuantity(dpa.Spec.Configuration.Velero.PodConfig.ResourceAllocations.Limits.Memory().String())
			ResourcesReqs.Limits[corev1.ResourceMemory] = parsedQuantiy
			if err != nil {
				return ResourcesReqs, err
			}
		}

	}

	return ResourcesReqs, nil
}

// Get Restic Resource Requirements
func (r *DPAReconciler) getResticResourceReqs(dpa *oadpv1alpha1.DataProtectionApplication) (corev1.ResourceRequirements, error) {

	// Set default values
	ResourcesReqs := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
	}

	if dpa != nil && dpa.Spec.Configuration != nil && dpa.Spec.Configuration.Restic != nil && dpa.Spec.Configuration.Restic.PodConfig != nil {
		// Set custom limits and requests values if defined on Restic Spec
		if dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Requests.Cpu() != nil && dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Requests.Cpu().Value() != 0 {
			parsedQuantity, err := resource.ParseQuantity(dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Requests.Cpu().String())
			ResourcesReqs.Requests[corev1.ResourceCPU] = parsedQuantity
			if err != nil {
				return ResourcesReqs, err
			}
		}

		if dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Requests.Memory() != nil && dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Requests.Memory().Value() != 0 {
			parsedQuantity, err := resource.ParseQuantity(dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Requests.Memory().String())
			ResourcesReqs.Requests[corev1.ResourceMemory] = parsedQuantity
			if err != nil {
				return ResourcesReqs, err
			}
		}

		if dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Limits.Cpu() != nil && dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Limits.Cpu().Value() != 0 {
			if ResourcesReqs.Limits == nil {
				ResourcesReqs.Limits = corev1.ResourceList{}
			}
			parsedQuantity, err := resource.ParseQuantity(dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Limits.Cpu().String())
			ResourcesReqs.Limits[corev1.ResourceCPU] = parsedQuantity
			if err != nil {
				return ResourcesReqs, err
			}
		}

		if dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Limits.Memory() != nil && dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Limits.Memory().Value() != 0 {
			if ResourcesReqs.Limits == nil {
				ResourcesReqs.Limits = corev1.ResourceList{}
			}
			parsedQuantiy, err := resource.ParseQuantity(dpa.Spec.Configuration.Restic.PodConfig.ResourceAllocations.Limits.Memory().String())
			ResourcesReqs.Limits[corev1.ResourceMemory] = parsedQuantiy
			if err != nil {
				return ResourcesReqs, err
			}
		}

	}

	return ResourcesReqs, nil
}

// noDefaultCredentials determines if a provider needs the default credentials.
// This returns a map of providers found to if they need a default credential,
// a boolean if Cloud Storage backup storage location was used and an error if any occured.
func (r DPAReconciler) noDefaultCredentials(dpa oadpv1alpha1.DataProtectionApplication) (map[string]bool, bool, error) {
	providerNeedsDefaultCreds := map[string]bool{}
	hasCloudStorage := false
	if dpa.Spec.Configuration.Velero.NoDefaultBackupLocation {
		needDefaultCred := false

		if dpa.Spec.UnsupportedOverrides[oadpv1alpha1.OperatorTypeKey] == oadpv1alpha1.OperatorTypeMTC {
			// MTC requires default credentials
			needDefaultCred = true
		}
		// go through cloudprovider plugins and mark providerNeedsDefaultCreds to false
		for _, provider := range dpa.Spec.Configuration.Velero.DefaultPlugins {
			if psf, ok := credentials.PluginSpecificFields[provider]; ok && psf.IsCloudProvider {
				providerNeedsDefaultCreds[psf.PluginName] = needDefaultCred
			}
		}
	} else {
		for _, bsl := range dpa.Spec.BackupLocations {
			if bsl.Velero != nil && bsl.Velero.Credential == nil {
				bslProvider := strings.TrimPrefix(bsl.Velero.Provider, "velero.io/")
				providerNeedsDefaultCreds[bslProvider] = true
			}
			if bsl.Velero != nil && bsl.Velero.Credential != nil {
				bslProvider := strings.TrimPrefix(bsl.Velero.Provider, "velero.io/")
				if found := providerNeedsDefaultCreds[bslProvider]; !found {
					providerNeedsDefaultCreds[bslProvider] = false
				}
			}
			if bsl.CloudStorage != nil {
				if bsl.CloudStorage.Credential == nil {
					cloudStorage := oadpv1alpha1.CloudStorage{}
					err := r.Get(r.Context, types.NamespacedName{Name: bsl.CloudStorage.CloudStorageRef.Name, Namespace: dpa.Namespace}, &cloudStorage)
					if err != nil {
						return nil, false, err
					}
					providerNeedsDefaultCreds[string(cloudStorage.Spec.Provider)] = true
				} else {
					hasCloudStorage = true
				}
			}
		}
	}
	for _, vsl := range dpa.Spec.SnapshotLocations {
		if vsl.Velero != nil {
			// To handle the case where we want to manually hand the credentials for a cloud storage created
			// Bucket credentials via configuration. Only AWS is supported
			provider := strings.TrimPrefix(vsl.Velero.Provider, "velero.io")
			if vsl.Velero.Credential != nil || provider == string(oadpv1alpha1.AWSBucketProvider) && hasCloudStorage {
				providerNeedsDefaultCreds[provider] = false
			} else {
				providerNeedsDefaultCreds[provider] = true
			}
		}
	}

	return providerNeedsDefaultCreds, hasCloudStorage, nil

}
