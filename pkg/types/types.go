package types

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DstSecretName string

const DstSecretNameSep = "."

func NewDstSecretName(srcSecretNamespace, srcSecretName string) DstSecretName {
	return DstSecretName(srcSecretNamespace + DstSecretNameSep + srcSecretName)
}

func (d DstSecretName) Split() (namespace, name string, ok bool) {
	s := strings.Split(string(d), DstSecretNameSep)
	if len(s) == 1 {
		return "", "", false
	}
	return s[0], s[1], true
}

func (d DstSecretName) String() string {
	return string(d)
}

func NewDstSecret(dstNamespace string, dstName DstSecretName, srcSecret *corev1.Secret) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: dstNamespace,
			Name:      dstName.String(),
			Labels:    NewDstSecretLabels(srcSecret.Namespace, srcSecret.Name),
		},
		Type: srcSecret.Type,
		Data: srcSecret.Data,
	}
}

const DstSecretLabelCreatedByRiggerKey = "created-by-rigger"
const DstSecretLabelCreatedByRiggerValue = "true"
const DstSecretLabelSrcNamespaceKey = "src-namespace"
const DstSecretLabelSrcNameKey = "src-name"

type DstSecretLabels map[string]string

func NewDstSecretLabels(srcSecretNamespace, srcSecretName string) DstSecretLabels {
	return DstSecretLabels{
		DstSecretLabelCreatedByRiggerKey: DstSecretLabelCreatedByRiggerValue,
		DstSecretLabelSrcNamespaceKey:    srcSecretNamespace,
		DstSecretLabelSrcNameKey:         srcSecretName,
	}
}

func (d DstSecretLabels) GetLabelSelector() string {
	ret := make([]string, len(d))
	for k, v := range d {
		ret = append(ret, k+"="+v)
	}
	return strings.Join(ret, ",")
}
