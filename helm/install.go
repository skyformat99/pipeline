package helm

import (
	"fmt"
	"github.com/banzaicloud/banzai-types/components"
	"github.com/banzaicloud/banzai-types/components/helm"
	"github.com/banzaicloud/banzai-types/constants"
	"github.com/banzaicloud/banzai-types/utils"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/helm/cmd/helm/installer"
	"k8s.io/helm/pkg/kube"
	"net/http"
)

//Create ServiceAccount and AccountRoleBinding
func PreInstall(helmInstall *helm.Install) error {
	_, client, err := getKubeClient(helmInstall.KubeContext)
	if err != nil {
		utils.LogErrorf(constants.TagHelmInstall, "could not get kubernetes client: %s", err)
		return err
	}
	serviceAccount := &apiv1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tiller",
		},
	}
	client.CoreV1().ServiceAccounts("kube-system").Create(serviceAccount)

	clusterRoleBinding := &v1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tiller",
		},
		RoleRef: v1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "tiller",
		},
		Subjects: []v1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "tiller",
				Namespace: "kube-system",
			}},
	}
	client.RbacV1().ClusterRoleBindings().Create(clusterRoleBinding)
	clusterRole := &v1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tiller",
		},
		Rules: []v1.PolicyRule{{
			APIGroups: []string{
				"",
				"extensions",
				"apps",
			},
			Resources: []string{
				"'*'",
			},
			Verbs: []string{
				"'*'",
			},
		}},
	}
	client.RbacV1().ClusterRoles().Create(clusterRole)
	return nil
}

// Install uses Kubernetes client to install Tiller.
func Install(helmInstall *helm.Install) *components.BanzaiResponse {
	opts := installer.Options{
		Namespace:      helmInstall.Namespace,
		ServiceAccount: helmInstall.ServiceAccount,
	}
	_, kubeClient, err := getKubeClient(helmInstall.KubeContext)
	if err != nil {
		utils.LogErrorf(constants.TagHelmInstall, "could not get kubernetes client: %s", err)
		return &components.BanzaiResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("could not get kubernetes client: %s", err),
		}
	}
	if err := installer.Install(kubeClient, &opts); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			utils.LogErrorf(constants.TagHelmInstall, "error installing: %s", err)
			return &components.BanzaiResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    fmt.Sprintf("error installing: %s", err),
			}
		}
		if helmInstall.Upgrade {
			if err := installer.Upgrade(kubeClient, &opts); err != nil {
				utils.LogErrorf(constants.TagHelmInstall, "error when upgrading: %s", err)
				return &components.BanzaiResponse{
					StatusCode: http.StatusInternalServerError,
					Message:    fmt.Sprintf("error when upgrading: %s", err),
				}
			}
			utils.LogInfo(constants.TagHelmInstall, "Tiller (the Helm server-side component) has been upgraded to the current version.")
		} else {
			utils.LogInfo(constants.TagHelmInstall, "Warning: Tiller is already installed in the cluster.")
		}
	} else {
		utils.LogInfo(constants.TagHelmInstall, "Tiller (the Helm server-side component) has been installed into your Kubernetes Cluster.")
	}
	utils.LogInfo(constants.TagHelmInstall, "Helm install finished")
	return &components.BanzaiResponse{
		StatusCode: http.StatusOK,
	}
}

// getKubeClient creates a Kubernetes config and client for a given kubeconfig context.
func getKubeClient(context string) (*rest.Config, kubernetes.Interface, error) {
	config, err := configForContext(context)
	if err != nil {
		return nil, nil, err
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, fmt.Errorf("could not get Kubernetes client: %s", err)
	}
	return config, client, nil
}

// configForContext creates a Kubernetes REST client configuration for a given kubeconfig context.
func configForContext(context string) (*rest.Config, error) {
	config, err := kube.GetConfig(context).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("could not get Kubernetes config for context %q: %s", context, err)
	}
	return config, nil
}
