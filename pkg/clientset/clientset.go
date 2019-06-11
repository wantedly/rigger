package clientset

import (
	"log"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var clientset = func() *kubernetes.Clientset {
	var config *rest.Config

	config, err := rest.InClusterConfig()
	if err != nil {
		if err == rest.ErrNotInCluster {
			kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
			config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
			if err != nil {
				log.Fatalf("failed to load local kubernetes config: %v", err)
			}
		} else {
			log.Fatalf("failed to load in-cluster kubernetes config: %v", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("failed to load clientset: %v", err)
	}

	return clientset
}()

func CreateSecret(namespace string, secret *corev1.Secret) (*corev1.Secret, error) {
	return clientset.CoreV1().Secrets(namespace).Create(secret)
}

func UpdateSecret(namespace string, secret *corev1.Secret) (*corev1.Secret, error) {
	return clientset.CoreV1().Secrets(namespace).Update(secret)
}

func DeleteSecret(namespace, name string, options *metav1.DeleteOptions) error {
	return clientset.CoreV1().Secrets(namespace).Delete(name, options)
}

func DeleteSecretCollection(namespace string, options *metav1.DeleteOptions, listOptions metav1.ListOptions) error {
	return clientset.CoreV1().Secrets(namespace).DeleteCollection(options, listOptions)
}

func GetAllNamespaceSecrets() ([]corev1.Secret, error) {
	nslist, err := clientset.CoreV1().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get Namespace list")
	}
	ret := []corev1.Secret{}
	for _, ns := range nslist.Items {
		seclist, err := clientset.CoreV1().Secrets(ns.Name).List(metav1.ListOptions{})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get Secret list in [namespace:%s]", ns.Name)
		}
		ret = append(ret, seclist.Items...)
	}
	return ret, nil
}
