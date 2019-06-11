package dstsecret

import (
	"context"
	"fmt"
	"reflect"

	riggerv1beta1 "github.com/wantedly/rigger/pkg/apis/rigger/v1beta1"
	"github.com/wantedly/rigger/pkg/clientset"
	"github.com/wantedly/rigger/pkg/controller/plan"
	riggertypes "github.com/wantedly/rigger/pkg/types"
	"github.com/wantedly/rigger/pkg/util"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("dst-secret-controller")

// Add creates a new Secret Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileDstSecret{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("dst-secret-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Secret
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileDstSecret{}

// ReconcileSecret reconciles a Secret object
type ReconcileDstSecret struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Secret object and makes changes based on the state read
// and what is in the Secret.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Secrets
// +kubebuilder:rbac:groups=cores,resources=secrets,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileDstSecret) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Secret instance
	dstSecret, dstSecretDeleted, err := util.ReconcilesFetchSecret(r, context.TODO(), request.NamespacedName)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get secret %s", request.NamespacedName.String())
	}
	dstSecretExists := !dstSecretDeleted

	// If the received Secret has been deleted, since the instance does not exist, get name and namespace from the request.
	dstNamespace := request.NamespacedName.Namespace
	dstName := riggertypes.DstSecretName(request.NamespacedName.Name)

	var srcNamespace string
	var srcName string
	// Verify that the Secret is sync target.
	if dstSecretDeleted {
		var ok bool
		srcNamespace, srcName, ok = dstName.Split()
		if !ok {
			return reconcile.Result{}, nil
		}
		found := false
		plan.Cache.Range(func(name, plan interface{}) bool {
			pl := plan.(*riggerv1beta1.Plan)
			if pl.Spec.SyncDestNamespace == dstNamespace && pl.Spec.SyncTargetSecretName == srcName && !util.Contains(srcNamespace, pl.Spec.IgnoreNamespaces) {
				found = true
				return false
			}
			return true // continue
		})
		if !found {
			return reconcile.Result{}, nil
		}
	} else {
		if dstSecret.Labels[riggertypes.DstSecretLabelCreatedByRiggerKey] != riggertypes.DstSecretLabelCreatedByRiggerValue {
			return reconcile.Result{}, nil
		}
		srcNamespace = dstSecret.Labels[riggertypes.DstSecretLabelSrcNamespaceKey]
		srcName = dstSecret.Labels[riggertypes.DstSecretLabelSrcNameKey]
	}

	// Following is operation for sync target.

	// ignore にいるやつを削除する的なことはしなくていいんだっけ
	// なんかログ内のNamespaceの表記揺れがひどい

	srcSecret, srcSecretNotFound, err := util.ReconcilesFetchSecret(r, context.TODO(), types.NamespacedName{Namespace: srcNamespace, Name: srcName})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get secret [namespace:%s,name:%s]", srcNamespace, srcName)
	}
	srcSecretExists := !srcSecretNotFound

	switch {
	case srcSecretExists && dstSecretDeleted:
		// Create destination Secret
		ds := riggertypes.NewDstSecret(dstNamespace, dstName, srcSecret)
		_, err := clientset.CreateSecret(dstNamespace, ds)
		if apierrors.IsAlreadyExists(err) {
			log.Info(fmt.Sprintf("tried to create a secret, but it already exists [namespace%s,name:%s]", dstNamespace, dstName))
		} else if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to create secret [namespace:%s,name:%s]", dstNamespace, dstName)
		} else {
			log.Info(fmt.Sprintf("succeeded to create secret [namespace:%s,name:%s]", dstNamespace, dstName))
		}
	case srcSecretExists && dstSecretExists:
		// Update destination Secret
		if reflect.DeepEqual(srcSecret.Data, dstSecret.Data) {
			return reconcile.Result{}, nil
		}
		ds := riggertypes.NewDstSecret(dstNamespace, dstName, srcSecret)
		_, err := clientset.UpdateSecret(dstNamespace, ds)
		if apierrors.IsNotFound(err) {
			log.Info(fmt.Sprintf("tried to update a secret, but it not found [namespace:%s,name:%s]", dstNamespace, dstName))
		} else if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to update secret [namespace:%s,name:%s]", dstNamespace, dstName)
		} else {
			log.Info(fmt.Sprintf("succeeded to update secret [namespace:%s,name:%s]", dstNamespace, dstName))
		}
	case srcSecretNotFound && dstSecretExists:
		// Delete destination Secret
		err := clientset.DeleteSecret(dstNamespace, dstName.String(), nil)
		if apierrors.IsNotFound(err) {
			log.Info(fmt.Sprintf("tried to delete a secret, but it not found [namespace:%s,name:%s]", dstNamespace, dstName))
		} else if err != nil {
			log.Error(err, fmt.Sprintf("failed to delete secret [namespace:%s,name:%s]", dstNamespace, dstName))
		} else {
			log.Info(fmt.Sprintf("succeeded to delete secret [namespace:%s,name:%s]", dstNamespace, dstName))
		}
	}
	return reconcile.Result{}, nil
}
