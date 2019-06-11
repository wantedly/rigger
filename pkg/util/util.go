package util

import (
	"context"

	riggerv1beta1 "github.com/wantedly/rigger/pkg/apis/rigger/v1beta1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Contains(s string, ss []string) bool {
	for _, e := range ss {
		if e == s {
			return true
		}
	}
	return false
}

func Diff(base, changed []string) (added, deleted []string) {
	added = []string{}
	deleted = []string{}
	idx := map[string]bool{}
	for _, v := range base {
		idx[v] = true
	}
	for _, v := range changed {
		if _, ok := idx[v]; ok {
			delete(idx, v)
		} else {
			added = append(added, v)
		}
	}
	for v := range idx {
		deleted = append(deleted, v)
	}
	return
}

func ReconcilesFetchSecret(r client.Reader, ctx context.Context, key types.NamespacedName) (secret *corev1.Secret, notFound bool, err error) {
	secret = &corev1.Secret{}
	if e := r.Get(ctx, key, secret); e != nil {
		if errors.IsNotFound(e) {
			notFound = true // The received Secret has been deleted.
		} else {
			err = e
		}
	}
	return
}

func ReconcilesFetchPlan(r client.Reader, ctx context.Context, key types.NamespacedName) (plan *riggerv1beta1.Plan, notFound bool, err error) {
	plan = &riggerv1beta1.Plan{}
	if e := r.Get(ctx, key, plan); e != nil {
		if errors.IsNotFound(e) {
			notFound = true // The received Plan has been deleted.
		} else {
			err = e
		}
	}
	return
}

func ReconcilesUpdatePlan(r client.Writer, ctx context.Context, plan *riggerv1beta1.Plan) error {
	return r.Update(ctx, plan)
}
