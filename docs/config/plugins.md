<hr style="height:1px;border:none;color:#333;">
<h1 align="center">Plugins Customization</h1>
<hr style="height:1px;border:none;color:#333;">

### Configure Velero Plugins

There are mainly two categories of Velero plugins that can be specified while 
installing Velero:

1. `defaultPlugins`:<br>
   There are six types of default Velero plugins can be installed: 
   - `AWS` [Plugins for AWS
](https://github.com/vmware-tanzu/velero-plugin-for-aws)
   - `GCP` [Plugins for Google Cloud Platform](https://github.com/vmware-tanzu/velero-plugin-for-gcp)
   - `Azure` [Plugins for Microsoft Azure](https://github.com/vmware-tanzu/velero-plugin-for-microsoft-azure)
   - `OpenShift` [OpenShift Velero Plugin](https://github.com/openshift/openshift-velero-plugin)
   - `CSI` [Plugins for CSI](https://github.com/vmware-tanzu/velero-plugin-for-csi)
   - `kubevirt` [Plugins for Kubevirt](https://github.com/kubevirt/kubevirt-velero-plugin)
   - `VSM` [Plugin for Volume-Snapshot-Mover](https://github.com/migtools/velero-plugin-for-vsm)

   For installation, 
   you need to specify them in the `oadp_v1alpha1_dpa.yaml` file 
   during deployment.

   ```
    apiVersion: oadp.openshift.io/v1alpha1
    kind: DataProtectionApplication
    metadata:
      name: dpa-sample
    spec:
      configuration:
        velero:
          defaultPlugins:
          - openshift
          - aws
          - azure
          - gcp
   ```
   The above specification will install Velero with four of the default plugins.
   
2. `customPlugins`:<br>
   For installation of custom Velero plugins, you need to specify the plugin 
   `image` and plugin `name` in the `oadp_v1alpha1_dpa.yaml` file during 
   deployment.

   For instance, 
   ```
    apiVersion: oadp.openshift.io/v1alpha1
    kind: DataProtectionApplication
    metadata:
      name: dpa-sample
    spec:
      configuration:
        velero:
          defaultPlugins:
          - azure
          - gcp
          customPlugins:
          - name: custom-plugin-example
            image: quay.io/example-repo/custom-velero-plugin
   ```
   The above specification will install Velero with three plugins: 
   `azure`, `gcp`, and `custom-plugin-example`.
